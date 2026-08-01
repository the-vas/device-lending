package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestUsersIsAdminFieldExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	field := users.Fields.GetByName("is_admin")
	if field == nil {
		t.Fatal("expected users collection to have an 'is_admin' field")
	}
	if field.Type() != "bool" {
		t.Fatalf("expected is_admin to be a bool field, got %s", field.Type())
	}
}
