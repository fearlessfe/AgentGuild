package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWKSVerifier 通过 JWKS 端点验证 OAuth Agent Access Token，并把 claims 映射为 Principal。
// 它是 auth.TokenVerifier 的首个可替换实现：transport 层只依赖 TokenVerifier 接口，
// 后续切换身份模型时无需改动生命周期服务。
type JWKSVerifier struct {
	issuer   string
	audience string
	jwksURL  string
	client   *http.Client

	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

// NewJWKSVerifier 创建一个验证指定 issuer、audience 的 JWT verifier。
// client 为 nil 时使用带 10 秒超时和 TLS 校验的默认 HTTP client。
func NewJWKSVerifier(issuer, audience, jwksURL string, client *http.Client) *JWKSVerifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &JWKSVerifier{
		issuer:   issuer,
		audience: audience,
		jwksURL:  jwksURL,
		client:   client,
		keys:     make(map[string]*rsa.PublicKey),
	}
}

// Verify 解析并验证 rawToken，返回对应的 Principal。
func (v *JWKSVerifier) Verify(ctx context.Context, rawToken string) (Principal, error) {
	var principal Principal
	if rawToken == "" {
		return principal, errors.New("token is empty")
	}

	token, err := jwt.Parse(rawToken, v.keyFunc(ctx), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return principal, ErrTokenExpired
		}
		return principal, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return principal, errors.New("token is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return principal, errors.New("token claims are not map claims")
	}

	principal = Principal{
		TenantID:       stringClaim(claims, "tenant_id"),
		Type:           PrincipalTypeAgent,
		AgentID:        stringClaim(claims, "agent_id"),
		AgentVersionID: stringClaim(claims, "agent_version_id"),
		Scopes:         stringSliceClaim(claims, "scopes"),
		RepoScope:      stringSliceClaim(claims, "repo_scope"),
	}
	if principal.TenantID == "" || principal.AgentID == "" || principal.AgentVersionID == "" {
		return principal, errors.New("token is missing required identity claims")
	}
	return principal, nil
}

func (v *JWKSVerifier) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("token header missing kid")
		}

		v.mu.RLock()
		key, ok := v.keys[kid]
		v.mu.RUnlock()
		if ok {
			return key, nil
		}

		v.mu.Lock()
		defer v.mu.Unlock()
		// 双重检查，防止并发时重复刷新。
		if key, ok := v.keys[kid]; ok {
			return key, nil
		}
		if err := v.fetchKeys(ctx); err != nil {
			return nil, err
		}
		key, ok = v.keys[kid]
		if !ok {
			return nil, fmt.Errorf("key %q not found in JWKS", kid)
		}
		return key, nil
	}
}

func (v *JWKSVerifier) fetchKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	keys, err := parseJWKS(body)
	if err != nil {
		return err
	}
	for kid, key := range keys {
		v.keys[kid] = key
	}
	return nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

func parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var doc jwksDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, jwk := range doc.Keys {
		if strings.ToUpper(jwk.Kty) != "RSA" {
			continue
		}
		if jwk.Kid == "" || jwk.N == "" || jwk.E == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			return nil, fmt.Errorf("decode JWK n: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			return nil, fmt.Errorf("decode JWK e: %w", err)
		}
		pub := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
		keys[jwk.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("JWKS contains no usable RSA keys")
	}
	return keys, nil
}

func stringClaim(claims jwt.MapClaims, key string) string {
	value, ok := claims[key].(string)
	if !ok {
		return ""
	}
	return value
}

func stringSliceClaim(claims jwt.MapClaims, key string) []string {
	raw, ok := claims[key]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return splitScopeString(value)
	default:
		return nil
	}
}

func splitScopeString(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	// OAuth scope 字符串通常以空格分隔；为兼容也支持逗号。
	for _, sep := range []string{" ", ","} {
		if strings.Contains(value, sep) {
			parts := strings.Split(value, sep)
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}
	}
	return []string{value}
}
