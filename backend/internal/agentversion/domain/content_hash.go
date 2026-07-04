package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ComputeContentHash returns a canonical SHA256 content-addressed hash for the
// immutable configuration references. The inputs are normalized (arrays are
// sorted) so equivalent configurations produce identical hashes.
func ComputeContentHash(
	runtime, model string,
	capabilities []string,
	promptRef string,
	skillRefs []string,
	memoryRef string,
	toolRefs []string,
) string {
	caps := sortedCopy(capabilities)
	skills := sortedCopy(skillRefs)
	tools := sortedCopy(toolRefs)

	parts := []string{
		runtime,
		model,
		strings.Join(caps, ","),
		promptRef,
		strings.Join(skills, ","),
		memoryRef,
		strings.Join(tools, ","),
	}
	joined := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(joined))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ComputeConfigFingerprint returns a short, stable fingerprint used for quick
// change detection. It reuses the first 16 hex characters of the content hash.
func ComputeConfigFingerprint(
	runtime, model string,
	capabilities []string,
	promptRef string,
	skillRefs []string,
	memoryRef string,
	toolRefs []string,
) string {
	hash := ComputeContentHash(runtime, model, capabilities, promptRef, skillRefs, memoryRef, toolRefs)
	// "sha256:" is 7 characters; take the next 16 hex digits.
	if len(hash) > 7+16 {
		return hash[7 : 7+16]
	}
	return hash
}

func sortedCopy(values []string) []string {
	if values == nil {
		return []string{}
	}
	copy := append([]string(nil), values...)
	sort.Strings(copy)
	return copy
}
