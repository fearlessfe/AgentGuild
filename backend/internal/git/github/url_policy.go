package github

import (
	"errors"
	"net/url"
	"strings"
)

// ValidateBaseURL normalizes a GitHub API origin and requires its host to be
// explicitly trusted. GitHub Enterprise hosts must be supplied by deployment
// configuration; tenant input alone can never expand this allowlist.
func ValidateBaseURL(raw string, allowedHosts []string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("github base_url must be a credential-free HTTPS URL")
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, candidate := range allowedHosts {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if parsed, parseErr := url.Parse(candidate); parseErr == nil && parsed.Hostname() != "" {
			candidate = strings.ToLower(parsed.Hostname())
		}
		if candidate == host {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", errors.New("github base_url host is not in GITHUB_ALLOWED_HOSTS")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}
