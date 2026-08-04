package devices

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/the-vas/device-lending/internal/mail"
)

// BindDeleteCascade emails each requester with a pending lending_requests
// row before a device is deleted. The "device" relation on lending_requests
// is configured with CascadeDelete (Task 6's migration), so PocketBase
// itself deletes every lending_requests row for the device — pending,
// accepted, rejected, and withdrawn alike — as part of the device delete.
// This hook only needs to read the pending rows and notify their requesters
// before that happens; it binds to OnRecordDelete (not
// OnRecordAfterDeleteSuccess) so it runs before the cascade deletion, while
// the pending rows still exist, and so it covers both API-triggered deletes
// and direct app.Delete() calls.
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

			if err := notifier.DeviceRemoved(requester, e.Record); err != nil {
				return err
			}
		}

		return e.Next()
	})
}
