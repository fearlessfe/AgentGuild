package rest

import (
	"encoding/json"
	"strings"
	"testing"

	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	"github.com/stretchr/testify/require"
)

func TestToVersionDiffViewConversionInternal(t *testing.T) {
	diff := &agentversionapp.VersionDiff{
		BaseVersionID: "base-1",
		Added: map[string]agentversionapp.RefChange{
			"capability:go":      {},
			"skill:lint":         {},
			"tool:unused":        {},
			"unprefixed:ignored": {},
		},
		Removed: map[string]agentversionapp.RefChange{
			"capability:python": {},
		},
		Changed: map[string]agentversionapp.RefChange{
			"prompt_ref": {From: "old-prompt", To: "new-prompt"},
			"memory_ref": {From: "", To: "new-memory"},
		},
	}

	view := toVersionDiffView("target-1", diff)

	require.Equal(t, "base-1", view.BaseVersionID)
	require.Equal(t, "target-1", view.TargetVersionID)
	require.Equal(t, []string{"go", "lint", "unused"}, view.AddedCapabilities)
	require.Equal(t, []string{"python"}, view.RemovedCapabilities)
	require.Len(t, view.ChangedRefs, 2)
	require.Equal(t, "memory_ref", view.ChangedRefs[0].Field)
	require.Nil(t, view.ChangedRefs[0].From)
	require.NotNil(t, view.ChangedRefs[0].To)
	require.Equal(t, "new-memory", *view.ChangedRefs[0].To)
	require.Equal(t, "prompt_ref", view.ChangedRefs[1].Field)
	require.NotNil(t, view.ChangedRefs[1].From)
	require.Equal(t, "old-prompt", *view.ChangedRefs[1].From)
	require.NotNil(t, view.ChangedRefs[1].To)
	require.Equal(t, "new-prompt", *view.ChangedRefs[1].To)

	bytes, err := json.Marshal(view)
	require.NoError(t, err)
	wire := string(bytes)
	require.Contains(t, wire, `"base_version_id":"base-1"`)
	require.Contains(t, wire, `"target_version_id":"target-1"`)
	require.Contains(t, wire, `"added_capabilities":["go","lint","unused"]`)
	require.Contains(t, wire, `"removed_capabilities":["python"]`)
	require.Contains(t, wire, `"changed_refs":[`)
	require.Contains(t, wire, `"field":"memory_ref"`)
	require.Contains(t, wire, `"field":"prompt_ref"`)
	require.NotContains(t, strings.ToLower(wire), "baseversionid")
	require.NotContains(t, strings.ToLower(wire), "targetversionid")
	require.NotContains(t, strings.ToLower(wire), "addedcapabilities")
}
