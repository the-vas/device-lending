package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func revertDeviceIfNoPending(app core.App, device *core.Record) error {
	if device.GetString("status") != "requested" {
		return nil
	}

	remaining, err := pendingRequests(app, device.Id)
	if err != nil {
		return fmt.Errorf("checking remaining pending requests: %w", err)
	}

	if len(remaining) == 0 {
		device.Set("status", "available")
		if err := app.Save(device); err != nil {
			return fmt.Errorf("reverting device status: %w", err)
		}
	}

	return nil
}

func Reject(app core.App, notifier *mail.Notifier, request *core.Record) error {
	if request.GetString("status") != "pending" {
		return fmt.Errorf("request %s is not pending", request.Id)
	}

	device, err := app.FindRecordById("devices", request.GetString("device"))
	if err != nil {
		return fmt.Errorf("loading device: %w", err)
	}

	request.Set("status", "rejected")
	request.Set("decided_at", types.NowDateTime())
	if err := app.Save(request); err != nil {
		return fmt.Errorf("rejecting request: %w", err)
	}

	if err := revertDeviceIfNoPending(app, device); err != nil {
		return err
	}

	requester, err := app.FindRecordById("users", request.GetString("requester"))
	if err != nil {
		return fmt.Errorf("loading requester: %w", err)
	}

	logNotifyFailure(app, "request_rejected", notifier.RequestRejected(requester, device))

	return nil
}
