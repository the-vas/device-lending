// internal/devices/notify.go
package devices

import "github.com/pocketbase/pocketbase/core"

// logNotifyFailure records a failed notification email without turning it into
// a failure of the surrounding operation.
//
// The database writes of every workflow action are already committed by the
// time the notifier runs, so returning the mailer's error would report a
// misleading failure for a change that did take effect. It is not a corner
// case either: PocketBase's default mailer is sendmail, which does not exist
// in the deployed Alpine image, so until an operator configures SMTP every
// single send fails. The state change is the operation's real effect; the
// email is a best-effort side notification.
func logNotifyFailure(app core.App, action string, err error) {
	if err == nil {
		return
	}

	app.Logger().Error(
		"failed to send notification email; the action itself succeeded",
		"action", action,
		"error", err.Error(),
	)
}
