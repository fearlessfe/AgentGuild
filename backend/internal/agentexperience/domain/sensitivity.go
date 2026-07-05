package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// SensitivityClass classifies the disclosure risk of an experience candidate.
type SensitivityClass string

const (
	SensitivityPublic      SensitivityClass = "public"
	SensitivityInternal    SensitivityClass = "internal"
	SensitivityRestricted  SensitivityClass = "restricted"
	SensitivityForbidden   SensitivityClass = "forbidden"
)

// SensitivityPolicy classifies evidence content. Implementations may use
// rule-based matching today and can be replaced by an NLP pipeline later
// without changing the domain model.
type SensitivityPolicy interface {
	Classify(evidence []byte) (SensitivityClass, string)
}

// RuleBasedSensitivityPolicy is the initial rule-based classifier. It scans
// evidence for patterns commonly associated with secrets, credentials, or
// restricted data and returns the most restrictive class found.
type RuleBasedSensitivityPolicy struct{}

// NewRuleBasedSensitivityPolicy creates a default rule-based policy.
func NewRuleBasedSensitivityPolicy() *RuleBasedSensitivityPolicy {
	return &RuleBasedSensitivityPolicy{}
}

// forbiddenPatterns detect content that must never be turned into experience.
var forbiddenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)password\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)secret\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)api[_-]?key\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)private[_-]?key\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)token\s*[:=]\s*[a-zA-Z0-9_\-]{16,}`),
	regexp.MustCompile(`(?i)aws[_-]?secret[_-]?access[_-]?key\s*[:=]\s*\S+`),
}

// restrictedPatterns detect content that is likely PII or business-sensitive.
var restrictedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ssn\s*[:=]?\s*\d{3}-\d{2}-\d{4}`),
	regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`), // credit card
	regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),  // email
}

// internalPatterns detect content that should remain inside the tenant boundary.
var internalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(internal|confidential|proprietary)`),
}

func (p *RuleBasedSensitivityPolicy) Classify(evidence []byte) (SensitivityClass, string) {
	lower := bytes.ToLower(evidence)
	for _, re := range forbiddenPatterns {
		if loc := re.FindIndex(evidence); loc != nil {
			snippet := snippetAround(evidence, loc[0], loc[1])
			return SensitivityForbidden, "forbidden pattern detected: " + snippet
		}
	}
	for _, re := range restrictedPatterns {
		if loc := re.FindIndex(evidence); loc != nil {
			snippet := snippetAround(evidence, loc[0], loc[1])
			return SensitivityRestricted, "restricted pattern detected: " + snippet
		}
	}
	for _, re := range internalPatterns {
		if loc := re.FindIndex(lower); loc != nil {
			snippet := snippetAround(evidence, loc[0], loc[1])
			return SensitivityInternal, "internal pattern detected: " + snippet
		}
	}
	return SensitivityPublic, ""
}

func snippetAround(data []byte, start, end int) string {
	const width = 24
	s := start - width
	if s < 0 {
		s = 0
	}
	e := end + width
	if e > len(data) {
		e = len(data)
	}
	return strings.TrimSpace(string(data[s:e]))
}

// ComputeContentHash returns a SHA256 content-addressed hash for a byte slice.
func ComputeContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
