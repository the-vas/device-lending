package webauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func fakeDiscovery(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
	return oidcdiscovery.Document{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/token",
		UserinfoEndpoint:      "https://auth.example.com/userinfo",
	}, nil
}

func TestLoginHandler(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	cfg := config.Config{OIDCIssuer: "https://auth.example.com", OIDCClientID: "abc", OIDCClientSecret: "secret"}
	if err := authsetup.ConfigureOAuth2(app, cfg, fakeDiscovery); err != nil {
		t.Fatal(err)
	}

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/oidc/login", webauth.LoginHandler("https://lending.example.com", signer))
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/oidc/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://auth.example.com/authorize") {
		t.Errorf("unexpected redirect location: %s", loc)
	}
	if !strings.Contains(loc, "code_challenge=") {
		t.Errorf("expected code_challenge param, got %s", loc)
	}

	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == webauth.PKCECookieName {
			found = true
			if c.Value == "" {
				t.Error("expected non-empty pkce cookie value")
			}
		}
	}
	if !found {
		t.Error("expected pkce cookie to be set")
	}
}
