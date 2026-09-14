package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestWithdraw_NotifiesOwnerAndRevertsDevice(t *testing.T) {
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

	if err := devices.Withdraw(app, notifier, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedReq, err := app.FindRecordById("lending_requests", req.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.GetString("status") != "withdrawn" {
		t.Errorf("expected status 'withdrawn', got %q", updatedReq.GetString("status"))
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "available" {
		t.Errorf("expected device status 'available', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to owner, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != owner.Email() {
		t.Error("expected email to owner")
	}
}
