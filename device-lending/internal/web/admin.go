// internal/web/admin.go
package web

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func requireAdmin(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/oidc/login")
	}
	if !e.Auth.GetBool("is_admin") {
		return e.ForbiddenError("admin only", nil)
	}
	return nil
}

func AdminHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if err := requireAdmin(e); err != nil {
			return err
		}

		devicesRecords, err := app.FindRecordsByFilter("devices", "", "name", 0, 0)
		if err != nil {
			return e.InternalServerError("failed to load devices", err)
		}
		requestsRecords, err := app.FindRecordsByFilter("lending_requests", "", "-created", 0, 0)
		if err != nil {
			return e.InternalServerError("failed to load requests", err)
		}

		html, err := Render([]string{"templates/admin.html"}, map[string]any{
			"Title":       "Admin",
			"CurrentUser": e.Auth,
			"IsAdmin":     true,
			"Devices":     devicesRecords,
			"Requests":    requestsRecords,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

func AdminDeleteDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if err := requireAdmin(e); err != nil {
			return err
		}

		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if device.GetString("status") == "lent" {
			return e.BadRequestError("cannot delete a device that is currently lent", nil)
		}

		if err := app.Delete(device); err != nil {
			return e.InternalServerError("failed to delete device", err)
		}
		return e.Redirect(http.StatusFound, "/admin")
	}
}

func AdminDeleteRequestHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if err := requireAdmin(e); err != nil {
			return err
		}

		request, err := app.FindRecordById("lending_requests", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("request not found", err)
		}

		if err := app.Delete(request); err != nil {
			return e.InternalServerError("failed to delete request", err)
		}
		return e.Redirect(http.StatusFound, "/admin")
	}
}
