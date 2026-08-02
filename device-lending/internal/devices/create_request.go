package devices

import (
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

// CreateRequest creates a pending lending_requests record, transitions the device
// from "available" to "requested" if needed, and emails the owner.
func CreateRequest(
	app core.App,
	notifier *mail.Notifier,
	device, requester *core.Record,
	requestedStart, requestedEnd types.DateTime,
	message string,
) (*core.Record, error) {
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

	if err := notifier.NewRequest(owner, device, requester); err != nil {
		return nil, fmt.Errorf("sending notification: %w", err)
	}

	return request, nil
}
