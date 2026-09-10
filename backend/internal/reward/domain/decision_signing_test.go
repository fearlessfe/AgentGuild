package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

const testDeploymentSecret = "0123456789abcdef0123456789abcdef"

// 奖励决定的签名密钥必须与部署密钥本身不同：CURSOR_SECRET 同时为发给每一个
// 匿名调用方的分页游标签名，直接复用会让游标侧的泄露等同于资金完整性被攻破。
func TestSigningKeyIsDerivedNotTheDeploymentSecret(t *testing.T) {
	signer, err := NewHMACSigner([]byte(testDeploymentSecret))
	require.NoError(t, err)

	hash := hex.EncodeToString(sha256.New().Sum(nil))
	signature, err := signer.Sign(hash)
	require.NoError(t, err)

	raw := hmac.New(sha256.New, []byte(testDeploymentSecret))
	_, _ = raw.Write([]byte(hash))
	require.NotEqual(t, hex.EncodeToString(raw.Sum(nil)), signature,
		"签名不得等于用部署密钥直接 HMAC 的结果")

	require.True(t, signer.Verify(hash, signature))
	require.False(t, signer.Verify(hash, hex.EncodeToString(raw.Sum(nil))))
}

func TestSigningIsDeterministicAcrossSignerInstances(t *testing.T) {
	first, err := NewHMACSigner([]byte(testDeploymentSecret))
	require.NoError(t, err)
	second, err := NewHMACSigner([]byte(testDeploymentSecret))
	require.NoError(t, err)

	hash := hex.EncodeToString(sha256.New().Sum(nil))
	a, err := first.Sign(hash)
	require.NoError(t, err)
	b, err := second.Sign(hash)
	require.NoError(t, err)
	require.Equal(t, a, b, "同一密钥必须产生同一签名，否则历史决定无法复验")
}

func TestSignerRejectsShortSecretAndNonHashInput(t *testing.T) {
	_, err := NewHMACSigner([]byte("too-short"))
	require.Error(t, err)

	signer, err := NewHMACSigner([]byte(testDeploymentSecret))
	require.NoError(t, err)
	_, err = signer.Sign("not-a-hash")
	require.Error(t, err)
}
