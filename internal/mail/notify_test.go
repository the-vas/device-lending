package mail_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestUser(t *testing.T, app core.App, email, name string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail(email)
	r.Set("name", name)
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func newTestDevice(t *testing.T, app core.App, owner *core.Record, name string) *core.Record {
	t.Helper()
	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Power tools")
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}

	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(devicesCol)
	d.Set("name", name)
	d.Set("category", category.Id)
	d.Set("owner", owner.Id)
	d.Set("status", "available")
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNotifier_AllTriggers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUser(t, app, "owner@example.com", "Alice")
	requester := newTestUser(t, app, "requester@example.com", "Bob <script>")
	device := newTestDevice(t, app, owner, "Cordless Drill")

	n := mail.New(app, "https://lending.example.com")

	cases := []struct {
		name        string
		call        func() error
		wantTo      string
		wantSubject string
		wantBody    []string
		notWantBody []string
	}{
		{"NewRequest", func() error { return n.NewRequest(owner, device, requester) }, owner.Email(), "Cordless Drill", []string{"Bob &lt;script&gt;"}, []string{"<script>"}},
		{"RequestRejected", func() error { return n.RequestRejected(requester, device) }, requester.Email(), "Cordless Drill", []string{"declined"}, nil},
		{"HandoverAccepted", func() error { return n.HandoverAccepted(requester, device, "2026-08-10") }, requester.Email(), "Cordless Drill", []string{"2026-08-10"}, nil},
		{"DeviceUnavailable", func() error { return n.DeviceUnavailable(requester, device, "2026-08-10", 2) }, requester.Email(), "Cordless Drill", []string{"2 other"}, nil},
		{"DeviceAvailableAgain", func() error { return n.DeviceAvailableAgain(requester, device) }, requester.Email(), "Cordless Drill", []string{"available again"}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app.TestMailer.Reset()

			if err := c.call(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if app.TestMailer.TotalSend() != 1 {
				t.Fatalf("expected 1 email sent, got %d", app.TestMailer.TotalSend())
			}
			msg := app.TestMailer.LastMessage()
			if len(msg.To) != 1 || msg.To[0].Address != c.wantTo {
				t.Errorf("expected recipient %s, got %v", c.wantTo, msg.To)
			}
			if !strings.Contains(msg.Subject, c.wantSubject) {
				t.Errorf("expected subject to contain %q, got %q", c.wantSubject, msg.Subject)
			}
			for _, want := range c.wantBody {
				if !strings.Contains(msg.HTML, want) {
					t.Errorf("expected body to contain %q, got %q", want, msg.HTML)
				}
			}
			for _, notWant := range c.notWantBody {
				if strings.Contains(msg.HTML, notWant) {
					t.Errorf("expected body NOT to contain unescaped %q, got %q", notWant, msg.HTML)
				}
			}
		})
	}
}

func TestNotifier_RequestWithdrawnAndDeviceRemoved(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUser(t, app, "owner@example.com", "Alice")
	requester := newTestUser(t, app, "requester@example.com", "Bob")
	device := newTestDevice(t, app, owner, "Cordless Drill")

	n := mail.New(app, "https://lending.example.com")

	if err := n.RequestWithdrawn(owner, device, requester); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != owner.Email() {
		t.Errorf("expected RequestWithdrawn to notify the owner")
	}

	app.TestMailer.Reset()

	if err := n.DeviceRemoved(requester, device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != requester.Email() {
		t.Errorf("expected DeviceRemoved to notify the requester")
	}
}
