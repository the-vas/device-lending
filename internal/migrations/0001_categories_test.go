// internal/migrations/0001_categories_test.go
package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestCategoriesCollectionExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatalf("expected categories collection to exist: %v", err)
	}

	if collection.Fields.GetByName("name") == nil {
		t.Fatal("expected categories collection to have a 'name' field")
	}
}
