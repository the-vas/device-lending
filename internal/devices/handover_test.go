package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestHandover_AcceptsChosenAndNotifiesOthers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	chosen := newUser(t, app, "chosen@example.com")
	other := newUser(t, app, "other@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	chosenReq := newRequest(t, app, device, chosen, "pending")
	otherReq := newRequest(t, app, device, other, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	lendEnd, err := types.ParseDateTime("2026-08-10 00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}

	if err := devices.Handover(app, notifier, device, chosenReq, lendEnd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "lent" {
		t.Errorf("expected device status 'lent', got %q", updatedDevice.GetString("status"))
	}
	if updatedDevice.GetString("current_borrower") != chosen.Id {
		t.Errorf("expected current_borrower %q, got %q", chosen.Id, updatedDevice.GetString("current_borrower"))
	}

	updatedChosenReq, err := app.FindRecordById("lending_requests", chosenReq.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedChosenReq.GetString("status") != "accepted" {
		t.Errorf("expected chosen request status 'accepted', got %q", updatedChosenReq.GetString("status"))
	}

	updatedOtherReq, err := app.FindRecordById("lending_requests", otherReq.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedOtherReq.GetString("status") != "pending" {
		t.Errorf("expected other request to remain 'pending', got %q", updatedOtherReq.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 2 {
		t.Fatalf("expected 2 emails (chosen + other), got %d", app.TestMailer.TotalSend())
	}

	recipients := map[string]bool{}
	for _, m := range app.TestMailer.Messages() {
		recipients[m.To[0].Address] = true
	}
	if !recipients[chosen.Email()] || !recipients[other.Email()] {
		t.Errorf("expected emails to both chosen and other requester, got %v", recipients)
	}
}

func TestHandover_RejectsRequestFromOtherDevice(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	deviceA := newDevice(t, app, owner, "Drill", "requested")
	deviceB := newDevice(t, app, owner, "Saw", "requested")
	reqForB := newRequest(t, app, deviceB, requester, "pending")

	notifier := mail.New(app, "https://lending.example.com")

	err = devices.Handover(app, notifier, deviceA, reqForB, types.DateTime{})
	if err == nil {
		t.Fatal("expected error when request belongs to a different device")
	}
}
