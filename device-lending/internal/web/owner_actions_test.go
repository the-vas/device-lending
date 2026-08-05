package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/mail"
	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestHandoverHandler_OwnerHandsOverToRequester(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "requested")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}
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

	notifier := mail.New(app, "https://lending.example.com")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/{id}/handover", web.HandoverHandler(app, notifier))
	})

	form := url.Values{"request_id": {request.Id}, "lend_end": {"2026-08-10"}}
	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/handover", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetString("status") != "lent" {
		t.Errorf("expected status 'lent', got %q", updated.GetString("status"))
	}
	if updated.GetString("current_borrower") != requester.Id {
		t.Errorf("expected current_borrower %q, got %q", requester.Id, updated.GetString("current_borrower"))
	}
}

func TestDeleteDeviceHandler_BlocksWhileLent(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "lent")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/{id}/delete", web.DeleteDeviceHandler(app))
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/delete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	stillThere, err := app.FindRecordById("devices", device.Id)
	if err != nil || stillThere == nil {
		t.Error("expected device to still exist after a blocked delete")
	}
}

func TestToggleUnavailableHandler_RoundTrips(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/{id}/toggle-unavailable", web.ToggleUnavailableHandler(app))
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/toggle-unavailable", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetString("status") != "unavailable" {
		t.Errorf("expected status 'unavailable', got %q", updated.GetString("status"))
	}
}
