// internal/web/device_form_test.go
package web_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestCategory(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func multipartDeviceForm(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestCreateDeviceHandler_SetsOwnerAndSaves(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	category := newTestCategory(t, app, "Power tools")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/new", web.CreateDeviceHandler(app))
	})

	body, contentType := multipartDeviceForm(t, map[string]string{
		"name":           "Cordless Drill",
		"description":    "18V, cordless",
		"category":       category.Id,
		"location_label": "Garage",
		"lat":            "52.5",
		"lon":            "13.4",
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/new", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	created, err := app.FindRecordsByFilter("devices", "name = {:n}", "", 0, 0, map[string]any{"n": "Cordless Drill"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 device created, got %d", len(created))
	}
	if created[0].GetString("owner") != owner.Id {
		t.Errorf("expected owner %q, got %q", owner.Id, created[0].GetString("owner"))
	}
	if created[0].GetString("status") != "available" {
		t.Errorf("expected default status 'available', got %q", created[0].GetString("status"))
	}
	if created[0].GetGeoPoint("location_point").Lat != 52.5 {
		t.Errorf("expected lat 52.5, got %v", created[0].GetGeoPoint("location_point").Lat)
	}
}

func TestUpdateDeviceHandler_ForbidsNonOwner(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	intruder := newTestUserFor(t, app, "intruder@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = intruder
			return re.Next()
		})
		e.Router.POST("/devices/{id}/edit", web.UpdateDeviceHandler(app))
	})

	body, contentType := multipartDeviceForm(t, map[string]string{"name": "Hacked name"})
	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/edit", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
