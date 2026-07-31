package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		categories, err := app.FindCollectionByNameOrId("categories")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("devices")

		collection.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 200},
			&core.TextField{Name: "description", Max: 5000},
			&core.TextField{Name: "location_label", Max: 200},
			&core.GeoPointField{Name: "location_point"},
			&core.FileField{
				Name:      "photo",
				MaxSelect: 1,
				MaxSize:   5 << 20,
				MimeTypes: []string{"image/jpeg", "image/png", "image/webp"},
			},
			&core.RelationField{Name: "category", Required: true, CollectionId: categories.Id, MaxSelect: 1},
			&core.SelectField{
				Name:      "status",
				Required:  true,
				Values:    []string{"available", "requested", "lent", "unavailable"},
				MaxSelect: 1,
			},
			&core.RelationField{Name: "owner", Required: true, CollectionId: users.Id, MaxSelect: 1},
			&core.RelationField{Name: "current_borrower", CollectionId: users.Id, MaxSelect: 1},
			&core.DateField{Name: "lend_start"},
			&core.DateField{Name: "lend_end"},
		)

		// List/View are provisionally authenticated-only; Task 10's boot-time
		// setup overwrites them per the PUBLIC_READ config toggle.
		collection.ListRule = types.Pointer(`@request.auth.id != ""`)
		collection.ViewRule = types.Pointer(`@request.auth.id != ""`)

		collection.CreateRule = types.Pointer(`@request.auth.id != "" && owner = @request.auth.id`)
		collection.UpdateRule = types.Pointer(`@request.auth.id != "" && (owner = @request.auth.id || @request.auth.is_admin = true)`)
		collection.DeleteRule = types.Pointer(`@request.auth.id != "" && (owner = @request.auth.id || @request.auth.is_admin = true) && status != "lent"`)

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
