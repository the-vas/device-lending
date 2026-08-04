package devices

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

// BindDeleteCascade auto-rejects any pending lending_requests for a device
// before that device is deleted, and emails each affected requester. It
// binds to OnRecordDelete (not OnRecordAfterDeleteSuccess) so it covers both
// API-triggered deletes and direct app.Delete() calls, and so it runs before
// PocketBase's built-in relation-integrity check: the "device" relation on
// lending_requests is a required field (Task 6's migration), so PocketBase
// refuses to delete a device that any lending_requests row still points to.
// Clearing the "device" reference on the pending rows here — in addition to
// rejecting them — lets the subsequent delete proceed instead of failing
// with "record cannot be deleted because it is part of a required
// reference".
func BindDeleteCascade(app core.App, notifier *mail.Notifier) {
	app.OnRecordDelete("devices").BindFunc(func(e *core.RecordEvent) error {
		pending, err := pendingRequests(e.App, e.Record.Id)
		if err != nil {
			return err
		}

		for _, req := range pending {
			requester, err := e.App.FindRecordById("users", req.GetString("requester"))
			if err != nil {
				return err
			}

			req.Set("status", "rejected")
			req.Set("decided_at", types.NowDateTime())
			req.Set("device", "")
			if err := e.App.SaveNoValidate(req); err != nil {
				return err
			}

			if err := notifier.DeviceRemoved(requester, e.Record); err != nil {
				return err
			}
		}

		return e.Next()
	})
}
