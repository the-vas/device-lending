// internal/webauth/middleware_test.go
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

func newTestUserForSession(t *testing.T, app core.App) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail("session@example.com")
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLoadSession_ValidCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	user := newTestUserForSession(t, app)
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.BindFunc(webauth.LoadSession(signer))
		e.Router.GET("/whoami", func(re *core.RequestEvent) error {
			if re.Auth == nil {
				return re.String(http.StatusUnauthorized, "anonymous")
			}
			return re.String(http.StatusOK, re.Auth.Id)
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

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.AddCookie(&http.Cookie{Name: webauth.SessionCookieName, Value: signer.Sign(token)})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != user.Id {
		t.Errorf("expected body %q, got %q", user.Id, rec.Body.String())
	}
}

func TestLoadSession_NoCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.BindFunc(webauth.LoadSession(signer))
		e.Router.GET("/whoami", func(re *core.RequestEvent) error {
			if re.Auth == nil {
				return re.String(http.StatusUnauthorized, "anonymous")
			}
			return re.String(http.StatusOK, re.Auth.Id)
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

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogoutHandler(t *testing.T) {
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
		e.Router.POST("/logout", webauth.LogoutHandler())
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == webauth.SessionCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected logout to clear the session cookie")
	}
}
