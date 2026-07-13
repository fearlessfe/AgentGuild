package rest_test

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// TestOpenAPISpecIsValid 在 CI 或离线环境下等价校验 openapi.yaml。
func TestOpenAPISpecIsValid(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("openapi.yaml")
	require.NoError(t, err)
	err = doc.Validate(context.Background())
	require.NoError(t, err)
}

func TestOpenAPIDocumentsPluralGitHubAppRoutes(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("openapi.yaml")
	require.NoError(t, err)

	list := doc.Paths.Value("/v1/github-apps")
	require.NotNil(t, list)
	require.NotNil(t, list.Get)

	item := doc.Paths.Value("/v1/github-apps/{id}")
	require.NotNil(t, item)
	require.NotNil(t, item.Get)
	require.NotNil(t, item.Delete)
	require.NotNil(t, item.Delete.Responses.Value("409"))

	testConnection := doc.Paths.Value("/v1/github-apps/{id}:test")
	require.NotNil(t, testConnection)
	require.NotNil(t, testConnection.Post)

	publicRepository := doc.Paths.Value("/v1/repositories/public")
	require.NotNil(t, publicRepository.Post.Responses.Value("409"))

	appView := doc.Components.Schemas["GitHubAppPublicView"].Value
	for _, property := range []string{"id", "installation_account_login", "is_default"} {
		require.Contains(t, appView.Properties, property)
	}
	require.Contains(t, doc.Components.Schemas["RepositoryItem"].Value.Properties, "github_app_id")
}
