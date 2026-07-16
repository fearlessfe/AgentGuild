package github

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateBaseURLRequiresHTTPSAndDeploymentAllowlist(t *testing.T) {
	got, err := ValidateBaseURL("https://ghe.example/api/v3/", []string{"ghe.example"})
	require.NoError(t, err)
	require.Equal(t, "https://ghe.example/api/v3", got)

	for _, raw := range []string{
		"http://ghe.example/api/v3",
		"https://user:pass@ghe.example/api/v3",
		"https://127.0.0.1/api/v3",
		"file:///tmp/github",
	} {
		_, err := ValidateBaseURL(raw, []string{"ghe.example"})
		require.Error(t, err, raw)
	}
}

func TestValidateBaseURLAllowsPrivateEnterpriseOnlyWhenExplicitlyTrusted(t *testing.T) {
	_, err := ValidateBaseURL("https://ghe.internal/api/v3", []string{"api.github.com"})
	require.Error(t, err)

	got, err := ValidateBaseURL("https://ghe.internal/api/v3", []string{"ghe.internal"})
	require.NoError(t, err)
	require.Equal(t, "https://ghe.internal/api/v3", got)
}
