package authsetup

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func ApplyReadRules(app core.App, publicRead bool) error {
	rule := `@request.auth.id != ""`
	if publicRead {
		rule = ""
	}

	devices, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		return err
	}

	devices.ListRule = types.Pointer(rule)
	devices.ViewRule = types.Pointer(rule)

	return app.Save(devices)
}
