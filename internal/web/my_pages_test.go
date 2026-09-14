// internal/web/my_pages_test.go
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

func TestMyDevicesHandler_ListsOwnDevices(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	other := newTestUserFor(t, app, "other@example.com")
	newTestDeviceForBrowse(t, app, owner, "My Drill")
	newTestDeviceForBrowse(t, app, other, "Someone Else's Saw")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.GET("/my/devices", web.MyDevicesHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/my/devices", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "My Drill") {
		t.Error("expected page to list owner's own device")
	}
	if strings.Contains(rec.Body.String(), "Someone Else's Saw") {
		t.Error("expected page to NOT list another owner's device")
	}
}

func TestMyRequestsHandler_ListsOwnRequests(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	requestsCol, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatal(err)
	}
	request := core.NewRecord(requestsCol)
	request.Set("device", device.Id)
	request.Set("requester", requester.Id)
	request.Set("status", "pending")
	request.Set("requested_start", "2026-08-01 00:00:00.000Z")
	if err := app.Save(request); err != nil {
		t.Fatal(err)
	}

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = requester
			return re.Next()
		})
		e.Router.GET("/my/requests", web.MyRequestsHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/my/requests", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected page to list the requested device's name")
	}
}
