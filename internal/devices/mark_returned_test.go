package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestMarkReturned_NoPendingGoesAvailable(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	device := newDevice(t, app, owner, "Drill", "lent")
	notifier := mail.New(app, "https://lending.example.com")

	if err := devices.MarkReturned(app, notifier, device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "available" {
		t.Errorf("expected device status 'available', got %q", updatedDevice.GetString("status"))
	}
	if updatedDevice.GetString("current_borrower") != "" {
		t.Errorf("expected current_borrower cleared, got %q", updatedDevice.GetString("current_borrower"))
	}
	if app.TestMailer.TotalSend() != 0 {
		t.Errorf("expected no emails when there are no pending requests, got %d", app.TestMailer.TotalSend())
	}
}

func TestMarkReturned_PendingRequestsGoBackToRequestedAndNotified(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	waiting := newUser(t, app, "waiting@example.com")
	device := newDevice(t, app, owner, "Drill", "lent")
	newRequest(t, app, device, waiting, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	if err := devices.MarkReturned(app, notifier, device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status 'requested', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to the waiting requester, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != waiting.Email() {
		t.Error("expected email to the waiting requester")
	}
}
