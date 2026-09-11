package config

import "testing"

func envMap(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestLoad_RequiredFieldsMissing(t *testing.T) {
	_, err := Load(envMap(map[string]string{}))
	if err == nil {
		t.Fatal("expected error when required env vars are missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"OIDC_ISSUER":        "https://auth.example.com",
		"OIDC_CLIENT_ID":     "abc",
		"OIDC_CLIENT_SECRET": "secret",
		"BASE_URL":           "https://lending.example.com",
		"SESSION_SECRET":     "at-least-32-bytes-of-random-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PublicRead != false {
		t.Errorf("expected PublicRead to default to false")
	}
	if cfg.OIDCAdminGroup != "" {
		t.Errorf("expected empty admin group by default, got %q", cfg.OIDCAdminGroup)
	}
}

func TestLoad_Overrides(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"OIDC_ISSUER":        "https://auth.example.com",
		"OIDC_CLIENT_ID":     "abc",
		"OIDC_CLIENT_SECRET": "secret",
		"BASE_URL":           "https://lending.example.com",
		"SESSION_SECRET":     "at-least-32-bytes-of-random-secret",
		"PUBLIC_READ":        "true",
		"OIDC_ADMIN_GROUP":   "admin",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.PublicRead {
		t.Errorf("expected PublicRead true")
	}
	if cfg.OIDCAdminGroup != "admin" {
		t.Errorf("expected admin group 'admin', got %q", cfg.OIDCAdminGroup)
	}
}

func TestLoad_ShortSessionSecretRejected(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"OIDC_ISSUER":        "https://auth.example.com",
		"OIDC_CLIENT_ID":     "abc",
		"OIDC_CLIENT_SECRET": "secret",
		"BASE_URL":           "https://lending.example.com",
		"SESSION_SECRET":     "too-short",
	}))
	if err == nil {
		t.Fatal("expected error for a SESSION_SECRET shorter than 32 characters")
	}
}

func TestLoad_DevAuthSkipsOIDCRequirement(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "http://localhost:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DevAuth {
		t.Error("expected DevAuth to be true")
	}
}

func TestLoad_DevAuthAcceptsLoopbackIP(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "http://127.0.0.1:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DevAuth {
		t.Error("expected DevAuth to be true")
	}
}

func TestLoad_DevAuthRejectsNonLocalBaseURL(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "https://lending.example.com",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err == nil {
		t.Fatal("expected error for DEV_AUTH=true with a non-localhost BASE_URL")
	}
}

func TestLoad_DevAuthRejectsHTTPSBaseURL(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "https://localhost:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err == nil {
		t.Fatal("expected error for DEV_AUTH=true with an https:// BASE_URL")
	}
}

func TestLoad_DevAuthFalseStillRequiresOIDC(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"BASE_URL":       "http://localhost:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err == nil {
		t.Fatal("expected error: DEV_AUTH defaults to false, so OIDC vars are still required")
	}
}
