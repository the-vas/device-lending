package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestReject_LastPendingRevertsDeviceToAvailable(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, requester, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	if err := devices.Reject(app, notifier, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedReq, err := app.FindRecordById("lending_requests", req.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.GetString("status") != "rejected" {
		t.Errorf("expected status 'rejected', got %q", updatedReq.GetString("status"))
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "available" {
		t.Errorf("expected device status 'available', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to requester, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != requester.Email() {
		t.Error("expected email to requester")
	}
}

func TestReject_OtherPendingKeepsDeviceRequested(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	toReject := newUser(t, app, "reject-me@example.com")
	stillPending := newUser(t, app, "still-pending@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, toReject, "pending")
	newRequest(t, app, device, stillPending, "pending")

	notifier := mail.New(app, "https://lending.example.com")

	if err := devices.Reject(app, notifier, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status to remain 'requested', got %q", updatedDevice.GetString("status"))
	}
}
