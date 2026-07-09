package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// manifestRedirectURL is where GitHub renders the "create App from manifest"
// form. Only github.com is supported for now.
const manifestRedirectURL = "https://github.com/settings/apps/new"

// defaultConversionsBaseURL is the GitHub API host used to exchange a manifest
// code for App credentials.
const defaultConversionsBaseURL = "https://api.github.com"

const githubAppInstallBaseURL = "https://github.com/apps"

// stateTTL bounds how long a signed manifest state token stays valid.
const stateTTL = 15 * time.Minute

// ManifestOptions configures a ManifestService.
type ManifestOptions struct {
	// PublicBaseURL is this platform's externally reachable base URL. Callback
	// and setup URLs are derived from it.
	PublicBaseURL string
	// StateSecret signs the manifest state token (HMAC-SHA256).
	StateSecret []byte
	// ConversionsBaseURL overrides the GitHub API host for the manifest code
	// exchange. Defaults to https://api.github.com.
	ConversionsBaseURL string
	// HTTPClient is used for the conversions call. Defaults to
	// http.DefaultClient.
	HTTPClient *http.Client
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
}

// ManifestService orchestrates GitHub App creation via the manifest flow: it
// builds a manifest + signed state, verifies state on callback, and exchanges
// the resulting code for App credentials which it persists through the
// GitHubAppManager.
type ManifestService struct {
	apps               GitHubAppManager
	publicBaseURL      string
	stateSecret        []byte
	conversionsBaseURL string
	httpClient         *http.Client
	now                func() time.Time
}

// NewManifestService creates a ManifestService, applying option defaults.
func NewManifestService(apps GitHubAppManager, opts ManifestOptions) *ManifestService {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	base := opts.ConversionsBaseURL
	if base == "" {
		base = defaultConversionsBaseURL
	}
	return &ManifestService{
		apps:               apps,
		publicBaseURL:      strings.TrimRight(opts.PublicBaseURL, "/"),
		stateSecret:        opts.StateSecret,
		conversionsBaseURL: strings.TrimRight(base, "/"),
		httpClient:         client,
		now:                now,
	}
}

// manifestState is the signed payload embedded in the state token.
type manifestState struct {
	TenantID  string    `json:"tenant"`
	ExpiresAt time.Time `json:"exp"`
}

// gitHubAppManifest is the JSON body posted to GitHub's manifest form.
type gitHubAppManifest struct {
	Name               string            `json:"name"`
	URL                string            `json:"url"`
	RedirectURL        string            `json:"redirect_url"`
	SetupURL           string            `json:"setup_url"`
	Public             bool              `json:"public"`
	DefaultPermissions map[string]string `json:"default_permissions"`
}

// manifestConversion is the credential payload returned by the GitHub
// conversions endpoint.
type manifestConversion struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	PEM           string `json:"pem"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
}

// BuildManifest returns the manifest JSON, a signed state token, and the GitHub
// redirect URL for the manifest creation form.
func (m *ManifestService) BuildManifest(tenantID string) (string, string, string, error) {
	if tenantID == "" {
		return "", "", "", invalid("tenant_id")
	}
	manifest := gitHubAppManifest{
		Name:        "AgentGuild",
		URL:         m.publicBaseURL,
		RedirectURL: m.publicBaseURL + "/oauth/github/app/callback",
		SetupURL:    m.publicBaseURL + "/oauth/github/app/installed",
		Public:      false,
		DefaultPermissions: map[string]string{
			"contents": "read",
			"issues":   "write",
			"checks":   "read",
			"metadata": "read",
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return "", "", "", err
	}
	state := m.signState(manifestState{
		TenantID:  tenantID,
		ExpiresAt: m.now().Add(stateTTL),
	})
	return string(body), state, manifestRedirectURL, nil
}

// BuildInstallURL returns the GitHub URL for installing an already-created App.
func (m *ManifestService) BuildInstallURL(tenantID, appSlug string) (string, error) {
	if tenantID == "" {
		return "", invalid("tenant_id")
	}
	appSlug = strings.TrimSpace(appSlug)
	if appSlug == "" {
		return "", invalid("app_slug")
	}
	state := m.signState(manifestState{
		TenantID:  tenantID,
		ExpiresAt: m.now().Add(stateTTL),
	})
	return fmt.Sprintf("%s/%s/installations/new?state=%s", githubAppInstallBaseURL, appSlug, state), nil
}

// VerifyState validates a state token against the given tenant: HMAC must be
// valid, the tenant must match, and the token must not be expired.
func (m *ManifestService) VerifyState(state, tenantID string) error {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return invalid("state")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return invalid("state")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return invalid("state")
	}
	mac := hmac.New(sha256.New, m.stateSecret)
	_, _ = mac.Write(body)
	var payload manifestState
	if !hmac.Equal(signature, mac.Sum(nil)) ||
		json.Unmarshal(body, &payload) != nil ||
		payload.TenantID == "" ||
		payload.TenantID != tenantID ||
		!m.now().Before(payload.ExpiresAt) {
		return invalid("state")
	}
	return nil
}

// ExchangeCode exchanges a manifest code for App credentials via the GitHub
// conversions API, persists them, and returns the resulting public view.
func (m *ManifestService) ExchangeCode(ctx context.Context, tenantID, code string) (GitHubAppView, error) {
	if tenantID == "" {
		return GitHubAppView{}, invalid("tenant_id")
	}
	if code == "" {
		return GitHubAppView{}, invalid("code")
	}

	conversion, err := m.exchange(ctx, code)
	if err != nil {
		return GitHubAppView{}, err
	}

	if err := m.apps.Upsert(ctx, UpsertGitHubApp{
		TenantID:       tenantID,
		AppID:          conversion.ID,
		InstallationID: 0,
		PrivateKey:     conversion.PEM,
		BaseURL:        defaultConversionsBaseURL,
		WebhookSecret:  conversion.WebhookSecret,
		ClientID:       conversion.ClientID,
		ClientSecret:   conversion.ClientSecret,
		AppSlug:        conversion.Slug,
	}); err != nil {
		return GitHubAppView{}, err
	}

	return m.apps.Get(ctx, tenantID)
}

// exchange performs the POST to GitHub's conversions endpoint.
func (m *ManifestService) exchange(ctx context.Context, code string) (manifestConversion, error) {
	var conversion manifestConversion

	url := fmt.Sprintf("%s/app-manifests/%s/conversions", m.conversionsBaseURL, code)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return conversion, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return conversion, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return conversion, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return conversion, fmt.Errorf("github manifest conversion failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, &conversion); err != nil {
		return conversion, err
	}
	if conversion.ID == 0 {
		return conversion, invalid("app_id")
	}
	return conversion, nil
}

// signState signs a manifest state payload using the HMAC + base64 pattern.
func (m *ManifestService) signState(payload manifestState) string {
	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, m.stateSecret)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
