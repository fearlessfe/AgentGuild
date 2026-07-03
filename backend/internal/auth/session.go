package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const SessionCookieName = "agentguild_session"

type Session struct {
	TenantID   string    `json:"tenant_id"`
	OwnerID    string    `json:"owner_id"`
	OwnerEmail string    `json:"owner_email"`
	IsAdmin    bool      `json:"is_admin"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

func NewSessionCookie(session Session, secret string, secure bool) (*http.Cookie, error) {
	if secret == "" {
		return nil, errors.New("session secret is required")
	}
	if session.TenantID == "" || session.OwnerID == "" || session.OwnerEmail == "" {
		return nil, errors.New("session is missing required identity fields")
	}
	body, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	signature := signSessionPayload(payload, secret)
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    payload + "." + signature,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	if !session.ExpiresAt.IsZero() {
		cookie.Expires = session.ExpiresAt
	}
	return cookie, nil
}

func ParseSessionCookie(cookie *http.Cookie, secret string) (Session, error) {
	var session Session
	if cookie == nil {
		return session, errors.New("session cookie is required")
	}
	if cookie.Name != SessionCookieName {
		return session, errors.New("unexpected session cookie name")
	}
	if secret == "" {
		return session, errors.New("session secret is required")
	}
	payload, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok || payload == "" || signature == "" {
		return session, errors.New("session cookie is malformed")
	}
	if !hmac.Equal([]byte(signature), []byte(signSessionPayload(payload, secret))) {
		return session, errors.New("session cookie signature is invalid")
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return session, err
	}
	if err := json.Unmarshal(body, &session); err != nil {
		return session, err
	}
	if session.TenantID == "" || session.OwnerID == "" || session.OwnerEmail == "" {
		return session, errors.New("session is missing required identity fields")
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return session, errors.New("session is expired")
	}
	return session, nil
}

func signSessionPayload(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
