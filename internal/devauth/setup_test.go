package devauth_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestSetup_EnablesPasswordAuthAndSeedsUsers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if !users.PasswordAuth.Enabled {
		t.Fatal("expected password auth to be enabled")
	}

	regular, err := app.FindAuthRecordByEmail(users, devauth.UserEmail)
	if err != nil {
		t.Fatalf("expected regular dev user to exist: %v", err)
	}
	if regular.GetBool("is_admin") {
		t.Error("expected regular dev user to not be admin")
	}

	admin, err := app.FindAuthRecordByEmail(users, devauth.AdminEmail)
	if err != nil {
		t.Fatalf("expected admin dev user to exist: %v", err)
	}
	if !admin.GetBool("is_admin") {
		t.Error("expected admin dev user to be admin")
	}
}

func TestSetup_IdempotentOnSecondCall(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	records, err := app.FindAllRecords(users)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, r := range records {
		if r.GetString("email") == devauth.UserEmail || r.GetString("email") == devauth.AdminEmail {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected exactly 2 seeded dev users after two Setup calls, got %d", count)
	}
}
