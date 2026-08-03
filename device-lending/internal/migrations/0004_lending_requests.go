package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		devices, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("lending_requests")

		collection.Fields.Add(
			&core.RelationField{Name: "device", Required: true, CollectionId: devices.Id, MaxSelect: 1},
			&core.RelationField{Name: "requester", Required: true, CollectionId: users.Id, MaxSelect: 1},
			&core.SelectField{
				Name:      "status",
				Required:  true,
				Values:    []string{"pending", "accepted", "rejected", "withdrawn"},
				MaxSelect: 1,
			},
			&core.DateField{Name: "requested_start", Required: true},
			&core.DateField{Name: "requested_end"},
			&core.TextField{Name: "message", Max: 2000},
			&core.DateField{Name: "decided_at"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)

		viewRule := `@request.auth.id != "" && (requester = @request.auth.id || device.owner = @request.auth.id || @request.auth.is_admin = true)`
		collection.ListRule = types.Pointer(viewRule)
		collection.ViewRule = types.Pointer(viewRule)
		collection.CreateRule = types.Pointer(`@request.auth.id != "" && requester = @request.auth.id`)
		collection.UpdateRule = nil
		collection.DeleteRule = types.Pointer(`@request.auth.is_admin = true || (requester = @request.auth.id && status != "accepted")`)

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("lending_requests")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
