package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestLendingRequestsCollectionExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatalf("expected lending_requests collection to exist: %v", err)
	}

	for _, name := range []string{
		"device", "requester", "status", "requested_start",
		"requested_end", "message", "decided_at",
	} {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("expected lending_requests collection to have a %q field", name)
		}
	}

	if collection.DeleteRule == nil {
		t.Fatal("expected a non-nil DeleteRule")
	}
	if collection.UpdateRule != nil {
		t.Fatal("expected UpdateRule to be nil (status changes go through service-layer code, not the public API)")
	}
}
