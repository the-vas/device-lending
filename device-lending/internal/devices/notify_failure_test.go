// internal/devices/notify_failure_test.go
package devices_test

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

// breakMailer makes every outgoing message fail, the way PocketBase's default
// sendmail client does in the deployed container before an operator has
// configured SMTP.
func breakMailer(app *tests.TestApp) {
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		return errors.New("sendmail: executable file not found in $PATH")
	})
}

func mustFailToSend(t *testing.T, app *tests.TestApp) {
	t.Helper()
	if app.TestMailer.TotalSend() != 0 {
		t.Fatalf("test setup is wrong: %d message(s) actually went out", app.TestMailer.TotalSend())
	}
}

// TestWorkflowActions_SucceedWhenMailFails asserts that a notification failure
// never turns a committed state change into a reported error. Without this,
// every workflow action reports failure to the user until SMTP is configured,
// even though the action took effect.
func TestWorkflowActions_SucceedWhenMailFails(t *testing.T) {
	t.Run("CreateRequest", func(t *testing.T) {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		defer app.Cleanup()
		breakMailer(app)

		owner := newUser(t, app, "owner@example.com")
		requester := newUser(t, app, "requester@example.com")
		device := newDevice(t, app, owner, "Drill", "available")
		notifier := mail.New(app, "https://lending.example.com")

		req, err := devices.CreateRequest(app, notifier, device, requester, types.NowDateTime(), types.DateTime{}, "")
		if err != nil {
			t.Fatalf("expected success despite the mail failure, got: %v", err)
		}
		if req == nil {
			t.Fatal("expected the request record to be returned")
		}
		mustFailToSend(t, app)

		saved, err := app.FindRecordById("lending_requests", req.Id)
		if err != nil {
			t.Fatalf("expected the request to be persisted: %v", err)
		}
		if saved.GetString("status") != "pending" {
			t.Errorf("expected pending request, got %q", saved.GetString("status"))
		}
		updated, err := app.FindRecordById("devices", device.Id)
		if err != nil {
			t.Fatal(err)
		}
		if updated.GetString("status") != "requested" {
			t.Errorf("expected device status 'requested', got %q", updated.GetString("status"))
		}
	})

	t.Run("Handover", func(t *testing.T) {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		defer app.Cleanup()

		owner := newUser(t, app, "owner@example.com")
		chosen := newUser(t, app, "chosen@example.com")
		other := newUser(t, app, "other@example.com")
		device := newDevice(t, app, owner, "Saw", "requested")
		chosenReq := newRequest(t, app, device, chosen, "pending")
		newRequest(t, app, device, other, "pending")

		breakMailer(app)
		notifier := mail.New(app, "https://lending.example.com")

		if err := devices.Handover(app, notifier, device, chosenReq, types.DateTime{}); err != nil {
			t.Fatalf("expected success despite the mail failure, got: %v", err)
		}
		mustFailToSend(t, app)

		updated, err := app.FindRecordById("devices", device.Id)
		if err != nil {
			t.Fatal(err)
		}
		if updated.GetString("status") != "lent" {
			t.Errorf("expected device status 'lent', got %q", updated.GetString("status"))
		}
		if updated.GetString("current_borrower") != chosen.Id {
			t.Errorf("expected current_borrower %q, got %q", chosen.Id, updated.GetString("current_borrower"))
		}
	})

	t.Run("Reject", func(t *testing.T) {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		defer app.Cleanup()

		owner := newUser(t, app, "owner@example.com")
		requester := newUser(t, app, "requester@example.com")
		device := newDevice(t, app, owner, "Ladder", "requested")
		request := newRequest(t, app, device, requester, "pending")

		breakMailer(app)
		notifier := mail.New(app, "https://lending.example.com")

		if err := devices.Reject(app, notifier, request); err != nil {
			t.Fatalf("expected success despite the mail failure, got: %v", err)
		}
		mustFailToSend(t, app)

		updated, err := app.FindRecordById("lending_requests", request.Id)
		if err != nil {
			t.Fatal(err)
		}
		if updated.GetString("status") != "rejected" {
			t.Errorf("expected status 'rejected', got %q", updated.GetString("status"))
		}
	})

	t.Run("Withdraw", func(t *testing.T) {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		defer app.Cleanup()

		owner := newUser(t, app, "owner@example.com")
		requester := newUser(t, app, "requester@example.com")
		device := newDevice(t, app, owner, "Sander", "requested")
		request := newRequest(t, app, device, requester, "pending")

		breakMailer(app)
		notifier := mail.New(app, "https://lending.example.com")

		if err := devices.Withdraw(app, notifier, request); err != nil {
			t.Fatalf("expected success despite the mail failure, got: %v", err)
		}
		mustFailToSend(t, app)

		updated, err := app.FindRecordById("lending_requests", request.Id)
		if err != nil {
			t.Fatal(err)
		}
		if updated.GetString("status") != "withdrawn" {
			t.Errorf("expected status 'withdrawn', got %q", updated.GetString("status"))
		}
	})

	t.Run("MarkReturned", func(t *testing.T) {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		defer app.Cleanup()

		owner := newUser(t, app, "owner@example.com")
		borrower := newUser(t, app, "borrower@example.com")
		waiting := newUser(t, app, "waiting@example.com")
		device := newDevice(t, app, owner, "Trailer", "lent")
		device.Set("current_borrower", borrower.Id)
		if err := app.Save(device); err != nil {
			t.Fatal(err)
		}
		newRequest(t, app, device, waiting, "pending")

		breakMailer(app)
		notifier := mail.New(app, "https://lending.example.com")

		if err := devices.MarkReturned(app, notifier, device); err != nil {
			t.Fatalf("expected success despite the mail failure, got: %v", err)
		}
		mustFailToSend(t, app)

		updated, err := app.FindRecordById("devices", device.Id)
		if err != nil {
			t.Fatal(err)
		}
		if updated.GetString("status") != "requested" {
			t.Errorf("expected device status 'requested', got %q", updated.GetString("status"))
		}
		if updated.GetString("current_borrower") != "" {
			t.Errorf("expected current_borrower to be cleared, got %q", updated.GetString("current_borrower"))
		}
	})

	t.Run("DeleteCascade", func(t *testing.T) {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		defer app.Cleanup()

		owner := newUser(t, app, "owner@example.com")
		requester := newUser(t, app, "requester@example.com")
		device := newDevice(t, app, owner, "Jigsaw", "requested")
		newRequest(t, app, device, requester, "pending")

		breakMailer(app)
		devices.BindDeleteCascade(app, mail.New(app, "https://lending.example.com"))

		if err := app.Delete(device); err != nil {
			t.Fatalf("expected the delete to succeed despite the mail failure, got: %v", err)
		}
		mustFailToSend(t, app)

		if _, err := app.FindRecordById("devices", device.Id); err == nil {
			t.Error("expected the device to be deleted")
		}
	})
}
