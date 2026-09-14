// internal/web/my_pages.go
package web

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type myDeviceItem struct {
	ID           string
	Name         string
	Status       string
	PendingCount int
}

func MyDevicesHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		records, err := app.FindRecordsByFilter("devices", "owner = {:owner}", "name", 0, 0, dbx.Params{"owner": e.Auth.Id})
		if err != nil {
			return e.InternalServerError("failed to load devices", err)
		}

		items := make([]myDeviceItem, 0, len(records))
		for _, r := range records {
			pending, err := app.FindRecordsByFilter(
				"lending_requests",
				"device = {:device} && status = 'pending'",
				"", 0, 0,
				dbx.Params{"device": r.Id},
			)
			if err != nil {
				return e.InternalServerError("failed to load pending requests", err)
			}
			items = append(items, myDeviceItem{ID: r.Id, Name: r.GetString("name"), Status: r.GetString("status"), PendingCount: len(pending)})
		}

		html, err := Render([]string{"templates/my_devices.html"}, map[string]any{
			"Title":       "My devices",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Devices":     items,
			"LoginPath":   LoginPath,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

type myRequestItem struct {
	ID         string
	DeviceID   string
	DeviceName string
	Status     string
}

func MyRequestsHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		records, err := app.FindRecordsByFilter("lending_requests", "requester = {:r}", "-created", 0, 0, dbx.Params{"r": e.Auth.Id})
		if err != nil {
			return e.InternalServerError("failed to load requests", err)
		}

		items := make([]myRequestItem, 0, len(records))
		for _, r := range records {
			device, err := app.FindRecordById("devices", r.GetString("device"))
			if err != nil {
				return e.InternalServerError("failed to load device", err)
			}
			items = append(items, myRequestItem{ID: r.Id, DeviceID: device.Id, DeviceName: device.GetString("name"), Status: r.GetString("status")})
		}

		html, err := Render([]string{"templates/my_requests.html"}, map[string]any{
			"Title":       "My requests",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Requests":    items,
			"LoginPath":   LoginPath,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}
