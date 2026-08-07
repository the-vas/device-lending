package devices

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

// pendingRequests returns all "pending" lending_requests for a device, newest first.
func pendingRequests(app core.App, deviceID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter(
		"lending_requests",
		"device = {:device} && status = 'pending'",
		"-created",
		0, 0,
		dbx.Params{"device": deviceID},
	)
}

// ErrNotRequestable reports that the request-eligibility policy refuses a new
// lending request. Callers should surface it as a 400, not a 500.
var ErrNotRequestable = errors.New("device cannot be requested")

// CheckRequestable returns nil when requester is allowed to open a new pending
// request for device, and an error wrapping [ErrNotRequestable] otherwise.
//
// It is the single source of truth for the policy: the device detail page uses
// it to decide whether to render the request form, and CreateRequest enforces
// it so a direct POST cannot bypass the UI.
func CheckRequestable(app core.App, device, requester *core.Record) error {
	if requester.Id == device.GetString("owner") {
		return fmt.Errorf("%w: you cannot request your own device", ErrNotRequestable)
	}

	switch device.GetString("status") {
	case "available", "requested":
	default:
		return fmt.Errorf("%w: %q is not available to borrow", ErrNotRequestable, device.GetString("name"))
	}

	existing, err := app.FindFirstRecordByFilter(
		"lending_requests",
		"device = {:device} && requester = {:requester} && status = 'pending'",
		dbx.Params{"device": device.Id, "requester": requester.Id},
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking for an existing request: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("%w: you already have a pending request for this device", ErrNotRequestable)
	}

	return nil
}

// CreateRequest creates a pending lending_requests record, transitions the device
// from "available" to "requested" if needed, and emails the owner.
func CreateRequest(
	app core.App,
	notifier *mail.Notifier,
	device, requester *core.Record,
	requestedStart, requestedEnd types.DateTime,
	message string,
) (*core.Record, error) {
	if err := CheckRequestable(app, device, requester); err != nil {
		return nil, err
	}

	col, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		return nil, err
	}

	request := core.NewRecord(col)
	request.Set("device", device.Id)
	request.Set("requester", requester.Id)
	request.Set("status", "pending")
	request.Set("requested_start", requestedStart)
	if !requestedEnd.IsZero() {
		request.Set("requested_end", requestedEnd)
	}
	request.Set("message", message)

	if err := app.Save(request); err != nil {
		return nil, fmt.Errorf("saving request: %w", err)
	}

	if device.GetString("status") == "available" {
		device.Set("status", "requested")
		if err := app.Save(device); err != nil {
			return nil, fmt.Errorf("updating device status: %w", err)
		}
	}

	owner, err := app.FindRecordById("users", device.GetString("owner"))
	if err != nil {
		return nil, fmt.Errorf("loading device owner: %w", err)
	}

	logNotifyFailure(app, "create_request", notifier.NewRequest(owner, device, requester))

	return request, nil
}
