-- 链下奖励账本：sponsor escrow、不可变 RewardPolicy、执行级 RewardLock、
-- 可被第三方验证的 RewardDecision、争议与支付回执、Agent 收款目的地。
--
-- 三条贯穿全表的硬性约定：
-- 1. 金额一律 BIGINT minor units + currency code，禁止浮点。浮点金额既无法
--    保证分账加总闭合，也无法产生稳定的 policy_hash / decision_hash。
-- 2. 分成比例一律整数 basis points。第三方要能跨语言逐字节复算 decision，
--    小数在 JSON 序列化里没有唯一表示（doc §10）。
-- 3. 账本性质的表（分录、decision、争议事件、回执）append-only + 触发器拒绝
--    UPDATE/DELETE，写法沿用 000021_agent_contributions。

-- ---------------------------------------------------------------------------
-- Sponsor escrow
-- ---------------------------------------------------------------------------

-- 余额按 (tenant, currency) 分账。available 是可再承诺的部分，locked 是已被
-- RewardLock 占用、在释放或退款前不得二次承诺的部分。
CREATE TABLE sponsor_escrow_accounts (
    tenant_id TEXT NOT NULL,
    currency TEXT NOT NULL,
    available_minor BIGINT NOT NULL DEFAULT 0,
    locked_minor BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, currency),
    CONSTRAINT sponsor_escrow_accounts_tenant_nonempty CHECK (tenant_id <> ''),
    -- currency allowlist：第一版只支持稳定币与法币，绝不开放任意 code。
    CONSTRAINT sponsor_escrow_accounts_currency_valid CHECK (
        currency IN ('USDC', 'USD')
    ),
    -- 余额永不为负：透支等于无资金背书的承诺。
    CONSTRAINT sponsor_escrow_accounts_balances_nonnegative CHECK (
        available_minor >= 0 AND locked_minor >= 0
    )
);

-- 每一次余额变动的 append-only 分录。idempotency_key 让充值回调、锁定、
-- 释放、退款重复投递都退化成 no-op（doc §10）。
CREATE TABLE sponsor_escrow_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    currency TEXT NOT NULL,
    entry_type TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    available_delta BIGINT NOT NULL,
    locked_delta BIGINT NOT NULL,
    reference_kind TEXT NOT NULL,
    reference_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT sponsor_escrow_entries_account_fk
        FOREIGN KEY (tenant_id, currency)
        REFERENCES sponsor_escrow_accounts (tenant_id, currency),
    CONSTRAINT sponsor_escrow_entries_idempotency_key
        UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT sponsor_escrow_entries_type_valid CHECK (
        entry_type IN ('topup', 'lock', 'release', 'refund')
    ),
    CONSTRAINT sponsor_escrow_entries_reference_kind_valid CHECK (
        reference_kind IN ('topup', 'policy', 'lock', 'decision')
    ),
    CONSTRAINT sponsor_escrow_entries_amount_positive CHECK (amount_minor > 0),
    CONSTRAINT sponsor_escrow_entries_reference_nonempty CHECK (
        reference_id <> '' AND idempotency_key <> ''
    ),
    -- 分录必须守恒：一次变动只能在 available 与 locked 之间搬运，或从外部
    -- 注入（topup）。这条约束让"余额被凭空改写"在数据库层就写不进去。
    CONSTRAINT sponsor_escrow_entries_deltas_balanced CHECK (
        (entry_type = 'topup'
            AND available_delta = amount_minor AND locked_delta = 0)
        OR (entry_type = 'lock'
            AND available_delta = -amount_minor AND locked_delta = amount_minor)
        OR (entry_type = 'release'
            AND available_delta = 0 AND locked_delta = -amount_minor)
        OR (entry_type = 'refund'
            AND available_delta = amount_minor AND locked_delta = -amount_minor)
    )
);

CREATE INDEX sponsor_escrow_entries_account_order
    ON sponsor_escrow_entries (tenant_id, currency, id);

-- ---------------------------------------------------------------------------
-- RewardPolicy：任务级出资，Claim 后不可变
-- ---------------------------------------------------------------------------

CREATE TABLE reward_policies (
    resource_tenant_id TEXT NOT NULL,
    id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    currency TEXT NOT NULL,
    settlement_provider TEXT NOT NULL,
    gross_amount_minor BIGINT NOT NULL,
    funded_amount_minor BIGINT NOT NULL DEFAULT 0,
    -- 逐条 criterion 的权重（bps），键是 criterion_id。总和 ≤ 10000。
    criterion_weights_bps JSONB NOT NULL DEFAULT '{}'::jsonb,
    maintainer_share_bps INT NOT NULL DEFAULT 0,
    reviewer_pool_share_bps INT NOT NULL DEFAULT 0,
    platform_fee_bps INT NOT NULL DEFAULT 0,
    dispute_reserve_bps INT NOT NULL DEFAULT 0,
    -- quality_multiplier 的区间在建 policy 时冻结，Claim 时随快照锁定，
    -- 结果出来后不得调整（doc §5.1）。
    quality_multiplier_min_bps INT NOT NULL DEFAULT 10000,
    quality_multiplier_max_bps INT NOT NULL DEFAULT 10000,
    challenge_period_seconds BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    funded_at TIMESTAMPTZ,
    exhausted_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    PRIMARY KEY (resource_tenant_id, id),
    CONSTRAINT reward_policies_id_key UNIQUE (id),
    CONSTRAINT reward_policies_task_fk
        FOREIGN KEY (resource_tenant_id, task_id)
        REFERENCES tasks (tenant_id, id),
    CONSTRAINT reward_policies_hash_key UNIQUE (resource_tenant_id, policy_hash),
    CONSTRAINT reward_policies_hash_valid CHECK (policy_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT reward_policies_currency_valid CHECK (currency IN ('USDC', 'USD')),
    CONSTRAINT reward_policies_provider_nonempty CHECK (settlement_provider <> ''),
    CONSTRAINT reward_policies_status_valid CHECK (
        status IN ('unfunded', 'funded', 'exhausted', 'cancelled')
    ),
    CONSTRAINT reward_policies_amounts_valid CHECK (
        gross_amount_minor > 0
        AND funded_amount_minor >= 0
        AND funded_amount_minor <= gross_amount_minor
    ),
    CONSTRAINT reward_policies_shares_range CHECK (
        maintainer_share_bps BETWEEN 0 AND 10000
        AND reviewer_pool_share_bps BETWEEN 0 AND 10000
        AND platform_fee_bps BETWEEN 0 AND 10000
        AND dispute_reserve_bps BETWEEN 0 AND 10000
    ),
    -- 平台费与争议准备金先于净额扣除，两者之和不得吃掉全部 gross。
    CONSTRAINT reward_policies_gross_shares_sum CHECK (
        platform_fee_bps + dispute_reserve_bps <= 10000
    ),
    -- maintainer 与 reviewer 池从 net 中切分，其和不得超过 net。
    CONSTRAINT reward_policies_net_shares_sum CHECK (
        maintainer_share_bps + reviewer_pool_share_bps <= 10000
    ),
    CONSTRAINT reward_policies_quality_range CHECK (
        quality_multiplier_min_bps >= 0
        AND quality_multiplier_min_bps <= quality_multiplier_max_bps
        AND quality_multiplier_max_bps <= 20000
    ),
    CONSTRAINT reward_policies_challenge_period_valid CHECK (
        challenge_period_seconds >= 0
    ),
    CONSTRAINT reward_policies_weights_object CHECK (
        jsonb_typeof(criterion_weights_bps) = 'object'
    ),
    -- 状态与时间戳一致：funded 必须有 funded_at，以此类推。
    CONSTRAINT reward_policies_funded_requires_timestamp CHECK (
        status <> 'funded' OR funded_at IS NOT NULL
    ),
    CONSTRAINT reward_policies_exhausted_requires_timestamp CHECK (
        status <> 'exhausted' OR exhausted_at IS NOT NULL
    ),
    CONSTRAINT reward_policies_cancelled_requires_timestamp CHECK (
        status <> 'cancelled' OR cancelled_at IS NOT NULL
    ),
    CONSTRAINT reward_policies_unfunded_has_no_funding CHECK (
        status <> 'unfunded' OR (funded_amount_minor = 0 AND funded_at IS NULL)
    )
);

-- 一个任务同时只能有一条可领取的 policy。修改只能新建 policy，
-- 因此终态（exhausted / cancelled）不占用这个名额。
CREATE UNIQUE INDEX reward_policies_active_task
    ON reward_policies (resource_tenant_id, task_id)
    WHERE status IN ('unfunded', 'funded');

-- ---------------------------------------------------------------------------
-- RewardLock：执行级锁定
-- ---------------------------------------------------------------------------

CREATE TABLE reward_locks (
    resource_tenant_id TEXT NOT NULL,
    id TEXT NOT NULL,
    policy_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL,
    -- Claim 时刻的 policy 快照与其摘要。Issue 之后如何变更都不影响本次锁定，
    -- 这就是"Claim 后 policy 不可变"的落地方式（doc §10）。
    policy_hash TEXT NOT NULL,
    policy_snapshot JSONB NOT NULL,
    currency TEXT NOT NULL,
    locked_amount_minor BIGINT NOT NULL,
    challenge_period_seconds BIGINT NOT NULL,
    status TEXT NOT NULL,
    locked_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    releasable_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    refunded_at TIMESTAMPTZ,
    disputed_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    PRIMARY KEY (resource_tenant_id, id),
    CONSTRAINT reward_locks_id_key UNIQUE (id),
    -- 一个 Execution 最多一条锁：重复 Claim 不会重复占用资金。
    CONSTRAINT reward_locks_execution_key UNIQUE (resource_tenant_id, execution_id),
    CONSTRAINT reward_locks_policy_fk
        FOREIGN KEY (resource_tenant_id, policy_id)
        REFERENCES reward_policies (resource_tenant_id, id),
    CONSTRAINT reward_locks_execution_fk
        FOREIGN KEY (resource_tenant_id, execution_id, task_id)
        REFERENCES executions (tenant_id, id, task_id),
    CONSTRAINT reward_locks_agent_version_fk
        FOREIGN KEY (agent_id, agent_version_id)
        REFERENCES agent_identity_versions (agent_id, id),
    CONSTRAINT reward_locks_hash_valid CHECK (policy_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT reward_locks_currency_valid CHECK (currency IN ('USDC', 'USD')),
    CONSTRAINT reward_locks_amount_positive CHECK (locked_amount_minor > 0),
    CONSTRAINT reward_locks_challenge_period_valid CHECK (
        challenge_period_seconds >= 0
    ),
    CONSTRAINT reward_locks_snapshot_object CHECK (
        jsonb_typeof(policy_snapshot) = 'object'
    ),
    CONSTRAINT reward_locks_status_valid CHECK (
        status IN ('locked', 'releasable', 'released', 'refunded', 'disputed', 'expired')
    ),
    -- 终态必须带对应时间戳，否则"何时释放的"无从对账。
    CONSTRAINT reward_locks_released_requires_timestamp CHECK (
        status <> 'released' OR (released_at IS NOT NULL AND releasable_at IS NOT NULL)
    ),
    CONSTRAINT reward_locks_refunded_requires_timestamp CHECK (
        status <> 'refunded' OR refunded_at IS NOT NULL
    ),
    CONSTRAINT reward_locks_disputed_requires_timestamp CHECK (
        status <> 'disputed' OR disputed_at IS NOT NULL
    ),
    CONSTRAINT reward_locks_expired_requires_timestamp CHECK (
        status <> 'expired' OR expired_at IS NOT NULL
    ),
    CONSTRAINT reward_locks_releasable_requires_timestamp CHECK (
        status <> 'releasable' OR releasable_at IS NOT NULL
    ),
    CONSTRAINT reward_locks_locked_is_clean CHECK (
        status <> 'locked'
        OR (releasable_at IS NULL AND released_at IS NULL
            AND refunded_at IS NULL AND disputed_at IS NULL AND expired_at IS NULL)
    )
);

CREATE INDEX reward_locks_status_expiry
    ON reward_locks (status, expires_at);

CREATE INDEX reward_locks_agent_lookup
    ON reward_locks (agent_id, locked_at DESC, id);

-- ---------------------------------------------------------------------------
-- Agent 收款目的地：钱包不是主键，可轮换
-- ---------------------------------------------------------------------------

-- challenge nonce 由服务端签发且一次性，防止重放绑定他人钱包。
CREATE TABLE payout_destination_challenges (
    nonce TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    chain TEXT NOT NULL,
    address TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    CONSTRAINT payout_destination_challenges_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT payout_destination_challenges_fields_nonempty CHECK (
        nonce <> '' AND chain <> '' AND address <> ''
    ),
    CONSTRAINT payout_destination_challenges_window_valid CHECK (
        expires_at > issued_at
    ),
    CONSTRAINT payout_destination_challenges_consumed_valid CHECK (
        consumed_at IS NULL OR consumed_at >= issued_at
    )
);

CREATE TABLE payout_destinations (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    chain TEXT NOT NULL,
    address TEXT NOT NULL,
    -- recipient_ref = sha256(chain|address)。公开面只暴露它，用来证明
    -- "钱付给了哪个目的地"而不泄露钱包地址本身（doc §10）。
    recipient_ref TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT payout_destinations_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT payout_destinations_agent_recipient_key
        UNIQUE (agent_id, recipient_ref),
    CONSTRAINT payout_destinations_fields_nonempty CHECK (
        chain <> '' AND address <> ''
    ),
    CONSTRAINT payout_destinations_recipient_ref_valid CHECK (
        recipient_ref ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT payout_destinations_status_valid CHECK (
        status IN ('pending', 'verified', 'revoked')
    ),
    CONSTRAINT payout_destinations_verified_requires_timestamp CHECK (
        status <> 'verified' OR verified_at IS NOT NULL
    ),
    CONSTRAINT payout_destinations_revoked_requires_timestamp CHECK (
        status <> 'revoked' OR revoked_at IS NOT NULL
    )
);

-- 同一时刻只有一个生效目的地；轮换 = 撤销旧的再验证新的。历史 decision 里
-- 冻结的 recipient_ref 不受影响，旧目的地也无法二次领取。
CREATE UNIQUE INDEX payout_destinations_active
    ON payout_destinations (agent_id)
    WHERE status = 'verified';

-- ---------------------------------------------------------------------------
-- RewardDecision：append-only，第三方可验证
-- ---------------------------------------------------------------------------

CREATE TABLE reward_decisions (
    decision_hash TEXT PRIMARY KEY,
    resource_tenant_id TEXT NOT NULL,
    lock_id TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    task_spec_hash TEXT NOT NULL,
    contribution_hash TEXT NOT NULL,
    algorithm_version TEXT NOT NULL,
    currency TEXT NOT NULL,
    gross_amount_minor BIGINT NOT NULL,
    platform_fee_minor BIGINT NOT NULL,
    dispute_reserve_minor BIGINT NOT NULL,
    net_amount_minor BIGINT NOT NULL,
    agent_amount_minor BIGINT NOT NULL,
    maintainer_amount_minor BIGINT NOT NULL,
    reviewer_pool_amount_minor BIGINT NOT NULL,
    unallocated_amount_minor BIGINT NOT NULL,
    quality_multiplier_bps INT NOT NULL,
    -- 逐条 criterion 的判定结果，供第三方复算 sum(weight * verified)。
    criterion_results JSONB NOT NULL DEFAULT '[]'::jsonb,
    required_criteria_passed BOOLEAN NOT NULL,
    recipient_ref TEXT NOT NULL,
    challenge_deadline TIMESTAMPTZ NOT NULL,
    signature TEXT NOT NULL,
    signature_algorithm TEXT NOT NULL,
    decided_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT reward_decisions_lock_key UNIQUE (resource_tenant_id, lock_id),
    CONSTRAINT reward_decisions_lock_fk
        FOREIGN KEY (resource_tenant_id, lock_id)
        REFERENCES reward_locks (resource_tenant_id, id),
    CONSTRAINT reward_decisions_hashes_valid CHECK (
        decision_hash ~ '^[0-9a-f]{64}$'
        AND policy_hash ~ '^[0-9a-f]{64}$'
        AND task_spec_hash ~ '^[0-9a-f]{64}$'
        AND contribution_hash ~ '^[0-9a-f]{64}$'
        AND recipient_ref ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT reward_decisions_currency_valid CHECK (currency IN ('USDC', 'USD')),
    CONSTRAINT reward_decisions_algorithm_nonempty CHECK (
        algorithm_version <> '' AND signature <> '' AND signature_algorithm <> ''
    ),
    CONSTRAINT reward_decisions_amounts_nonnegative CHECK (
        gross_amount_minor > 0
        AND platform_fee_minor >= 0 AND dispute_reserve_minor >= 0
        AND net_amount_minor >= 0 AND agent_amount_minor >= 0
        AND maintainer_amount_minor >= 0 AND reviewer_pool_amount_minor >= 0
        AND unallocated_amount_minor >= 0
    ),
    -- 分账必须闭合到最后一分钱，否则账本无法对账。
    CONSTRAINT reward_decisions_gross_balanced CHECK (
        gross_amount_minor = platform_fee_minor + dispute_reserve_minor + net_amount_minor
    ),
    CONSTRAINT reward_decisions_net_balanced CHECK (
        net_amount_minor = agent_amount_minor + maintainer_amount_minor
            + reviewer_pool_amount_minor + unallocated_amount_minor
    ),
    CONSTRAINT reward_decisions_quality_range CHECK (
        quality_multiplier_bps BETWEEN 0 AND 20000
    ),
    CONSTRAINT reward_decisions_criterion_results_array CHECK (
        jsonb_typeof(criterion_results) = 'array'
    ),
    -- doc §10 的硬门禁下沉到数据库：任一 required criterion 未通过或未验证，
    -- Agent 份额必须为 0。应用层的 bug 也写不进一条违反它的记录。
    CONSTRAINT reward_decisions_required_gate CHECK (
        required_criteria_passed OR agent_amount_minor = 0
    )
);

CREATE INDEX reward_decisions_challenge_deadline
    ON reward_decisions (challenge_deadline);

CREATE FUNCTION reject_reward_decision_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'reward_decisions are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER reward_decisions_reject_update
    BEFORE UPDATE ON reward_decisions
    FOR EACH ROW EXECUTE FUNCTION reject_reward_decision_mutation();

CREATE TRIGGER reward_decisions_reject_delete
    BEFORE DELETE ON reward_decisions
    FOR EACH ROW EXECUTE FUNCTION reject_reward_decision_mutation();

-- ---------------------------------------------------------------------------
-- 争议
-- ---------------------------------------------------------------------------

CREATE TABLE reward_disputes (
    resource_tenant_id TEXT NOT NULL,
    id TEXT NOT NULL,
    lock_id TEXT NOT NULL,
    decision_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    opened_by TEXT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL,
    resolution TEXT,
    resolved_by TEXT,
    resolved_at TIMESTAMPTZ,
    resolved_agent_amount_minor BIGINT,
    PRIMARY KEY (resource_tenant_id, id),
    CONSTRAINT reward_disputes_id_key UNIQUE (id),
    -- 一个 lock 一条争议：重复发起是 no-op，不会派生第二条裁决路径。
    CONSTRAINT reward_disputes_lock_key UNIQUE (resource_tenant_id, lock_id),
    CONSTRAINT reward_disputes_lock_fk
        FOREIGN KEY (resource_tenant_id, lock_id)
        REFERENCES reward_locks (resource_tenant_id, id),
    CONSTRAINT reward_disputes_decision_fk
        FOREIGN KEY (decision_hash) REFERENCES reward_decisions (decision_hash),
    CONSTRAINT reward_disputes_status_valid CHECK (
        status IN ('open', 'resolved')
    ),
    CONSTRAINT reward_disputes_opened_by_nonempty CHECK (opened_by <> ''),
    CONSTRAINT reward_disputes_resolution_valid CHECK (
        resolution IS NULL OR resolution IN ('release', 'refund', 'split')
    ),
    CONSTRAINT reward_disputes_resolved_consistent CHECK (
        (status = 'open'
            AND resolution IS NULL AND resolved_by IS NULL
            AND resolved_at IS NULL AND resolved_agent_amount_minor IS NULL)
        OR (status = 'resolved'
            AND resolution IS NOT NULL AND resolved_by IS NOT NULL
            AND resolved_at IS NOT NULL AND resolved_agent_amount_minor IS NOT NULL
            AND resolved_agent_amount_minor >= 0)
    )
);

-- 争议过程本身是账本：谁在何时发起、谁裁决成什么，只能追加。
CREATE TABLE reward_dispute_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dispute_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT reward_dispute_events_dispute_fk
        FOREIGN KEY (dispute_id) REFERENCES reward_disputes (id),
    -- 同一争议的同一类事件只记一次：重复投递幂等。
    CONSTRAINT reward_dispute_events_type_key UNIQUE (dispute_id, event_type),
    CONSTRAINT reward_dispute_events_type_valid CHECK (
        event_type IN ('opened', 'resolved')
    ),
    CONSTRAINT reward_dispute_events_actor_nonempty CHECK (actor_id <> ''),
    CONSTRAINT reward_dispute_events_payload_object CHECK (
        jsonb_typeof(payload) = 'object'
    )
);

CREATE FUNCTION reject_reward_dispute_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'reward_dispute_events are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER reward_dispute_events_reject_update
    BEFORE UPDATE ON reward_dispute_events
    FOR EACH ROW EXECUTE FUNCTION reject_reward_dispute_event_mutation();

CREATE TRIGGER reward_dispute_events_reject_delete
    BEFORE DELETE ON reward_dispute_events
    FOR EACH ROW EXECUTE FUNCTION reject_reward_dispute_event_mutation();

-- ---------------------------------------------------------------------------
-- 支付回执
-- ---------------------------------------------------------------------------

-- 回执是 provider 事实的 append-only 投影，不是可变状态：provider 每报告一次
-- 终态就追加一行。(provider, provider_reference, status) 唯一 ⇒ 重复回调 no-op。
CREATE TABLE payment_receipts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_tenant_id TEXT NOT NULL,
    decision_hash TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_reference TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    currency TEXT NOT NULL,
    amount_minor BIGINT NOT NULL,
    recipient_ref TEXT NOT NULL,
    failure_reason TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_receipts_decision_fk
        FOREIGN KEY (decision_hash) REFERENCES reward_decisions (decision_hash),
    CONSTRAINT payment_receipts_provider_key
        UNIQUE (provider, provider_reference, status),
    CONSTRAINT payment_receipts_fields_nonempty CHECK (
        provider <> '' AND provider_reference <> ''
    ),
    CONSTRAINT payment_receipts_kind_valid CHECK (kind IN ('payment', 'refund')),
    CONSTRAINT payment_receipts_status_valid CHECK (
        status IN ('pending', 'settled', 'failed')
    ),
    CONSTRAINT payment_receipts_currency_valid CHECK (currency IN ('USDC', 'USD')),
    CONSTRAINT payment_receipts_amount_nonnegative CHECK (amount_minor >= 0),
    CONSTRAINT payment_receipts_recipient_ref_valid CHECK (
        recipient_ref ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT payment_receipts_failure_consistent CHECK (
        status = 'failed' OR failure_reason = ''
    )
);

CREATE INDEX payment_receipts_decision_order
    ON payment_receipts (decision_hash, occurred_at, id);

CREATE FUNCTION reject_payment_receipt_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'payment_receipts are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER payment_receipts_reject_update
    BEFORE UPDATE ON payment_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_payment_receipt_mutation();

CREATE TRIGGER payment_receipts_reject_delete
    BEFORE DELETE ON payment_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_payment_receipt_mutation();
