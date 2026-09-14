package authsetup_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"
)

func TestApplyAppName(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyAppName(app); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := app.Settings().Meta.AppName; got != authsetup.AppName {
		t.Errorf("expected app name %q, got %q", authsetup.AppName, got)
	}
}
