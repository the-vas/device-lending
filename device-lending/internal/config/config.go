package config

import (
	"fmt"
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

	var missing []string
	if cfg.OIDCIssuer == "" {
		missing = append(missing, "OIDC_ISSUER")
	}
	if cfg.OIDCClientID == "" {
		missing = append(missing, "OIDC_CLIENT_ID")
	}
	if cfg.OIDCClientSecret == "" {
		missing = append(missing, "OIDC_CLIENT_SECRET")
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

	return cfg, nil
}
