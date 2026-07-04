package github_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/github"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestNewDriverRequiresConfig(t *testing.T) {
	key, pem := newRSAKey(t)
	_ = key

	_, err := github.NewDriver(github.Config{})
	require.Error(t, err)

	_, err = github.NewDriver(github.Config{AppID: 1, InstallationID: 2, PrivateKey: pem})
	require.NoError(t, err)
}

func TestCreateCredentialReturnsTokenAndMetadata(t *testing.T) {
	key, pem := newRSAKey(t)
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(1 * time.Hour)

	var tokenReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			tokenReq = r
			auth := r.Header.Get("Authorization")
			require.True(t, strings.HasPrefix(auth, "Bearer "), "expected bearer token")
			verifyGitHubAppJWT(t, auth[7:], key, now)

			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"token":"ghs_installation_token","expires_at":"%s"}`, expiresAt.Format(time.RFC3339))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	d, err := github.NewDriver(github.Config{
		AppID:          42,
		InstallationID: 123,
		PrivateKey:     pem,
		BaseURL:        srv.URL,
	}, github.WithClock(func() time.Time { return now }))
	require.NoError(t, err)

	cred, err := d.CreateCredential(context.Background(), "owner/repo", "main", "abc123")
	require.NoError(t, err)
	require.Equal(t, "ghs_installation_token", cred.Token)
	require.Equal(t, "main", cred.Branch)
	require.Equal(t, "abc123", cred.BaseCommit)
	require.Equal(t, expiresAt, cred.ExpiresAt)
	require.True(t, strings.HasSuffix(cred.RepoURL, "/owner/repo.git"))
	require.NotNil(t, tokenReq)
}

func TestCreateCredentialRejectsInvalidRepo(t *testing.T) {
	_, pem := newRSAKey(t)
	d, err := github.NewDriver(github.Config{
		AppID:          42,
		InstallationID: 123,
		PrivateKey:     pem,
		BaseURL:        "https://api.github.com",
	})
	require.NoError(t, err)

	_, err = d.CreateCredential(context.Background(), "not-a-repo", "main", "abc")
	require.ErrorIs(t, err, git.ErrInvalidRepo)
}

func TestGetCommitReturnsCommit(t *testing.T) {
	key, pem := newRSAKey(t)
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/commits/abc123": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{
				"sha": "abc123",
				"commit": {
					"message": "initial commit",
					"author": {"name": "Ada", "email": "ada@example.com", "date": "2026-07-04T10:00:00Z"}
				},
				"html_url": "https://github.com/owner/repo/commit/abc123"
			}`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	commit, err := d.GetCommit(context.Background(), "owner/repo", "abc123")
	require.NoError(t, err)
	require.Equal(t, "abc123", commit.SHA)
	require.Equal(t, "initial commit", commit.Message)
	require.Equal(t, "Ada", commit.Author)
	require.Equal(t, "ada@example.com", commit.Email)
	require.Equal(t, time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC), commit.CommittedAt)
	require.Equal(t, "https://github.com/owner/repo/commit/abc123", commit.URL)
}

func TestGetCommitReturnsNotFound(t *testing.T) {
	key, pem := newRSAKey(t)
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/commits/missing": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	_, err := d.GetCommit(context.Background(), "owner/repo", "missing")
	require.ErrorIs(t, err, git.ErrRepoNotFound)
}

func TestCompareCommitsReturnsFiles(t *testing.T) {
	key, pem := newRSAKey(t)
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/compare/base...head": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{
				"status": "ahead",
				"ahead_by": 2,
				"behind_by": 0,
				"files": [
					{"filename":"README.md","status":"modified","additions":3,"deletions":1,"patch":"@@ -1 +1,3 @@"}
				]
			}`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	files, err := d.CompareCommits(context.Background(), "owner/repo", "base", "head")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "README.md", files[0].Filename)
	require.Equal(t, "modified", files[0].Status)
	require.Equal(t, 3, files[0].Additions)
	require.Equal(t, 1, files[0].Deletions)
	require.Equal(t, "@@ -1 +1,3 @@", files[0].Patch)
}

func TestIsAncestor(t *testing.T) {
	key, pem := newRSAKey(t)
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/compare/v1...v2": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"status":"ahead","ahead_by":5,"behind_by":0,"files":[]}`)
		},
		"/repos/owner/repo/compare/v1...v1": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"status":"identical","ahead_by":0,"behind_by":0,"files":[]}`)
		},
		"/repos/owner/repo/compare/v2...v1": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"status":"behind","ahead_by":0,"behind_by":5,"files":[]}`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)

	ok, err := d.IsAncestor(context.Background(), "owner/repo", "v1", "v2")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = d.IsAncestor(context.Background(), "owner/repo", "v1", "v1")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = d.IsAncestor(context.Background(), "owner/repo", "v2", "v1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestRateLimitedReturnsDomainError(t *testing.T) {
	key, pem := newRSAKey(t)
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/commits/abc": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "60")
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	_, err := d.GetCommit(context.Background(), "owner/repo", "abc")
	require.ErrorIs(t, err, domain.ErrRateLimited)
	require.Equal(t, "rate_limited", domain.CodeOf(err))
	require.Equal(t, 60*time.Second, domain.RetryAfterOf(err))
}

func newDriver(t *testing.T, pem, baseURL string) *github.Driver {
	t.Helper()
	d, err := github.NewDriver(github.Config{
		AppID:          42,
		InstallationID: 123,
		PrivateKey:     pem,
		BaseURL:        baseURL,
	})
	require.NoError(t, err)
	return d
}

func newGitHubServer(t *testing.T, key *rsa.PrivateKey, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			auth := r.Header.Get("Authorization")
			require.True(t, strings.HasPrefix(auth, "Bearer "))
			verifyGitHubAppJWT(t, auth[7:], key, time.Now())
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"token":"ghs_token","expires_at":"%s"}`, time.Now().Add(time.Hour).Format(time.RFC3339))
			return
		}
		if h, ok := handlers[r.URL.Path]; ok {
			h(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func newRSAKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return key, string(pemBytes)
}

func verifyGitHubAppJWT(t *testing.T, raw string, key *rsa.PrivateKey, now time.Time) {
	t.Helper()
	token, err := jwt.Parse(raw, func(_ *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	require.NoError(t, err)
	require.True(t, token.Valid)
	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)
	require.Equal(t, float64(42), claims["iss"])
	exp, err := claims.GetExpirationTime()
	require.NoError(t, err)
	require.True(t, exp.After(now.Add(9*time.Minute)))
}
