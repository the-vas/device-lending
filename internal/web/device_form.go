// internal/web/device_form.go
package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func loadCategories(app core.App) ([]*core.Record, error) {
	return app.FindRecordsByFilter("categories", "", "name", 0, 0)
}

func bindDeviceForm(e *core.RequestEvent, record *core.Record) error {
	if err := e.Request.ParseMultipartForm(10 << 20); err != nil {
		return err
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("description", e.Request.FormValue("description"))
	record.Set("category", e.Request.FormValue("category"))
	record.Set("location_label", e.Request.FormValue("location_label"))

	latStr, lonStr := e.Request.FormValue("lat"), e.Request.FormValue("lon")
	if latStr != "" && lonStr != "" {
		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			return err
		}
		lon, err := strconv.ParseFloat(lonStr, 64)
		if err != nil {
			return err
		}
		record.Set("location_point", types.GeoPoint{Lat: lat, Lon: lon})
	}

	files, err := e.FindUploadedFiles("photo")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		return err
	}
	if len(files) > 0 {
		record.Set("photo", files)
	}

	return nil
}

func NewDeviceFormHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		categories, err := loadCategories(app)
		if err != nil {
			return e.InternalServerError("failed to load categories", err)
		}

		html, err := Render([]string{"templates/device_form.html"}, map[string]any{
			"Title":       "New device",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Categories":  categories,
			"Device":      nil,
			"IsEdit":      false,
			"LoginPath":   LoginPath,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

func CreateDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return e.InternalServerError("failed to load devices collection", err)
		}

		record := core.NewRecord(collection)
		record.Set("owner", e.Auth.Id)
		record.Set("status", "available")

		if err := bindDeviceForm(e, record); err != nil {
			return e.BadRequestError("invalid form data", err)
		}
		if err := app.Save(record); err != nil {
			return e.BadRequestError("failed to save device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+record.Id)
	}
}

func EditDeviceFormHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if device.GetString("owner") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
			return e.ForbiddenError("not your device", nil)
		}

		categories, err := loadCategories(app)
		if err != nil {
			return e.InternalServerError("failed to load categories", err)
		}

		html, err := Render([]string{"templates/device_form.html"}, map[string]any{
			"Title":       "Edit " + device.GetString("name"),
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Categories":  categories,
			"Device":      device,
			"IsEdit":      true,
			"LoginPath":   LoginPath,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

func UpdateDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if device.GetString("owner") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
			return e.ForbiddenError("not your device", nil)
		}

		if err := bindDeviceForm(e, device); err != nil {
			return e.BadRequestError("invalid form data", err)
		}
		if err := app.Save(device); err != nil {
			return e.BadRequestError("failed to save device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}
