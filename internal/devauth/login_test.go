package devauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devauth"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func registerDevLoginRoutes(t *testing.T, app *tests.TestApp, signer webauth.Signer) http.Handler {
	t.Helper()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/dev/login", devauth.LoginPageHandler())
		e.Router.POST("/dev/login/user", devauth.LoginAsHandler(devauth.UserEmail, devauth.UserPassword, signer))
		e.Router.POST("/dev/login/admin", devauth.LoginAsHandler(devauth.AdminEmail, devauth.AdminPassword, signer))
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

func TestLoginAsHandler_RegularUser_SetsSessionCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error seeding dev users: %v", err)
	}

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodPost, "/dev/login/user", nil)
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
			if _, err := signer.Verify(c.Value); err != nil {
				t.Errorf("expected verifiable session cookie, got error: %v", err)
			}
		}
	}
	if !found {
		t.Fatal("expected session cookie to be set")
	}
}

func TestLoginAsHandler_Admin_SetsSessionCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error seeding dev users: %v", err)
	}

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodPost, "/dev/login/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginAsHandler_UnseededAccount_Fails(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// deliberately skip devauth.Setup: password auth stays disabled, so the
	// login attempt must fail rather than silently succeed.
	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodPost, "/dev/login/user", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Fatalf("expected login to fail without seeded users, got 302 redirect")
	}
}

func TestLoginPageHandler_Renders(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodGet, "/dev/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
