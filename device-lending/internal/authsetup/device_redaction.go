// internal/authsetup/device_redaction.go
package authsetup

import "github.com/pocketbase/pocketbase/core"

// anonymousHiddenDeviceFields are the devices fields that reveal who currently
// has a device. The design doc requires the borrower's identity to stay hidden
// from anonymous visitors; the HTML device page honours that itself, but with
// PUBLIC_READ=true the same record is also readable through the REST API,
// where the raw current_borrower id can be trivially de-anonymized by
// cross-referencing the public owner field of any other device.
var anonymousHiddenDeviceFields = []string{"current_borrower", "lend_start", "lend_end"}

// BindAnonymousDeviceRedaction strips the borrowing details from every devices
// record serialized for an unauthenticated requester (list, view and realtime
// responses alike). Authenticated requesters keep seeing them.
func BindAnonymousDeviceRedaction(app core.App) {
	app.OnRecordEnrich("devices").BindFunc(func(e *core.RecordEnrichEvent) error {
		if e.RequestInfo == nil || e.RequestInfo.Auth == nil {
			e.Record.Hide(anonymousHiddenDeviceFields...)
		}
		return e.Next()
	})
}
