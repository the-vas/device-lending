package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func Handover(app core.App, notifier *mail.Notifier, device, chosenRequest *core.Record, lendEnd types.DateTime) error {
	if chosenRequest.GetString("device") != device.Id {
		return fmt.Errorf("request %s does not belong to device %s", chosenRequest.Id, device.Id)
	}
	if chosenRequest.GetString("status") != "pending" {
		return fmt.Errorf("request %s is not pending", chosenRequest.Id)
	}

	others, err := pendingRequests(app, device.Id)
	if err != nil {
		return fmt.Errorf("loading pending requests: %w", err)
	}

	chosenRequest.Set("status", "accepted")
	chosenRequest.Set("decided_at", types.NowDateTime())
	if err := app.Save(chosenRequest); err != nil {
		return fmt.Errorf("accepting request: %w", err)
	}

	device.Set("status", "lent")
	device.Set("current_borrower", chosenRequest.GetString("requester"))
	device.Set("lend_start", types.NowDateTime())
	if !lendEnd.IsZero() {
		device.Set("lend_end", lendEnd)
	}
	if err := app.Save(device); err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	requester, err := app.FindRecordById("users", chosenRequest.GetString("requester"))
	if err != nil {
		return fmt.Errorf("loading requester: %w", err)
	}

	lendEndStr := ""
	if !lendEnd.IsZero() {
		lendEndStr = lendEnd.String()
	}

	logNotifyFailure(app, "handover_accepted", notifier.HandoverAccepted(requester, device, lendEndStr))

	remainingOthers := 0
	for _, other := range others {
		if other.Id != chosenRequest.Id {
			remainingOthers++
		}
	}

	for _, other := range others {
		if other.Id == chosenRequest.Id {
			continue
		}
		otherRequester, err := app.FindRecordById("users", other.GetString("requester"))
		if err != nil {
			return fmt.Errorf("loading other requester: %w", err)
		}
		logNotifyFailure(app, "device_unavailable", notifier.DeviceUnavailable(otherRequester, device, lendEndStr, remainingOthers-1))
	}

	return nil
}
