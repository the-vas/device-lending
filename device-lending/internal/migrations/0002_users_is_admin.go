package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// oauth2OnlyCreateRule limits public REST record creation on the users
// collection to PocketBase's own auth-with-oauth2 flow.
//
// The OIDC login creates the user record by dispatching an internal
// POST /api/collections/users/records with the request info context set to
// "oauth2" (see apis.sendOAuth2RecordCreateRequest); that internal request
// carries the *anonymous* auth state of the callback request, so it is checked
// against this very rule and a nil CreateRule would break first-time login.
// Any real, externally-issued request carries the "default" context and is
// therefore refused.
const oauth2OnlyCreateRule = "@request.context = 'oauth2'"

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.BoolField{Name: "is_admin"})

		// is_admin is an ordinary field on an otherwise stock users
		// collection, so with PocketBase's defaults (public self-signup, and
		// self-update of your own record) anyone could hand themselves
		// is_admin = true and satisfy every "@request.auth.is_admin = true"
		// rule in this app. This app never creates, updates or deletes user
		// records through the public REST API: they come from the OIDC login
		// flow, which saves them from Go with full app-level access.
		users.CreateRule = types.Pointer(oauth2OnlyCreateRule)
		users.UpdateRule = nil
		users.DeleteRule = nil

		// The auth model is OIDC-only ("no local registration form"), so every
		// other login method on the collection is turned off. MFA has to go
		// with them: PocketBase rejects an MFA-enabled collection that has
		// fewer than two auth methods enabled.
		users.PasswordAuth.Enabled = false
		users.OTP.Enabled = false
		users.MFA.Enabled = false

		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.RemoveByName("is_admin")

		// restore PocketBase's stock users collection defaults
		users.CreateRule = types.Pointer("")
		users.UpdateRule = types.Pointer("id = @request.auth.id")
		users.DeleteRule = types.Pointer("id = @request.auth.id")
		users.PasswordAuth.Enabled = true

		return app.Save(users)
	})
}
