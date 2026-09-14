package authsetup

import "github.com/pocketbase/pocketbase/core"

// AppName is shown in the PocketBase admin UI, its browser tab title, and
// default email templates, replacing PocketBase's "Acme" placeholder.
const AppName = "device-lending"

// ApplyAppName sets the PocketBase application name. Idempotent: safe to
// call on every boot.
func ApplyAppName(app core.App) error {
	settings := app.Settings()
	settings.Meta.AppName = AppName
	return app.Save(settings)
}
