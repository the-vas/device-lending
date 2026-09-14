// internal/web/browse_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestOwner(t *testing.T, app core.App) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail("owner@example.com")
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func newTestDeviceForBrowse(t *testing.T, app core.App, owner *core.Record, name string) *core.Record {
	t.Helper()
	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Category for "+name)
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}
	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(devicesCol)
	d.Set("name", name)
	d.Set("category", category.Id)
	d.Set("owner", owner.Id)
	d.Set("status", "available")
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func buildMux(t *testing.T, app *tests.TestApp, register func(e *core.ServeEvent)) http.Handler {
	t.Helper()
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		register(e)
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

func TestBrowseHandler_ListsAndFilters(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestOwner(t, app)
	newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	newTestDeviceForBrowse(t, app, owner, "Circular Saw")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/", web.BrowseHandler(app, true))
	})

	req := httptest.NewRequest(http.MethodGet, "/?q=Drill", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected result to contain 'Cordless Drill'")
	}
	if strings.Contains(rec.Body.String(), "Circular Saw") {
		t.Error("expected search filter to exclude 'Circular Saw'")
	}
}

func TestBrowseHandler_RequiresAuthWhenNotPublic(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/", web.BrowseHandler(app, false))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to login, got %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/oidc/login" {
		t.Errorf("expected redirect to /oidc/login, got %s", rec.Header().Get("Location"))
	}
}

func TestBrowseHandler_RedirectsToConfiguredLoginPath(t *testing.T) {
	defer func() { web.LoginPath = "/oidc/login" }()
	web.LoginPath = "/dev/login"

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/", web.BrowseHandler(app, false))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to login, got %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/dev/login" {
		t.Errorf("expected redirect to /dev/login, got %s", rec.Header().Get("Location"))
	}
}
