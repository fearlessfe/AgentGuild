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
	"net/url"
	"strings"
	"time"
)

// defaultConversionsBaseURL is the GitHub API host used to exchange a manifest
// code for App credentials.
const defaultConversionsBaseURL = "https://api.github.com"

const defaultGitHubWebBaseURL = "https://github.com"

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
	// WebBaseURL overrides the GitHub web host used to create and install Apps.
	// When empty it is derived from ConversionsBaseURL, stripping the standard
	// GitHub Enterprise Server /api/v3 suffix.
	WebBaseURL string
	// HTTPClient is used for the conversions call. Defaults to
	// http.DefaultClient.
	HTTPClient *http.Client
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
	// NewID allocates the local GitHub App ID embedded in signed state.
	NewID func() string
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
	webBaseURL         string
	httpClient         *http.Client
	now                func() time.Time
	newID              func() string
}

// NewManifestService creates a ManifestService, applying option defaults.
func NewManifestService(apps GitHubAppManager, opts ManifestOptions) (*ManifestService, error) {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	conversionsBaseURL, err := normalizeGitHubBaseURL(opts.ConversionsBaseURL, defaultConversionsBaseURL)
	if err != nil {
		return nil, fmt.Errorf("conversions base URL: %w", err)
	}
	webBaseURL := opts.WebBaseURL
	if webBaseURL == "" {
		webBaseURL = deriveGitHubWebBaseURL(conversionsBaseURL)
	}
	webBaseURL, err = normalizeGitHubBaseURL(webBaseURL, defaultGitHubWebBaseURL)
	if err != nil {
		return nil, fmt.Errorf("web base URL: %w", err)
	}
	newID := opts.NewID
	if newID == nil {
		newID = randomID
	}
	return &ManifestService{
		apps:               apps,
		publicBaseURL:      strings.TrimRight(opts.PublicBaseURL, "/"),
		stateSecret:        opts.StateSecret,
		conversionsBaseURL: conversionsBaseURL,
		webBaseURL:         webBaseURL,
		httpClient:         client,
		now:                now,
		newID:              newID,
	}, nil
}

func normalizeGitHubBaseURL(raw, fallback string) (string, error) {
	if raw == "" {
		raw = fallback
	}
	if raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "?#") {
		return "", fmt.Errorf("must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", fmt.Errorf("must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("must not contain dot path segments")
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func deriveGitHubWebBaseURL(apiBaseURL string) string {
	if apiBaseURL == defaultConversionsBaseURL {
		return defaultGitHubWebBaseURL
	}
	parsed, err := url.Parse(apiBaseURL)
	if err != nil {
		return defaultGitHubWebBaseURL
	}
	webPath := ""
	if strings.HasSuffix(parsed.Path, "/api/v3") {
		webPath = strings.TrimSuffix(parsed.Path, "/api/v3")
	}
	parsed.Path = webPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// ManifestState is the trusted payload recovered from a signed state token.
type ManifestState struct {
	TenantID    string    `json:"tenant"`
	GitHubAppID string    `json:"github_app_id"`
	ExpiresAt   time.Time `json:"exp"`
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
	githubAppID := m.newID()
	if githubAppID == "" {
		return "", "", "", invalid("github_app_id")
	}
	state := m.signState(ManifestState{
		TenantID:    tenantID,
		GitHubAppID: githubAppID,
		ExpiresAt:   m.now().Add(stateTTL),
	})
	return string(body), state, m.webBaseURL + "/settings/apps/new", nil
}

// BuildInstallURL returns the GitHub URL for installing an already-created App.
func (m *ManifestService) BuildInstallURL(tenantID, githubAppID, appSlug string) (string, error) {
	if tenantID == "" {
		return "", invalid("tenant_id")
	}
	if githubAppID == "" {
		return "", invalid("github_app_id")
	}
	appSlug = strings.TrimSpace(appSlug)
	if appSlug == "" {
		return "", invalid("app_slug")
	}
	state := m.signState(ManifestState{
		TenantID:    tenantID,
		GitHubAppID: githubAppID,
		ExpiresAt:   m.now().Add(stateTTL),
	})
	return fmt.Sprintf("%s/apps/%s/installations/new?state=%s", m.webBaseURL, url.PathEscape(appSlug), state), nil
}

// VerifyState validates a state token against the given tenant: HMAC must be
// valid, the tenant must match, and the token must not be expired.
func (m *ManifestService) VerifyState(state, tenantID string) (ManifestState, error) {
	var payload ManifestState
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return payload, invalid("state")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return payload, invalid("state")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return payload, invalid("state")
	}
	mac := hmac.New(sha256.New, m.stateSecret)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) ||
		json.Unmarshal(body, &payload) != nil ||
		payload.TenantID == "" ||
		payload.GitHubAppID == "" ||
		payload.TenantID != tenantID ||
		!m.now().Before(payload.ExpiresAt) {
		return ManifestState{}, invalid("state")
	}
	return payload, nil
}

// ExchangeCode exchanges a manifest code for App credentials via the GitHub
// conversions API, persists them, and returns the resulting public view.
func (m *ManifestService) ExchangeCode(ctx context.Context, tenantID, githubAppID, code string) (GitHubAppView, error) {
	if tenantID == "" {
		return GitHubAppView{}, invalid("tenant_id")
	}
	if githubAppID == "" {
		return GitHubAppView{}, invalid("github_app_id")
	}
	if code == "" {
		return GitHubAppView{}, invalid("code")
	}

	conversion, err := m.exchange(ctx, code)
	if err != nil {
		return GitHubAppView{}, err
	}

	if err := m.apps.Upsert(ctx, UpsertGitHubApp{
		ID:             githubAppID,
		TenantID:       tenantID,
		AppID:          conversion.ID,
		InstallationID: 0,
		PrivateKey:     conversion.PEM,
		BaseURL:        m.conversionsBaseURL,
		WebhookSecret:  conversion.WebhookSecret,
		ClientID:       conversion.ClientID,
		ClientSecret:   conversion.ClientSecret,
		AppSlug:        conversion.Slug,
	}); err != nil {
		return GitHubAppView{}, err
	}

	return m.apps.GetByID(ctx, tenantID, githubAppID)
}

// Install fetches trusted installation account metadata and records it on the
// state-selected App.
func (m *ManifestService) Install(ctx context.Context, tenantID, githubAppID string, installationID int64) (GitHubAppView, error) {
	login, err := m.apps.InstallationAccount(ctx, tenantID, githubAppID, installationID)
	if err != nil {
		return GitHubAppView{}, err
	}
	return m.apps.InstallByID(ctx, tenantID, githubAppID, installationID, login)
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
func (m *ManifestService) signState(payload ManifestState) string {
	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, m.stateSecret)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
