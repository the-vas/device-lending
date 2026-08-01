package authsetup_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestApplyReadRules_Public(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyReadRules(app, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	devices, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	if devices.ListRule == nil || *devices.ListRule != "" {
		t.Errorf("expected empty (public) ListRule, got %v", devices.ListRule)
	}
	if devices.ViewRule == nil || *devices.ViewRule != "" {
		t.Errorf("expected empty (public) ViewRule, got %v", devices.ViewRule)
	}
}

func TestApplyReadRules_AuthenticatedOnly(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyReadRules(app, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	devices, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	want := `@request.auth.id != ""`
	if devices.ListRule == nil || *devices.ListRule != want {
		t.Errorf("expected ListRule %q, got %v", want, devices.ListRule)
	}
}
