package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func Withdraw(app core.App, notifier *mail.Notifier, request *core.Record) error {
	if request.GetString("status") != "pending" {
		return fmt.Errorf("request %s is not pending", request.Id)
	}

	device, err := app.FindRecordById("devices", request.GetString("device"))
	if err != nil {
		return fmt.Errorf("loading device: %w", err)
	}

	request.Set("status", "withdrawn")
	request.Set("decided_at", types.NowDateTime())
	if err := app.Save(request); err != nil {
		return fmt.Errorf("withdrawing request: %w", err)
	}

	if err := revertDeviceIfNoPending(app, device); err != nil {
		return err
	}

	owner, err := app.FindRecordById("users", device.GetString("owner"))
	if err != nil {
		return fmt.Errorf("loading owner: %w", err)
	}
	requester, err := app.FindRecordById("users", request.GetString("requester"))
	if err != nil {
		return fmt.Errorf("loading requester: %w", err)
	}

	logNotifyFailure(app, "request_withdrawn", notifier.RequestWithdrawn(owner, device, requester))

	return nil
}
