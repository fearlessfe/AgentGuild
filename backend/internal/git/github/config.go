package github

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// Config holds the parameters required to act as a GitHub App installation.
type Config struct {
	AppID          int64
	PrivateKey     string
	InstallationID int64
	BaseURL        string
}

// Validate checks that the configuration contains the mandatory values and
// applies defaults.
func (c Config) Validate() error {
	if c.AppID == 0 {
		return fmt.Errorf("github app_id is required")
	}
	if c.PrivateKey == "" {
		return fmt.Errorf("github private_key is required")
	}
	if c.InstallationID == 0 {
		return fmt.Errorf("github installation_id is required")
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.github.com"
	}
	return nil
}

// PrivateRSAKey parses the configured PEM-encoded private key. Both PKCS#1
// and PKCS#8 RSA keys are supported.
func (c Config) PrivateRSAKey() (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(c.PrivateKey))
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("PEM is not an RSA private key")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
}
