package authsetup_test

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/auth"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func fakeDiscovery(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
	return oidcdiscovery.Document{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/token",
		UserinfoEndpoint:      "https://auth.example.com/userinfo",
	}, nil
}

func TestConfigureOAuth2(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	cfg := config.Config{
		OIDCIssuer:       "https://auth.example.com",
		OIDCClientID:     "client-123",
		OIDCClientSecret: "secret-456",
	}

	if err := authsetup.ConfigureOAuth2(app, cfg, fakeDiscovery); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	if !users.OAuth2.Enabled {
		t.Fatal("expected OAuth2 to be enabled")
	}

	provider, ok := users.OAuth2.GetProviderConfig("oidc")
	if !ok {
		t.Fatal("expected an 'oidc' provider config")
	}
	if provider.ClientId != "client-123" {
		t.Errorf("unexpected ClientId: %s", provider.ClientId)
	}
	if provider.AuthURL != "https://auth.example.com/authorize" {
		t.Errorf("unexpected AuthURL: %s", provider.AuthURL)
	}
	if provider.TokenURL != "https://auth.example.com/token" {
		t.Errorf("unexpected TokenURL: %s", provider.TokenURL)
	}
	if provider.UserInfoURL != "https://auth.example.com/userinfo" {
		t.Errorf("unexpected UserInfoURL: %s", provider.UserInfoURL)
	}

	// calling it again (as happens on every boot) must not create a duplicate provider entry
	if err := authsetup.ConfigureOAuth2(app, cfg, fakeDiscovery); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	users, err = app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	// NOTE: PocketBase's bundled test fixtures pre-configure the "users"
	// collection with unrelated OAuth2 providers (e.g. gitlab, google), so we
	// can't assert on the total provider count here. Instead we assert that
	// re-running setup didn't create a duplicate "oidc" entry.
	oidcCount := 0
	for _, p := range users.OAuth2.Providers {
		if p.Name == "oidc" {
			oidcCount++
		}
	}
	if oidcCount != 1 {
		t.Fatalf("expected exactly 1 'oidc' provider after re-running setup, got %d", oidcCount)
	}
}

func TestRegisterOIDCScopes(t *testing.T) {
	authsetup.RegisterOIDCScopes("groups")

	provider, err := auth.NewProviderByName(auth.NameOIDC)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, s := range provider.Scopes() {
		if s == "groups" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'groups' scope to be registered, got %v", provider.Scopes())
	}
}
