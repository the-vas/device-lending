package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"github.com/the-vas/device-lending/internal/mail"
)

func MarkReturned(app core.App, notifier *mail.Notifier, device *core.Record) error {
	if device.GetString("status") != "lent" {
		return fmt.Errorf("device %s is not currently lent", device.Id)
	}

	remaining, err := pendingRequests(app, device.Id)
	if err != nil {
		return fmt.Errorf("loading pending requests: %w", err)
	}

	device.Set("current_borrower", "")
	device.Set("lend_start", "")
	device.Set("lend_end", "")
	if len(remaining) > 0 {
		device.Set("status", "requested")
	} else {
		device.Set("status", "available")
	}
	if err := app.Save(device); err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	for _, req := range remaining {
		requester, err := app.FindRecordById("users", req.GetString("requester"))
		if err != nil {
			return fmt.Errorf("loading requester: %w", err)
		}
		logNotifyFailure(app, "device_available_again", notifier.DeviceAvailableAgain(requester, device))
	}

	return nil
}
