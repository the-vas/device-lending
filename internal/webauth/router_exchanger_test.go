// internal/webauth/router_exchanger_test.go
package webauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

// fakeIdP is a minimal OIDC provider: just the token and userinfo endpoints
// that PocketBase's oidc provider actually calls during an auth-with-oauth2
// code exchange.
func fakeIdP(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"idp-access-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"sub": "idp-subject-1",
			"name": "Alice Example",
			"email": "alice@example.com",
			"email_verified": true,
			"groups": ["lending-admins"]
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func configureFakeIdP(t *testing.T, app core.App, idp *httptest.Server) {
	t.Helper()

	cfg := config.Config{
		OIDCIssuer:       idp.URL,
		OIDCClientID:     "device-lending",
		OIDCClientSecret: "client-secret",
	}
	discover := func(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
		return oidcdiscovery.Document{
			AuthorizationEndpoint: idp.URL + "/authorize",
			TokenEndpoint:         idp.URL + "/token",
			UserinfoEndpoint:      idp.URL + "/userinfo",
		}, nil
	}
	if err := authsetup.ConfigureOAuth2(app, cfg, discover); err != nil {
		t.Fatal(err)
	}
}

// TestRouterExchanger_FirstLoginCreatesUser is the guard for the whole OIDC
// login flow: it drives the real RouterExchanger (which POSTs in-process to
// /api/collections/users/auth-with-oauth2) against a fake IdP, with no user
// record existing yet. It must keep passing after the users collection is
// locked down against public self-signup.
func TestRouterExchanger_FirstLoginCreatesUser(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	idp := fakeIdP(t)
	configureFakeIdP(t, app, idp)

	if _, err := app.FindAuthRecordByEmail("users", "alice@example.com"); err == nil {
		t.Fatal("precondition failed: alice already exists")
	}

	result, err := webauth.RouterExchanger(app, "auth-code", "verifier", "https://lending.example.com/oidc/callback")
	if err != nil {
		t.Fatalf("oidc exchange failed: %v", err)
	}
	if result.Token == "" {
		t.Fatal("expected a non-empty auth token")
	}

	created, err := app.FindAuthRecordByEmail("users", "alice@example.com")
	if err != nil {
		t.Fatalf("expected the OIDC login to have created a user record: %v", err)
	}
	if !created.Verified() {
		t.Error("expected the OIDC-created record to be marked verified")
	}
}

// TestRouterExchanger_SecondLoginReusesUser covers the returning-user branch,
// which saves the record directly via Go (no update API rule involved).
func TestRouterExchanger_SecondLoginReusesUser(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	idp := fakeIdP(t)
	configureFakeIdP(t, app, idp)

	before, err := app.FindAllRecords("users")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := webauth.RouterExchanger(app, "auth-code", "verifier", "https://lending.example.com/oidc/callback"); err != nil {
		t.Fatalf("first oidc exchange failed: %v", err)
	}
	first, err := app.FindAuthRecordByEmail("users", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := webauth.RouterExchanger(app, "auth-code", "verifier", "https://lending.example.com/oidc/callback"); err != nil {
		t.Fatalf("second oidc exchange failed: %v", err)
	}
	second, err := app.FindAuthRecordByEmail("users", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if first.Id != second.Id {
		t.Errorf("expected the same user record to be reused, got %q then %q", first.Id, second.Id)
	}

	after, err := app.FindAllRecords("users")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("expected exactly 1 new user record after two logins, went from %d to %d", len(before), len(after))
	}
}
