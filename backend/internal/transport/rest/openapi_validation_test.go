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
