// internal/web/device_detail_test.go
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

func newTestUserFor(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail(email)
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDeviceDetailHandler_RendersDevice(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, true))
	})

	req := httptest.NewRequest(http.MethodGet, "/devices/"+device.Id, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected page to contain device name")
	}
}

func TestDeviceDetailHandler_HidesBorrowerFromAnonymous(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	borrower := newTestUserFor(t, app, "borrower@example.com")
	borrower.Set("name", "Secret Borrower")
	if err := app.Save(borrower); err != nil {
		t.Fatal(err)
	}
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "lent")
	device.Set("current_borrower", borrower.Id)
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	// A single router is built (and calling apis.NewRouter more than once per
	// app permanently accumulates duplicate OnServe hooks in this PocketBase
	// version, which panics on the second BuildMux), with the auth state read
	// per-request from this variable so both scenarios below can share it.
	var currentAuth *core.Record
	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = currentAuth
			return re.Next()
		})
		e.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, true))
	})

	req := httptest.NewRequest(http.MethodGet, "/devices/"+device.Id, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "Secret Borrower") {
		t.Error("expected anonymous visitor to NOT see the borrower's name")
	}

	currentAuth = owner
	req2 := httptest.NewRequest(http.MethodGet, "/devices/"+device.Id, nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "Secret Borrower") {
		t.Error("expected an authenticated visitor to see the borrower's name")
	}
}

func TestRequestDeviceHandler_CreatesPendingRequest(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	notifier := mail.New(app, "https://lending.example.com")

	form := url.Values{"requested_start": {"2026-08-01"}, "message": {"pretty please"}}
	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()

	// simulate an authenticated session by setting e.Auth directly is not
	// possible from outside the handler chain, so bind a tiny middleware
	// that stands in for webauth.LoadSession in this isolated test.
	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = requester
			return re.Next()
		})
		e.Router.POST("/devices/{id}/request", web.RequestDeviceHandler(app, notifier))
	})
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	pending, err := app.FindRecordsByFilter("lending_requests", "device = {:d} && requester = {:r}", "", 0, 0, map[string]any{"d": device.Id, "r": requester.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 request created, got %d", len(pending))
	}
}

func TestWithdrawRequestHandler_ForbidsOtherUsers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	intruder := newTestUserFor(t, app, "intruder@example.com")
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
			re.Auth = intruder
			return re.Next()
		})
		e.Router.POST("/devices/{id}/requests/{reqId}/withdraw", web.WithdrawRequestHandler(app, notifier))
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/requests/"+request.Id+"/withdraw", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
