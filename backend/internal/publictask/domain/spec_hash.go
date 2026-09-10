package domain

import (
	"crypto/sha256"
	"encoding/hex"

	canonicaljson "github.com/gibson042/canonicaljson-go"
)

// specDocument 是参与规格摘要计算的字段白名单。它刻意只包含定义"要做什么、
// 做到什么算完成"的部分：标题、诊断、方案等描述性字段变化不应改变契约身份，
// 但基线 commit、验收标准和非目标变化必须改变。
//
// 字段顺序无关——canonical JSON 按键排序，因此第三方可以用公开字段独立复算
// 出同一个 spec_hash。
type specDocument struct {
	TaskSpecificationVersionID string                `json:"task_specification_version_id"`
	CanonicalRepository        string                `json:"canonical_repository"`
	SourceIssueURL             string                `json:"source_issue_url"`
	IssueRevision              string                `json:"issue_revision"`
	BaseCommit                 string                `json:"base_commit"`
	AcceptanceCriteria         []AcceptanceCriterion `json:"acceptance_criteria"`
	Constraints                []string              `json:"constraints"`
	NonGoals                   []string              `json:"non_goals"`
	DifficultyClass            string                `json:"difficulty_class"`
}

// ComputeSpecHash 返回任务规格的规范化 sha256 摘要（小写 hex）。
func (p Projection) ComputeSpecHash() (string, error) {
	document := specDocument{
		TaskSpecificationVersionID: p.TaskSpecificationVersionID,
		CanonicalRepository:        p.CanonicalRepository,
		SourceIssueURL:             p.SourceIssueURL,
		IssueRevision:              p.IssueRevision,
		BaseCommit:                 p.BaseCommit,
		AcceptanceCriteria:         p.AcceptanceCriteria,
		Constraints:                emptyIfNil(p.Constraints),
		NonGoals:                   emptyIfNil(p.NonGoals),
		DifficultyClass:            p.DifficultyClass,
	}
	if document.AcceptanceCriteria == nil {
		document.AcceptanceCriteria = []AcceptanceCriterion{}
	}
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// emptyIfNil 让 nil 与空切片产生同一个摘要，避免调用方的表示差异改变契约身份。
func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
