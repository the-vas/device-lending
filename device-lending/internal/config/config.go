// internal/config/config.go
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCAdminGroup   string
	PublicRead       bool
	BaseURL          string
	SessionSecret    string
	DevAuth          bool
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		OIDCIssuer:       getenv("OIDC_ISSUER"),
		OIDCClientID:     getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: getenv("OIDC_CLIENT_SECRET"),
		OIDCAdminGroup:   getenv("OIDC_ADMIN_GROUP"),
		BaseURL:          strings.TrimSuffix(getenv("BASE_URL"), "/"),
		SessionSecret:    getenv("SESSION_SECRET"),
	}

	if v := getenv("PUBLIC_READ"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PUBLIC_READ value %q: %w", v, err)
		}
		cfg.PublicRead = b
	}

	if v := getenv("DEV_AUTH"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DEV_AUTH value %q: %w", v, err)
		}
		cfg.DevAuth = b
	}

	var missing []string
	if !cfg.DevAuth {
		if cfg.OIDCIssuer == "" {
			missing = append(missing, "OIDC_ISSUER")
		}
		if cfg.OIDCClientID == "" {
			missing = append(missing, "OIDC_CLIENT_ID")
		}
		if cfg.OIDCClientSecret == "" {
			missing = append(missing, "OIDC_CLIENT_SECRET")
		}
	}
	if cfg.BaseURL == "" {
		missing = append(missing, "BASE_URL")
	}
	if cfg.SessionSecret == "" {
		missing = append(missing, "SESSION_SECRET")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if len(cfg.SessionSecret) < 32 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least 32 characters, got %d", len(cfg.SessionSecret))
	}

	if cfg.DevAuth {
		if err := requireLocalBaseURL(cfg.BaseURL); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

// requireLocalBaseURL enforces that DEV_AUTH=true can only ever boot against
// a plain-http localhost/127.0.0.1 BASE_URL, so this dev-only auth bypass can
// never activate in a real deployment.
func requireLocalBaseURL(baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("DEV_AUTH=true requires a valid BASE_URL: %w", err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("DEV_AUTH=true requires BASE_URL to use http, got scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return fmt.Errorf("DEV_AUTH=true requires BASE_URL host to be localhost or 127.0.0.1, got %q", host)
	}
	return nil
}
