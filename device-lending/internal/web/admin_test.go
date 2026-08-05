// internal/web/admin_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestAdminHandler_ForbidsNonAdmin(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	regular := newTestUserFor(t, app, "regular@example.com")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = regular
			return re.Next()
		})
		e.Router.GET("/admin", web.AdminHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminHandler_ListsAllDevices(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	admin := newTestUserFor(t, app, "admin@example.com")
	admin.Set("is_admin", true)
	if err := app.Save(admin); err != nil {
		t.Fatal(err)
	}
	owner := newTestUserFor(t, app, "owner@example.com")
	newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = admin
			return re.Next()
		})
		e.Router.GET("/admin", web.AdminHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected admin page to list all devices, including ones the admin doesn't own")
	}
}

func TestAdminDeleteDeviceHandler_BlocksWhileLent(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	admin := newTestUserFor(t, app, "admin@example.com")
	admin.Set("is_admin", true)
	if err := app.Save(admin); err != nil {
		t.Fatal(err)
	}
	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "lent")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = admin
			return re.Next()
		})
		e.Router.POST("/admin/devices/{id}/delete", web.AdminDeleteDeviceHandler(app))
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/devices/"+device.Id+"/delete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
