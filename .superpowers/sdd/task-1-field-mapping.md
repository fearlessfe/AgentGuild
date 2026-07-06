# Task 1: Backend/Frontend Field Mapping

This document lists the field mapping between the backend application-layer view structs (in `identity`, `agentversion`, `agentexperience`, `evaluation`) and the frontend TypeScript types. Fields that differ by more than snake_case/PascalCase casing are flagged as mismatches.

Source files reviewed:

- Backend
  - `backend/internal/identity/application/contracts.go`
  - `backend/internal/identity/application/queries.go`
  - `backend/internal/identity/application/commands.go`
  - `backend/internal/agentversion/application/contracts.go`
  - `backend/internal/agentversion/application/queries.go`
  - `backend/internal/agentexperience/application/contracts.go`
  - `backend/internal/agentexperience/application/queries.go`
  - `backend/internal/evaluation/application/contracts.go`
  - `backend/internal/evaluation/application/queries.go`
  - `backend/internal/evaluation/domain/evaluation_run.go`
  - `backend/internal/transport/rest/identity_router.go`
  - `backend/internal/transport/rest/agent_version_router.go`
  - `backend/internal/transport/rest/experience_router.go`
  - `backend/internal/transport/rest/evaluation_router.go`
- Frontend
  - `frontend/src/api/client.ts`
  - `frontend/src/features/agents/agents.types.ts`
  - `frontend/src/features/agents/agents.api.ts`
  - `frontend/src/features/versions/versions.types.ts`
  - `frontend/src/features/versions/versions.api.ts`
  - `frontend/src/features/experiences/experiences.types.ts`
  - `frontend/src/features/experiences/experiences.api.ts`
  - `frontend/src/features/evaluations/evaluations.types.ts`
  - `frontend/src/features/evaluations/evaluations.api.ts`

Legend:

- ✅ Aligned: backend field serializes to the snake_case name the frontend expects.
- ⚠️ Mismatch / missing: the field does not exist on the corresponding frontend type, has a different name, or has an incompatible type/shape.
- 🔜 Design-doc only: the backend struct does not exist yet; it is planned by `2026-07-05-backend-response-format-alignment-design.md`.

---

## 1. identity/application/contracts.go

### `Envelope[T]`

Backend struct (`backend/internal/identity/application/contracts.go:46`):

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `Data` | `data` | ✅ | Maps to `Envelope<T>["data"]` in `frontend/src/api/client.ts:2`. |
| `Meta` | `meta` | ✅ | Maps to `Envelope<T>["meta"]` in `frontend/src/api/client.ts:2`. |

### `Meta`

Backend struct (`backend/internal/identity/application/contracts.go:51`):

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ServerTime` | `server_time` | ✅ | `time.Time` serializes as RFC3339 string; frontend expects `string`. |
| `ResourceVersion` | `resource_version` | ✅ | `int64` → `number`. |
| `PollAfterSeconds` | `poll_after_seconds` | ✅ | `int` → `number`; frontend marks optional (`?`). |
| — | `next_cursor` | ⚠️ | Frontend `Envelope.meta` declares optional `next_cursor?: string` (`client.ts:8`). Backend `Meta` has no corresponding field. Adding tag later will not create this value; it is only meaningful for paginated list endpoints. |

### `AgentView`

Backend struct (`backend/internal/identity/application/contracts.go:57`).
Frontend type: `AgentView` in `frontend/src/features/agents/agents.types.ts:3`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `TenantID` | — | ⚠️ | Backend includes tenant id; frontend `AgentView` has no `tenant_id` field. |
| `Name` | `name` | ✅ | |
| `Description` | `description?` | ✅ | Backend is non-nullable `string`; frontend is optional. Empty string will satisfy `?` at runtime but semantically differs. |
| `Status` | `status: AgentStatus` | ✅ | Values (`pending_activation`, `active`, `suspended`, `revoked`) align. Backend `string` vs frontend union type. |
| `Team` | `team?` | ✅ | Backend is non-nullable `string`; frontend optional. |
| `OwnerID` | — | ⚠️ | Backend includes owner id; frontend has `owner_email` but no `owner_id`. |
| `OwnerEmail` | `owner_email` | ✅ | |
| `Scopes` | `scopes` | ✅ | |
| `RepoScope` | `repo_scope?` | ✅ | Backend non-nullable slice; frontend optional. |
| `CurrentVersion` | — | ⚠️ | Backend embeds `*AgentVersionView`; frontend `AgentView` has no nested version field. This struct currently serializes as PascalCase (`CurrentVersion`), so it is invisible to frontend anyway until JSON tags are added, but even with snake_case the frontend type has no place for it. |
| `LastSeenAt` | `last_seen_at?` | ✅ | `*time.Time` → optional `string`. |
| `BudgetCents` | `budget_cents?` | ✅ | `int64` → optional `number`. |
| `BudgetCurrency` | `budget_currency?` | ✅ | `string` → optional `string`. |
| `CreatedAt` | `created_at` | ✅ | `time.Time` → `string`. |
| `UpdatedAt` | `updated_at?` | ✅ | `time.Time` → optional `string`. |

### `AgentVersionView`

Backend struct (`backend/internal/identity/application/contracts.go:76`).
Frontend type: this maps to `VersionView` in `frontend/src/features/versions/versions.types.ts:3`. (The task brief suggests `agents.types.ts`, but the frontend actually defines version shapes in `versions.types.ts`.)

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `TenantID` | `tenant_id` | ✅ | |
| `AgentID` | `agent_id` | ✅ | |
| `VersionNumber` | `version_number` | ✅ | `int` → `number`. |
| `Runtime` | `runtime` | ✅ | |
| `Model` | `model` | ✅ | |
| `Capabilities` | `capabilities` | ✅ | |
| `ConfigFingerprint` | `config_fingerprint` | ✅ | |
| `CreatedAt` | `created_at` | ✅ | |
| — | `parent_version_id?` | ⚠️ | Frontend expects optional parent version id; backend `AgentVersionView` omits it. |
| — | `status: VersionStatus` | ⚠️ | Frontend expects version status; backend `AgentVersionView` omits it. |
| — | `content_hash` | ⚠️ | Frontend expects `content_hash`; backend `AgentVersionView` omits it. |
| — | `environment_digest` | ⚠️ | Frontend expects `environment_digest`; backend `AgentVersionView` omits it. |
| — | `prompt_ref?` | ⚠️ | Frontend expects optional `prompt_ref`; backend `AgentVersionView` omits it. |
| — | `skill_refs` | ⚠️ | Frontend expects `skill_refs`; backend `AgentVersionView` omits it. |
| — | `memory_ref?` | ⚠️ | Frontend expects optional `memory_ref`; backend `AgentVersionView` omits it. |
| — | `tool_refs` | ⚠️ | Frontend expects `tool_refs`; backend `AgentVersionView` omits it. |
| — | `created_by` | ⚠️ | Frontend expects `created_by`; backend `AgentVersionView` omits it. |
| — | `promoted_at?` | ⚠️ | Frontend expects optional `promoted_at`; backend `AgentVersionView` omits it. |
| — | `retired_at?` | ⚠️ | Frontend expects optional `retired_at`; backend `AgentVersionView` omits it. |
| — | `rejected_reason?` | ⚠️ | Frontend expects optional `rejected_reason`; backend `AgentVersionView` omits it. |

Summary: `AgentVersionView` is a minimal subset of `VersionView`. It is only used as `AgentView.CurrentVersion`; after JSON tag alignment it will still be missing most fields the frontend `VersionView` requires.

### `AccessTokenView`

Backend struct (`backend/internal/identity/application/contracts.go:88`).

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `Token` | — | ⚠️ | No frontend type consumes this view. Agent activation/issue-token endpoints (`/v1/agents/activate`, `/v1/agents/token`) return `Envelope[AccessTokenView]` but the console UI does not declare a TypeScript type for it. |
| `TokenType` | — | ⚠️ | |
| `ExpiresAt` | — | ⚠️ | |
| `AgentID` | — | ⚠️ | |
| `AgentVersionID` | — | ⚠️ | |
| `Scopes` | — | ⚠️ | |
| `RepoScope` | — | ⚠️ | |

### `RegisterAgentResponse`

Backend struct (`backend/internal/identity/application/contracts.go:98`).
Frontend type: `RegisterAgentResponse` in `frontend/src/features/agents/agents.types.ts:33`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `Agent` | `agent` | ✅ | Type is `AgentView`; see `AgentView` mapping above. |
| `ActivationToken` | `activation_token` | ✅ | |
| `ActivationExpiresAt` | `activation_expires_at?` | ✅ | `*time.Time` → optional `string` (frontend allows `string \| null`). |

Note: the nested `Agent` currently contains `CurrentVersion` (see `AgentView` mismatch), which the frontend `AgentView` does not define.

### `ActivationStatusView`

Backend struct (`backend/internal/identity/application/contracts.go:104`).

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `AgentID` | — | ⚠️ | No dedicated frontend type. Endpoint `/v1/agents/{id}/token` returns this envelope. The console only uses `RegisterAgentResponse` for token reveal (`AgentTokenReveal.tsx`). |
| `Status` | — | ⚠️ | |
| `ActivationStatus` | — | ⚠️ | |
| `ActivationExpiresAt` | — | ⚠️ | |
| `ActivatedAt` | — | ⚠️ | |

### `AgentPage`

Backend struct (`backend/internal/identity/application/contracts.go:112`).
Frontend type: `AgentPage` in `frontend/src/features/agents/agents.types.ts:19`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `Items` | `items` | ✅ | `[]AgentView` → `AgentView[]`. |

---

## 2. agentversion/application/contracts.go

### `VersionSummary`

Backend struct (`backend/internal/agentversion/application/contracts.go:129`).
Frontend type: list items use `VersionView` (`frontend/src/features/versions/versions.types.ts:3`) inside `VersionPage` (`versions.types.ts:27`).

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `VersionNumber` | `version_number` | ✅ | |
| `Status` | `status` | ✅ | `domain.VersionStatus` serializes as string; values align with `VersionStatus` union. |
| `ParentVersionID` | `parent_version_id?` | ✅ | `string` → optional `string`. |
| `ConfigFingerprint` | `config_fingerprint` | ✅ | |
| `CreatedAt` | `created_at` | ✅ | |
| `PromotedAt` | `promoted_at?` | ✅ | `*time.Time` → optional `string`. |
| `RetiredAt` | `retired_at?` | ✅ | `*time.Time` → optional `string`. |
| — | `tenant_id` | ⚠️ | Frontend `VersionView` requires `tenant_id`; `VersionSummary` omits it. |
| — | `agent_id` | ⚠️ | Frontend `VersionView` requires `agent_id`; `VersionSummary` omits it. |
| — | `runtime` | ⚠️ | Frontend `VersionView` requires `runtime`; `VersionSummary` omits it. |
| — | `model` | ⚠️ | Frontend `VersionView` requires `model`; `VersionSummary` omits it. |
| — | `capabilities` | ⚠️ | Frontend `VersionView` requires `capabilities`; `VersionSummary` omits it. |
| — | `content_hash` | ⚠️ | Frontend `VersionView` requires `content_hash`; `VersionSummary` omits it. |
| — | `environment_digest` | ⚠️ | Frontend `VersionView` requires `environment_digest`; `VersionSummary` omits it. |
| — | `prompt_ref?` | ⚠️ | Frontend `VersionView` expects optional `prompt_ref`; `VersionSummary` omits it. |
| — | `skill_refs` | ⚠️ | Frontend `VersionView` expects `skill_refs`; `VersionSummary` omits it. |
| — | `memory_ref?` | ⚠️ | Frontend `VersionView` expects optional `memory_ref`; `VersionSummary` omits it. |
| — | `tool_refs` | ⚠️ | Frontend `VersionView` expects `tool_refs`; `VersionSummary` omits it. |
| — | `created_by` | ⚠️ | Frontend `VersionView` expects `created_by`; `VersionSummary` omits it. |
| — | `rejected_reason?` | ⚠️ | Frontend `VersionView` expects optional `rejected_reason`; `VersionSummary` omits it. |

Summary: the list endpoint (`GET /v1/agents/{id}/versions`) returns `[]VersionSummary`, but the frontend `listVersions` call is typed as `VersionPage` containing full `VersionView` items. After adding snake_case tags, the list will still be missing many required frontend fields.

### `VersionDetail`

Backend struct (`backend/internal/agentversion/application/contracts.go:140`).
Frontend type: `VersionView` in `frontend/src/features/versions/versions.types.ts:3`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `TenantID` | `tenant_id` | ✅ | |
| `AgentID` | `agent_id` | ✅ | |
| `VersionNumber` | `version_number` | ✅ | |
| `ParentVersionID` | `parent_version_id?` | ✅ | `string` → optional `string`. |
| `Status` | `status` | ✅ | |
| `Runtime` | `runtime` | ✅ | |
| `Model` | `model` | ✅ | |
| `Capabilities` | `capabilities` | ✅ | |
| `ConfigFingerprint` | `config_fingerprint` | ✅ | |
| `ContentHash` | `content_hash` | ✅ | |
| `EnvironmentDigest` | `environment_digest` | ✅ | |
| `PromptRef` | `prompt_ref?` | ✅ | Backend non-nullable; frontend optional. |
| `SkillRefs` | `skill_refs` | ✅ | |
| `MemoryRef` | `memory_ref?` | ✅ | Backend non-nullable; frontend optional. |
| `ToolRefs` | `tool_refs` | ✅ | |
| `CreatedBy` | `created_by` | ✅ | |
| `CreatedAt` | `created_at` | ✅ | |
| `PromotedAt` | `promoted_at?` | ✅ | |
| `RetiredAt` | `retired_at?` | ✅ | |
| — | `rejected_reason?` | ⚠️ | Frontend expects optional `rejected_reason`; `VersionDetail` does not include it. |

### `RefChange` / `VersionDiff`

Backend structs (`backend/internal/agentversion/application/contracts.go:163` and `:168`).
Frontend type: `VersionDiff` in `frontend/src/features/versions/versions.types.ts:29`.

Current backend shape is internal-only and does **not** match the frontend:

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `BaseVersionID` | `base_version_id` | ✅ | Same concept. |
| — | `target_version_id` | ⚠️ | Frontend expects explicit `target_version_id`. Backend `VersionDiff` only stores `BaseVersionID`; the target is implicit from the request path. The REST DTO planned in the design doc adds this field. |
| `Added map[string]RefChange` | `added_capabilities: string[]` | ⚠️ | Backend is a map keyed with prefixes (`capability:…`, `skill:…`, `tool:…`); frontend expects a flat array of capability strings. The design doc proposes deriving `added_capabilities` from capability entries only. |
| `Removed map[string]RefChange` | `removed_capabilities: string[]` | ⚠️ | Same as `Added`. |
| `Changed map[string]RefChange` | `changed_refs: {field, from?, to?}[]` | ⚠️ | Backend map values contain `From`/`To`; frontend expects an array of objects keyed by `field`. The design doc proposes a REST DTO `versionDiffView` / `refChangeView` to perform this conversion. |

The design doc adds a REST-layer DTO:

```go
type versionDiffView struct {
    BaseVersionID       string          `json:"base_version_id"`
    TargetVersionID     string          `json:"target_version_id"`
    AddedCapabilities   []string        `json:"added_capabilities"`
    RemovedCapabilities []string        `json:"removed_capabilities"`
    ChangedRefs         []refChangeView `json:"changed_refs"`
}

type refChangeView struct {
    Field string  `json:"field"`
    From  *string `json:"from,omitempty"`
    To    *string `json:"to,omitempty"`
}
```

That DTO aligns exactly with frontend `VersionDiff`.

---

## 3. agentexperience/application/contracts.go

### `CandidateSummary`

Backend struct (`backend/internal/agentexperience/application/contracts.go:119`).
Frontend type: `ExperienceCandidateView` in `frontend/src/features/experiences/experiences.types.ts:4`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `TenantID` | `tenant_id` | ✅ | |
| `AgentID` | `agent_id` | ✅ | |
| `SourceTaskID` | `source_task_id?` | ✅ | `string` → optional `string`. |
| `SourceSubmissionID` | `source_submission_id?` | ✅ | `string` → optional `string`. |
| `SourceReviewID` | `source_review_id?` | ✅ | `string` → optional `string`. |
| `EvidenceRef` | `evidence_ref` | ✅ | |
| `ContentHash` | `content_hash` | ✅ | |
| `ApplicableCapabilities` | `applicable_capabilities` | ✅ | |
| `TenantScope` | `tenant_scope` | ✅ | |
| `SensitivityClass` | `sensitivity_class` | ✅ | Backend `string`; frontend `SensitivityClass` union (`public`, `internal`, `restricted`, `forbidden`). |
| `Status` | `status` | ✅ | Backend `string`; frontend `ExperienceStatus` union (`pending_review`, `approved`, `rejected`). |
| `PolicyReason` | `policy_reason?` | ✅ | `string` → optional `string`. |
| `ReviewedBy` | `reviewed_by?` | ✅ | `string` → optional `string`. |
| `ReviewedAt` | `reviewed_at?` | ✅ | `*time.Time` → optional `string`. |
| `CreatedAt` | `created_at` | ✅ | `time.Time` → `string`. |

Note: the create endpoint (`POST /v1/agents/{id}/experiences`) currently returns an inline object `{experience_id, status, sensitivity_class}` rather than `CandidateSummary`, while the frontend `extractExperience` is typed to return `ExperienceCandidateView`. This is a route-level mismatch not fixed by adding JSON tags to `CandidateSummary`.

---

## 4. evaluation/application/contracts.go

### `BenchmarkSetSummary`

Backend struct (`backend/internal/evaluation/application/contracts.go:135`).
Frontend type: `BenchmarkSetView` in `frontend/src/features/evaluations/evaluations.types.ts:1`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `TenantID` | `tenant_id` | ✅ | |
| `VersionNumber` | `version_number` | ✅ | |
| `Name` | `name` | ✅ | |
| `Description` | `description?` | ✅ | Backend non-nullable `string`; frontend optional. |
| `IsActive` | `is_active` | ✅ | |
| `CreatedBy` | `created_by` | ✅ | |
| `CreatedAt` | `created_at` | ✅ | |

Note: the create endpoint (`POST /v1/benchmarks`) returns `{benchmark_set_id, version_number}`, but the frontend `createBenchmarkSet` is typed to return `BenchmarkSetView`. This is a route-level mismatch.

### `EvaluationRunSummary`

Backend struct (`backend/internal/evaluation/application/contracts.go:146`).
Frontend type: `EvaluationRunView` in `frontend/src/features/evaluations/evaluations.types.ts:14`.

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | ✅ | |
| `TenantID` | `tenant_id` | ✅ | |
| `AgentVersionID` | `agent_version_id` | ✅ | |
| `BenchmarkSetID` | `benchmark_set_id` | ✅ | |
| `Status` | `status` | ✅ | `domain.EvaluationRunStatus` (`running`, `passed`, `failed`) aligns with frontend `EvaluationRunStatus`. |
| `EnvironmentDigest` | `environment_digest` | ✅ | |
| `ScoringRuleVersion` | `scoring_rule_version` | ✅ | |
| `StartedAt` | `started_at` | ✅ | |
| `CompletedAt` | `completed_at?` | ✅ | `*time.Time` → optional `string`. |
| — | `threshold_results` | ⚠️ | Frontend `EvaluationRunView` requires `threshold_results: {name, passed, evidence?}[]`. `EvaluationRunSummary` omits it. |
| — | `summary` | ⚠️ | Frontend `EvaluationRunView` requires `summary: {pass_rate, avg_latency_ms, cost_cents, security_passed, extra?}`. `EvaluationRunSummary` omits it. |

The list endpoint (`GET /v1/evaluations`) returns `[]EvaluationRunSummary`, but the frontend `listEvaluationRuns` is typed as `EvaluationRunPage` containing full `EvaluationRunView` items. After snake_case tagging, the list will still be missing `threshold_results` and `summary`.

### `EvaluationRunDetail` (planned)

The design doc proposes a new application query DTO:

```go
type EvaluationRunDetail struct {
    ID                 string
    TenantID           string
    AgentVersionID     string
    BenchmarkSetID     string
    Status             string
    EnvironmentDigest  string
    ScoringRuleVersion string
    ThresholdResults   []domain.ThresholdResult
    Summary            domain.EvaluationSummary
    StartedAt          time.Time
    CompletedAt        *time.Time
}
```

This maps to frontend `EvaluationRunView` as follows:

| Backend field | Frontend field | Status | Notes |
|---------------|----------------|--------|-------|
| `ID` | `id` | 🔜/✅ | |
| `TenantID` | `tenant_id` | 🔜/✅ | |
| `AgentVersionID` | `agent_version_id` | 🔜/✅ | |
| `BenchmarkSetID` | `benchmark_set_id` | 🔜/✅ | |
| `Status` | `status` | 🔜/✅ | |
| `EnvironmentDigest` | `environment_digest` | 🔜/✅ | |
| `ScoringRuleVersion` | `scoring_rule_version` | 🔜/✅ | |
| `ThresholdResults` | `threshold_results` | 🔜/✅ | `[]ThresholdResult{Name, Passed, Evidence}` → `{name, passed, evidence?}[]`. |
| `Summary` | `summary` | 🔜/✅ | `EvaluationSummary{PassRate, AvgLatencyMs, CostCents, SecurityPassed, Extra}` → `{pass_rate, avg_latency_ms, cost_cents, security_passed, extra?}`. |
| `StartedAt` | `started_at` | 🔜/✅ | |
| `CompletedAt` | `completed_at?` | 🔜/✅ | |

---

## 5. Additional route-level mismatches (not fixed by JSON tags alone)

These endpoints return hand-written `map[string]any` shapes that differ from the frontend API types. They need handler-level changes, not just struct tags.

| Endpoint | Backend response shape | Frontend expected type | Mismatch |
|----------|------------------------|------------------------|----------|
| `POST /v1/agents/{id}/versions` | `{version_id, version_number, status}` | `versions.api.ts` types `{version_id, version_number, status}` | ✅ Actually matches the frontend call signature. |
| `POST /v1/agents/{id}/versions/{version_id}/evaluations` | `{evaluation_run_id, status}` | `versions.api.ts` types `{evaluation_run_id: string}` | ⚠️ Backend includes extra `status`; frontend ignores it. Harmless. |
| `POST /v1/agents/{id}/experiences` | `{experience_id, status, sensitivity_class}` | `ExperienceCandidateView` | ⚠️ Frontend expects full candidate view; backend returns only ids/status. |
| `POST /v1/benchmarks` | `{benchmark_set_id, version_number}` | `BenchmarkSetView` | ⚠️ Frontend expects full benchmark set view; backend returns ids only. |
| `GET /v1/evaluations/{id}` | `EvaluationRunSummary` (today) | `EvaluationRunView` | ⚠️ Missing `threshold_results` and `summary`; design doc fixes this with `EvaluationRunDetail`. |
| `GET /v1/agents/{id}/versions` | `[]VersionSummary` | `VersionPage` of `VersionView` | ⚠️ Summaries are missing required `VersionView` fields. |

---

## 6. Summary of required changes for alignment

1. Add `json:"snake_case,omitempty"` tags to every exported field in the application view structs listed in sections 1–4.
2. For `VersionDiff`, implement the REST-layer DTO (`versionDiffView` / `refChangeView`) described in section 2 and update `agent_version_router.go` `diffAgentVersion` to return it.
3. Implement `EvaluationRunDetail` and `GetEvaluationRunDetail` as described in section 4, and update `evaluation_router.go` `getEvaluation` to return it.
4. Decide how to handle `AgentVersionView` embedded in `AgentView.CurrentVersion`: either the console `AgentView` should gain a `current_version?: VersionView` field, or the backend should not embed a full version view there.
5. Add a frontend type for `AccessTokenView` / `ActivationStatusView`, or explicitly document that the console does not consume these endpoints.
6. Consider whether `VersionSummary` should be expanded or the frontend should define a slimmer `VersionSummary` type for list responses, because the current list response is missing many frontend-required fields.
7. Align create-experience and create-benchmark route responses with the frontend API expectations, or adjust the frontend API types to match the minimal backend responses.
