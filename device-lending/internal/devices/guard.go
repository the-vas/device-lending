// Package devices contains hooks and services enforcing invariants on the
// devices collection's lending state machine.
package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

var guardedFields = []string{"status", "current_borrower", "lend_start", "lend_end"}

// BindStateFieldGuard registers a request hook on the devices collection that
// rejects direct API edits to the lending state-machine fields. Those fields
// may only be changed via the dedicated service-layer functions (Tasks
// 17-21), which call app.Save() directly and bypass this request-scoped hook.
func BindStateFieldGuard(app core.App) {
	app.OnRecordUpdateRequest("devices").BindFunc(func(e *core.RecordRequestEvent) error {
		original, err := e.App.FindRecordById("devices", e.Record.Id)
		if err != nil {
			return err
		}

		for _, field := range guardedFields {
			if fmt.Sprint(e.Record.Get(field)) != fmt.Sprint(original.Get(field)) {
				return e.BadRequestError(
					fmt.Sprintf("field %q can only be changed via the dedicated action endpoints, not a direct update", field),
					nil,
				)
			}
		}

		return e.Next()
	})
}
