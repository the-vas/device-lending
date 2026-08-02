package webauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func registerCallbackRoute(t *testing.T, app *tests.TestApp, signer webauth.Signer, exchange webauth.Exchanger) http.Handler {
	t.Helper()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/oidc/callback", webauth.CallbackHandler("https://lending.example.com", signer, exchange))
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	return mux
}

func TestCallbackHandler_Success(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	fakeExchange := func(app core.App, code, codeVerifier, redirectURL string) (webauth.ExchangeResult, error) {
		if code != "auth-code-123" {
			t.Errorf("unexpected code: %s", code)
		}
		if codeVerifier != "verifier-abc" {
			t.Errorf("unexpected codeVerifier: %s", codeVerifier)
		}
		return webauth.ExchangeResult{Token: "fake-auth-token", Record: map[string]any{"id": "u1"}}, nil
	}

	mux := registerCallbackRoute(t, app, signer, fakeExchange)

	req := httptest.NewRequest(http.MethodGet, "/oidc/callback?code=auth-code-123&state=state-xyz", nil)
	req.AddCookie(&http.Cookie{
		Name:  webauth.PKCECookieName,
		Value: signer.Sign("state-xyz|verifier-abc"),
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/" {
		t.Errorf("expected redirect to /, got %s", rec.Header().Get("Location"))
	}

	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == webauth.SessionCookieName {
			found = true
			if c.Value == "" {
				t.Error("expected non-empty session cookie value")
			}
		}
	}
	if !found {
		t.Error("expected session cookie to be set")
	}
}

func TestCallbackHandler_StateMismatch(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	neverCalled := func(app core.App, code, codeVerifier, redirectURL string) (webauth.ExchangeResult, error) {
		t.Fatal("exchange should not be called on state mismatch")
		return webauth.ExchangeResult{}, nil
	}

	mux := registerCallbackRoute(t, app, signer, neverCalled)

	req := httptest.NewRequest(http.MethodGet, "/oidc/callback?code=auth-code-123&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  webauth.PKCECookieName,
		Value: signer.Sign("state-xyz|verifier-abc"),
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCallbackHandler_MissingCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	neverCalled := func(app core.App, code, codeVerifier, redirectURL string) (webauth.ExchangeResult, error) {
		t.Fatal("exchange should not be called without a pkce cookie")
		return webauth.ExchangeResult{}, nil
	}

	mux := registerCallbackRoute(t, app, signer, neverCalled)

	req := httptest.NewRequest(http.MethodGet, "/oidc/callback?code=auth-code-123&state=state-xyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
