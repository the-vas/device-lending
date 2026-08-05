package web

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
)

func requireOwnerOrAdmin(e *core.RequestEvent, device *core.Record) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/oidc/login")
	}
	if device.GetString("owner") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
		return e.ForbiddenError("not your device", nil)
	}
	return nil
}

func HandoverHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		if err := e.Request.ParseForm(); err != nil {
			return e.BadRequestError("invalid form data", err)
		}

		request, err := app.FindRecordById("lending_requests", e.Request.FormValue("request_id"))
		if err != nil {
			return e.NotFoundError("request not found", err)
		}

		var lendEnd types.DateTime
		if v := e.Request.FormValue("lend_end"); v != "" {
			lendEnd, err = types.ParseDateTime(v)
			if err != nil {
				return e.BadRequestError("invalid lend_end date", err)
			}
		}

		if err := devices.Handover(app, notifier, device, request, lendEnd); err != nil {
			return e.BadRequestError("failed to hand over device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func MarkReturnedHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		if err := devices.MarkReturned(app, notifier, device); err != nil {
			return e.BadRequestError("failed to mark device returned", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func RejectRequestHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		request, err := app.FindRecordById("lending_requests", e.Request.PathValue("reqId"))
		if err != nil {
			return e.NotFoundError("request not found", err)
		}
		device, err := app.FindRecordById("devices", request.GetString("device"))
		if err != nil {
			return e.InternalServerError("failed to load device", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		if err := devices.Reject(app, notifier, request); err != nil {
			return e.BadRequestError("failed to reject request", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func ToggleUnavailableHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		switch device.GetString("status") {
		case "unavailable":
			device.Set("status", "available")
		case "available":
			device.Set("status", "unavailable")
		default:
			return e.BadRequestError("device must be available or unavailable to toggle", nil)
		}

		if err := app.Save(device); err != nil {
			return e.InternalServerError("failed to update device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func DeleteDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}
		if device.GetString("status") == "lent" {
			return e.BadRequestError("cannot delete a device that is currently lent", nil)
		}

		if err := app.Delete(device); err != nil {
			return e.InternalServerError("failed to delete device", err)
		}

		return e.Redirect(http.StatusFound, "/my/devices")
	}
}
