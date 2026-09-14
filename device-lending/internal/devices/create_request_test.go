package devices_test

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestCreateRequest_AvailableDeviceBecomesRequested(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "available")
	notifier := mail.New(app, "https://lending.example.com")

	start := types.NowDateTime()
	req, err := devices.CreateRequest(app, notifier, device, requester, start, types.DateTime{}, "please and thank you")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.GetString("status") != "pending" {
		t.Errorf("expected pending status, got %q", req.GetString("status"))
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status 'requested', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to owner, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != owner.Email() {
		t.Errorf("expected email to owner")
	}
}

func TestCreateRequest_RejectsEndBeforeStart(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "available")
	notifier := mail.New(app, "https://lending.example.com")

	start, err := types.ParseDateTime("2026-08-10 00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	end, err := types.ParseDateTime("2026-08-01 00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}

	_, err = devices.CreateRequest(app, notifier, device, requester, start, end, "")
	if !errors.Is(err, devices.ErrInvalidDateRange) {
		t.Fatalf("expected ErrInvalidDateRange, got %v", err)
	}

	pending, err := app.FindRecordsByFilter("lending_requests", "device = {:d}", "", 0, 0, map[string]any{"d": device.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("expected no request to be created, got %d", len(pending))
	}
}

func TestCreateRequest_AllowsEndEqualToStart(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "available")
	notifier := mail.New(app, "https://lending.example.com")

	same, err := types.ParseDateTime("2026-08-10 00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := devices.CreateRequest(app, notifier, device, requester, same, same, ""); err != nil {
		t.Fatalf("unexpected error for a same-day borrow: %v", err)
	}
}

func TestCreateRequest_AlreadyRequestedDeviceStaysRequested(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	firstRequester := newUser(t, app, "first@example.com")
	secondRequester := newUser(t, app, "second@example.com")
	device := newDevice(t, app, owner, "Saw", "requested")
	newRequest(t, app, device, firstRequester, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	_, err = devices.CreateRequest(app, notifier, device, secondRequester, types.NowDateTime(), types.DateTime{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status to remain 'requested', got %q", updatedDevice.GetString("status"))
	}
	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to owner, got %d", app.TestMailer.TotalSend())
	}
}
