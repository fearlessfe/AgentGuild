package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// DestinationStatus 是收款目的地的状态。
type DestinationStatus string

const (
	DestinationPending  DestinationStatus = "pending"
	DestinationVerified DestinationStatus = "verified"
	DestinationRevoked  DestinationStatus = "revoked"
)

// RecipientRef 返回 sha256(chain|address) 的小写 hex。
//
// 它是公开面唯一暴露的收款标识：既能证明某笔钱付给了哪个目的地，
// 又不泄露钱包地址本身（doc §10）。
func RecipientRef(chain, address string) string {
	sum := sha256.Sum256([]byte(chain + "|" + address))
	return hex.EncodeToString(sum[:])
}

// UnassignedRecipientRef 是 Agent 份额为 0 时使用的占位收款标识。
//
// decision 必须始终带一个 64 位 hex 的 recipient_ref（数据库 CHECK），
// 而"没有钱要付给谁"是一个真实存在的结果——required criterion 未通过时
// 就会走到这里。用一个固定的哨兵值而不是空串，能让第三方一眼看出
// 这笔决策没有任何 Agent 收款方。
var UnassignedRecipientRef = RecipientRef("unassigned", "")

// Destination 是 Agent 的收款目的地。
//
// 钱包不是 Agent 主键，可以轮换；轮换不改变任何历史 decision 里已冻结的
// recipient_ref，因此历史归属不变，旧目的地也无法二次领取（doc §5.5）。
type Destination struct {
	ID           string
	AgentID      string
	Chain        string
	Address      string
	RecipientRef string
	Status       DestinationStatus
	CreatedAt    time.Time
	VerifiedAt   *time.Time
	RevokedAt    *time.Time
}

type NewDestinationParams struct {
	ID        string
	AgentID   string
	Chain     string
	Address   string
	CreatedAt time.Time
}

func NewDestination(params NewDestinationParams) (*Destination, error) {
	for _, field := range []struct{ name, value string }{
		{"id", params.ID},
		{"agent_id", params.AgentID},
		{"chain", params.Chain},
		{"address", params.Address},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if params.CreatedAt.IsZero() {
		return nil, invalid("created_at")
	}
	return &Destination{
		ID: params.ID, AgentID: params.AgentID, Chain: params.Chain,
		Address: params.Address, RecipientRef: RecipientRef(params.Chain, params.Address),
		Status: DestinationPending, CreatedAt: params.CreatedAt.UTC(),
	}, nil
}

func (d *Destination) Verify(now time.Time) error {
	if now.IsZero() {
		return invalid("now")
	}
	if d.Status != DestinationPending {
		return ErrStateConflict
	}
	moment := now.UTC()
	d.Status = DestinationVerified
	d.VerifiedAt = &moment
	return nil
}

// Revoke 撤销目的地。已撤销的目的地不再是任何新决策的收款方。
func (d *Destination) Revoke(now time.Time) error {
	if now.IsZero() {
		return invalid("now")
	}
	if d.Status == DestinationRevoked {
		return ErrStateConflict
	}
	moment := now.UTC()
	d.Status = DestinationRevoked
	d.RevokedAt = &moment
	return nil
}

// Challenge 是绑定目的地前服务端签发的一次性 nonce。
//
// 绑定拆成 challenge + verify 两步：nonce 必须由服务端签发且只能用一次，
// 单步接口无法防重放。
type Challenge struct {
	Nonce      string
	AgentID    string
	Chain      string
	Address    string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

type NewChallengeParams struct {
	Nonce    string
	AgentID  string
	Chain    string
	Address  string
	IssuedAt time.Time
	TTL      time.Duration
}

func NewChallenge(params NewChallengeParams) (*Challenge, error) {
	for _, field := range []struct{ name, value string }{
		{"nonce", params.Nonce},
		{"agent_id", params.AgentID},
		{"chain", params.Chain},
		{"address", params.Address},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if params.IssuedAt.IsZero() {
		return nil, invalid("issued_at")
	}
	if params.TTL <= 0 {
		return nil, invalid("ttl")
	}
	return &Challenge{
		Nonce: params.Nonce, AgentID: params.AgentID, Chain: params.Chain,
		Address: params.Address, IssuedAt: params.IssuedAt.UTC(),
		ExpiresAt: params.IssuedAt.UTC().Add(params.TTL),
	}, nil
}

// Consume 校验并消费 nonce。已消费或过期的 nonce 一律拒绝。
func (c *Challenge) Consume(agentID, chain, address string, now time.Time) error {
	if c.ConsumedAt != nil {
		return ErrStateConflict
	}
	if !now.UTC().Before(c.ExpiresAt) {
		return ErrStateConflict
	}
	if c.AgentID != agentID || c.Chain != chain || c.Address != address {
		return ErrForbidden
	}
	moment := now.UTC()
	c.ConsumedAt = &moment
	return nil
}
