package rest_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestOpenRegistrationRoutesRequireNoExistingIdentity(t *testing.T) {
	publicKey := make([]byte, 32)
	signature := make([]byte, 64)
	service := &fakeOpenRegistrationService{
		challenge: identityapp.Envelope[identityapp.RegistrationChallengeView]{Data: identityapp.RegistrationChallengeView{
			ChallengeID: "challenge-1", Nonce: "nonce-1", ExpiresAt: time.Now().Add(time.Minute),
		}},
		registration: identityapp.Envelope[identityapp.OpenRegistrationResponse]{Data: identityapp.OpenRegistrationResponse{
			AgentID: "agent-1", AgentVersionID: "version-1", OrganizationID: "public",
			AccessToken: "access-1", TokenType: "Bearer", TrustLevel: "unverified",
		}},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{}, rest.WithOpenRegistrationService(service)).Router()
	encodedKey := base64.RawURLEncoding.EncodeToString(publicKey)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents:registration-challenge", strings.NewReader(`{"public_key":"`+encodedKey+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, publicKey, service.lastChallenge.PublicKey)

	body := `{"challenge_id":"challenge-1","public_key":"` + encodedKey + `","signature":"` + base64.RawURLEncoding.EncodeToString(signature) + `","runtime":"codex","model":"gpt-5"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/agents:register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "register-1")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), "access-1")
	require.Equal(t, "challenge-1", service.lastRegistration.ChallengeID)
	require.Equal(t, "codex", service.lastRegistration.Runtime)
}

func TestOpenRegistrationRejectsMalformedPublicKeyEncoding(t *testing.T) {
	service := &fakeOpenRegistrationService{}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{}, rest.WithOpenRegistrationService(service)).Router()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents:registration-challenge", strings.NewReader(`{"public_key":"not base64!"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, service.lastChallenge.PublicKey)
}

func TestOpenRegistrationChallengeIsRateLimitedBySocketSource(t *testing.T) {
	publicKey := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	service := &fakeOpenRegistrationService{challenge: identityapp.Envelope[identityapp.RegistrationChallengeView]{}}
	limiter := &perKeyBudgetRateLimiter{budget: 1}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithOpenRegistrationService(service), rest.WithRateLimiter(limiter),
	).Router()

	request := func(remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/agents:registration-challenge", strings.NewReader(`{"public_key":"`+publicKey+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	require.Equal(t, http.StatusOK, request("203.0.113.10:1234").Code)
	require.Equal(t, http.StatusTooManyRequests, request("203.0.113.10:5678").Code)
	require.Equal(t, http.StatusOK, request("203.0.113.11:1234").Code)
}

func TestGlobalAgentRefreshUsesOpenRegistrationIdentity(t *testing.T) {
	service := &fakeOpenRegistrationService{refresh: identityapp.Envelope[identityapp.AccessTokenView]{Data: identityapp.AccessTokenView{
		Token: "refreshed-global-token", TokenType: "Bearer", AgentID: "global-agent", AgentVersionID: "global-version",
	}}}
	principal := auth.Principal{
		SubjectID: auth.AgentSubject("global-agent"), IdentityScope: auth.IdentityScopeGlobal,
		Type: auth.PrincipalTypeAgent, AgentID: "global-agent", AgentVersionID: "global-version",
	}
	server := rest.NewServer(&fakeApplication{}, &fakeVerifier{principal: principal},
		rest.WithIdentityService(&fakeIdentityApplication{}), rest.WithOpenRegistrationService(service),
	).Router()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/me:refresh", nil)
	req.Header.Set("Authorization", "Bearer global-token")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "refreshed-global-token")
}

type fakeOpenRegistrationService struct {
	challenge        identityapp.Envelope[identityapp.RegistrationChallengeView]
	challengeErr     error
	registration     identityapp.Envelope[identityapp.OpenRegistrationResponse]
	registrationErr  error
	lastChallenge    identityapp.CreateRegistrationChallenge
	lastRegistration identityapp.OpenRegisterAgent
	refresh          identityapp.Envelope[identityapp.AccessTokenView]
	refreshErr       error
	self             identityapp.Envelope[identityapp.GlobalAgentView]
	selfErr          error
}

func (s *fakeOpenRegistrationService) CreateChallenge(_ context.Context, command identityapp.CreateRegistrationChallenge) (identityapp.Envelope[identityapp.RegistrationChallengeView], error) {
	s.lastChallenge = command
	return s.challenge, s.challengeErr
}

func (s *fakeOpenRegistrationService) Register(_ context.Context, command identityapp.OpenRegisterAgent) (identityapp.Envelope[identityapp.OpenRegistrationResponse], error) {
	s.lastRegistration = command
	return s.registration, s.registrationErr
}

func (s *fakeOpenRegistrationService) Refresh(_ context.Context, _, _ string) (identityapp.Envelope[identityapp.AccessTokenView], error) {
	return s.refresh, s.refreshErr
}

func (s *fakeOpenRegistrationService) GetSelf(_ context.Context, _, _ string) (identityapp.Envelope[identityapp.GlobalAgentView], error) {
	return s.self, s.selfErr
}

func (s *fakeOpenRegistrationService) Heartbeat(_ context.Context, _, _ string) (identityapp.Envelope[identityapp.GlobalAgentView], error) {
	return s.self, s.selfErr
}
