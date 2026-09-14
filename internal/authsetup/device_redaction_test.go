// internal/authsetup/device_redaction_test.go
package authsetup_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newLentDevice(t *testing.T, app core.App) (device, borrower *core.Record) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	owner := core.NewRecord(users)
	owner.SetEmail("redaction-owner@example.com")
	owner.Set("name", "Owner")
	owner.SetRandomPassword()
	if err := app.Save(owner); err != nil {
		t.Fatal(err)
	}
	borrower = core.NewRecord(users)
	borrower.SetEmail("redaction-borrower@example.com")
	borrower.Set("name", "Borrower")
	borrower.SetRandomPassword()
	if err := app.Save(borrower); err != nil {
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
	device.Set("name", "Hammer drill")
	device.Set("category", category.Id)
	device.Set("owner", owner.Id)
	device.Set("status", "lent")
	device.Set("current_borrower", borrower.Id)
	device.Set("lend_start", "2026-08-01 00:00:00.000Z")
	device.Set("lend_end", "2026-08-08 00:00:00.000Z")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	return device, borrower
}

func devicesAPIMux(t *testing.T, app *tests.TestApp) http.Handler {
	t.Helper()

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
	return mux
}

func getJSON(t *testing.T, mux http.Handler, url, token string) map[string]any {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", url, rec.Code, rec.Body.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

// TestAnonymousDeviceRedaction_ViewEndpoint asserts that with PUBLIC_READ
// enabled the REST API does not leak who currently has a device. The raw
// current_borrower id is de-anonymizable by cross-referencing the public owner
// field of any other device, so the HTML page hiding it is not enough.
func TestAnonymousDeviceRedaction_ViewEndpoint(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyReadRules(app, true); err != nil {
		t.Fatal(err)
	}
	authsetup.BindAnonymousDeviceRedaction(app)

	device, borrower := newLentDevice(t, app)
	mux := devicesAPIMux(t, app)

	anon := getJSON(t, mux, "/api/collections/devices/records/"+device.Id, "")
	for _, field := range []string{"current_borrower", "lend_start", "lend_end"} {
		if v, ok := anon[field]; ok {
			t.Errorf("anonymous response leaked %q = %v", field, v)
		}
	}
	if anon["name"] != "Hammer drill" {
		t.Errorf("expected the non-sensitive fields to survive, got name=%v", anon["name"])
	}

	token, err := borrower.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	authed := getJSON(t, mux, "/api/collections/devices/records/"+device.Id, token)
	if authed["current_borrower"] != borrower.Id {
		t.Errorf("expected authenticated requests to still see current_borrower, got %v", authed["current_borrower"])
	}
	if authed["lend_end"] == nil || authed["lend_end"] == "" {
		t.Errorf("expected authenticated requests to still see lend_end, got %v", authed["lend_end"])
	}
}

// TestAnonymousDeviceRedaction_ListEndpoint covers the same leak via the list
// endpoint, which serializes records through a different code path.
func TestAnonymousDeviceRedaction_ListEndpoint(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyReadRules(app, true); err != nil {
		t.Fatal(err)
	}
	authsetup.BindAnonymousDeviceRedaction(app)

	_, borrower := newLentDevice(t, app)
	mux := devicesAPIMux(t, app)

	firstItem := func(payload map[string]any) map[string]any {
		items, ok := payload["items"].([]any)
		if !ok || len(items) == 0 {
			t.Fatalf("expected at least one device in the list response, got %v", payload)
		}
		item, ok := items[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected list item shape: %v", items[0])
		}
		return item
	}

	anon := firstItem(getJSON(t, mux, "/api/collections/devices/records", ""))
	if v, ok := anon["current_borrower"]; ok {
		t.Errorf("anonymous list response leaked current_borrower = %v", v)
	}

	token, err := borrower.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	authed := firstItem(getJSON(t, mux, "/api/collections/devices/records", token))
	if authed["current_borrower"] != borrower.Id {
		t.Errorf("expected authenticated list responses to still include current_borrower, got %v", authed["current_borrower"])
	}
}
