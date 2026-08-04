package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestDeleteCascade_RejectsPendingAndNotifies(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	notifier := mail.New(app, "https://lending.example.com")
	devices.BindDeleteCascade(app, notifier)

	owner := newUser(t, app, "owner@example.com")
	waiting := newUser(t, app, "waiting@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, waiting, "pending")

	app.TestMailer.Reset()

	if err := app.Delete(device); err != nil {
		t.Fatalf("unexpected error deleting device: %v", err)
	}

	if _, err := app.FindRecordById("lending_requests", req.Id); err == nil {
		t.Error("expected the pending request to be cascade-deleted along with the device")
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to the waiting requester, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != waiting.Email() {
		t.Error("expected email to the waiting requester")
	}
}
