package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestDevicesCollectionExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatalf("expected devices collection to exist: %v", err)
	}

	for _, name := range []string{
		"name", "description", "location_label", "location_point",
		"photo", "category", "status", "owner", "current_borrower",
		"lend_start", "lend_end",
	} {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("expected devices collection to have a %q field", name)
		}
	}

	statusField, ok := collection.Fields.GetByName("status").(*core.SelectField)
	if !ok {
		t.Fatal("expected status to be a select field")
	}
	wantValues := []string{"available", "requested", "lent", "unavailable"}
	if len(statusField.Values) != len(wantValues) {
		t.Fatalf("expected %d status values, got %d", len(wantValues), len(statusField.Values))
	}

	if collection.DeleteRule == nil || *collection.DeleteRule == "" {
		t.Fatal("expected a non-empty DeleteRule")
	}
}
