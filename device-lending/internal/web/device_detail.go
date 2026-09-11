// internal/web/device_detail.go
package web

import (
	"errors"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
)

type pendingRequestView struct {
	ID             string
	RequesterName  string
	RequesterEmail string
	RequestedStart string
	RequestedEnd   string
	Message        string
}

func DeviceDetailHandler(app core.App, publicRead bool) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		id := e.Request.PathValue("id")
		device, err := app.FindRecordById("devices", id)
		if err != nil {
			return e.NotFoundError("device not found", err)
		}

		category, err := app.FindRecordById("categories", device.GetString("category"))
		if err != nil {
			return e.InternalServerError("failed to load category", err)
		}
		owner, err := app.FindRecordById("users", device.GetString("owner"))
		if err != nil {
			return e.InternalServerError("failed to load owner", err)
		}

		var myPendingRequest *core.Record
		if e.Auth != nil {
			myPendingRequest, _ = app.FindFirstRecordByFilter(
				"lending_requests",
				"device = {:device} && requester = {:requester} && status = 'pending'",
				dbx.Params{"device": device.Id, "requester": e.Auth.Id},
			)
		}

		isOwner := e.Auth != nil && device.GetString("owner") == e.Auth.Id
		isAdmin := e.Auth != nil && e.Auth.GetBool("is_admin")
		status := device.GetString("status")

		// Borrower identity is only shown to authenticated users (design
		// doc requirement), never to anonymous visitors even when
		// PUBLIC_READ is enabled.
		borrowerName := ""
		if e.Auth != nil && status == "lent" {
			if borrower, err := app.FindRecordById("users", device.GetString("current_borrower")); err == nil {
				borrowerName = borrower.GetString("name")
			}
		}

		data := map[string]any{
			"Title":            device.GetString("name"),
			"CurrentUser":      e.Auth,
			"IsAdmin":          isAdmin,
			"Device":           device,
			"CategoryName":     category.GetString("name"),
			"OwnerName":        owner.GetString("name"),
			"BorrowerName":     borrowerName,
			"PhotoURL":         devicePhotoURL(device),
			"IsOwner":          isOwner || isAdmin,
			"CanRequest":       e.Auth != nil && devices.CheckRequestable(app, device, e.Auth) == nil,
			"MyPendingRequest": myPendingRequest,
			"Lat":              device.GetGeoPoint("location_point").Lat,
			"Lon":              device.GetGeoPoint("location_point").Lon,
			"LocationLabel":    device.GetString("location_label"),
			"LoginPath":        LoginPath,
		}

		if isOwner || isAdmin {
			pending, err := app.FindRecordsByFilter(
				"lending_requests",
				"device = {:device} && status = 'pending'",
				"created", 0, 0,
				dbx.Params{"device": device.Id},
			)
			if err != nil {
				return e.InternalServerError("failed to load pending requests", err)
			}

			views := make([]pendingRequestView, 0, len(pending))
			for _, p := range pending {
				requester, err := app.FindRecordById("users", p.GetString("requester"))
				if err != nil {
					return e.InternalServerError("failed to load requester", err)
				}
				views = append(views, pendingRequestView{
					ID:             p.Id,
					RequesterName:  requester.GetString("name"),
					RequesterEmail: requester.Email(),
					RequestedStart: p.GetDateTime("requested_start").String(),
					RequestedEnd:   p.GetDateTime("requested_end").String(),
					Message:        p.GetString("message"),
				})
			}
			data["PendingRequests"] = views
		}

		html, err := Render([]string{"templates/device_detail.html"}, data)
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}

		return e.HTML(http.StatusOK, html)
	}
}

func RequestDeviceHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		id := e.Request.PathValue("id")
		device, err := app.FindRecordById("devices", id)
		if err != nil {
			return e.NotFoundError("device not found", err)
		}

		if err := e.Request.ParseForm(); err != nil {
			return e.BadRequestError("invalid form data", err)
		}

		start, err := types.ParseDateTime(e.Request.FormValue("requested_start"))
		if err != nil {
			return e.BadRequestError("invalid start date", err)
		}

		var end types.DateTime
		if v := e.Request.FormValue("requested_end"); v != "" {
			end, err = types.ParseDateTime(v)
			if err != nil {
				return e.BadRequestError("invalid end date", err)
			}
		}

		if _, err := devices.CreateRequest(app, notifier, device, e.Auth, start, end, e.Request.FormValue("message")); err != nil {
			// The device page only renders the request form when the policy
			// allows it, but a direct POST has to be refused too.
			if errors.Is(err, devices.ErrNotRequestable) {
				return e.BadRequestError(err.Error(), err)
			}
			return e.InternalServerError("failed to submit request", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func WithdrawRequestHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}

		reqID := e.Request.PathValue("reqId")
		request, err := app.FindRecordById("lending_requests", reqID)
		if err != nil {
			return e.NotFoundError("request not found", err)
		}

		if request.GetString("requester") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
			return e.ForbiddenError("not your request", nil)
		}

		if err := devices.Withdraw(app, notifier, request); err != nil {
			return e.InternalServerError("failed to withdraw request", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+request.GetString("device"))
	}
}
