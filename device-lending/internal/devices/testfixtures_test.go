package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func newUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail(email)
	r.Set("name", email)
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func newDevice(t *testing.T, app core.App, owner *core.Record, name, status string) *core.Record {
	t.Helper()

	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Category for "+name)
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
	d.Set("status", status)
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func newRequest(t *testing.T, app core.App, device, requester *core.Record, status string) *core.Record {
	t.Helper()

	col, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("device", device.Id)
	r.Set("requester", requester.Id)
	r.Set("status", status)
	r.Set("requested_start", "2026-08-01 00:00:00.000Z")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}
