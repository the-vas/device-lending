package devices_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func setupDeviceForGuardTest(t *testing.T, app core.App) (owner *core.Record, device *core.Record) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	owner = core.NewRecord(users)
	owner.SetEmail("owner@example.com")
	owner.SetRandomPassword()
	if err := app.Save(owner); err != nil {
		t.Fatal(err)
	}

	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Power tools")
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}

	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	device = core.NewRecord(devicesCol)
	device.Set("name", "Drill")
	device.Set("category", category.Id)
	device.Set("owner", owner.Id)
	device.Set("status", "available")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	return owner, device
}

func TestStateFieldGuard_BlocksDirectStatusEdit(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	devices.BindStateFieldGuard(app)

	owner, device := setupDeviceForGuardTest(t, app)

	token, err := owner.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	if err := app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error { return e.Next() }); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/collections/devices/records/"+device.Id, strings.NewReader(`{"status":"lent"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 blocking direct status edit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStateFieldGuard_AllowsNonGuardedFieldEdit(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	devices.BindStateFieldGuard(app)

	owner, device := setupDeviceForGuardTest(t, app)

	token, err := owner.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	if err := app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error { return e.Next() }); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/collections/devices/records/"+device.Id, strings.NewReader(`{"description":"cordless, 18V"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 allowing a non-guarded field edit, got %d: %s", rec.Code, rec.Body.String())
	}
}
