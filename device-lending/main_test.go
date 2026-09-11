package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/devauth"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/webauth"
)

// TestBootstrap_FreshDataDir boots the app's real OnBootstrap chain against a
// genuinely empty data directory — unlike tests.NewTestApp(), which calls
// RunAllMigrations() itself before any hook fires and therefore hides the
// bootstrap ordering entirely.
func TestBootstrap_FreshDataDir(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	defer app.ResetBootstrapState() //nolint:errcheck // best-effort test cleanup

	cfg := config.Config{
		OIDCIssuer:       "https://auth.example.com",
		OIDCClientID:     "abc",
		OIDCClientSecret: "secret",
		BaseURL:          "https://lending.example.com",
		SessionSecret:    "at-least-32-bytes-of-random-secret",
	}

	discover := func(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
		return oidcdiscovery.Document{
			AuthorizationEndpoint: "https://auth.example.com/authorize",
			TokenEndpoint:         "https://auth.example.com/token",
			UserinfoEndpoint:      "https://auth.example.com/userinfo",
		}, nil
	}

	bindBootstrap(app, cfg, discover)

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap against a fresh data dir failed: %v", err)
	}

	for _, name := range []string{"categories", "devices", "lending_requests"} {
		if _, err := app.FindCollectionByNameOrId(name); err != nil {
			t.Errorf("expected the %q collection to exist after bootstrap: %v", name, err)
		}
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("is_admin") == nil {
		t.Error("expected the users collection to have the is_admin field after bootstrap")
	}
}

func TestHealthz(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/healthz", func(re *core.RequestEvent) error {
			return re.String(200, "ok")
		})
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %q", rec.Body.String())
	}
}

func TestBootstrap_DevAuth_SkipsOIDCAndSeedsUsers(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	defer app.ResetBootstrapState() //nolint:errcheck // best-effort test cleanup

	cfg := config.Config{
		DevAuth:       true,
		BaseURL:       "http://localhost:8090",
		SessionSecret: "at-least-32-bytes-of-random-secret",
	}

	discover := func(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
		t.Fatal("OIDC discovery must not be called when DevAuth is true")
		return oidcdiscovery.Document{}, nil
	}

	bindBootstrap(app, cfg, discover)

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap under DevAuth failed: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if !users.PasswordAuth.Enabled {
		t.Error("expected password auth to be enabled under DevAuth")
	}
	if _, err := app.FindAuthRecordByEmail(users, devauth.UserEmail); err != nil {
		t.Errorf("expected seeded dev user to exist: %v", err)
	}
}

func assertRouteStatus(t *testing.T, mux http.Handler, method, path string, want int) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: expected status %d, got %d", method, path, want, rec.Code)
	}
}

func TestBindAuthRoutes_DevAuth_RegistersDevLoginNotOIDC(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{DevAuth: true, BaseURL: "http://localhost:8090", SessionSecret: "at-least-32-bytes-of-random-secret"}
	signer := webauth.NewSigner(cfg.SessionSecret)

	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		bindAuthRoutes(e.Router, cfg, signer)
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	assertRouteStatus(t, mux, http.MethodGet, "/dev/login", http.StatusOK)
	assertRouteStatus(t, mux, http.MethodGet, "/oidc/login", http.StatusNotFound)
}

func TestBindAuthRoutes_OIDC_RegistersOIDCNotDevLogin(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{BaseURL: "https://lending.example.com", SessionSecret: "at-least-32-bytes-of-random-secret"}
	signer := webauth.NewSigner(cfg.SessionSecret)

	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		bindAuthRoutes(e.Router, cfg, signer)
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	assertRouteStatus(t, mux, http.MethodGet, "/dev/login", http.StatusNotFound)

	req := httptest.NewRequest(http.MethodGet, "/oidc/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /oidc/login route to be registered")
	}
}
