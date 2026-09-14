package devauth

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Fixed dev-only credentials. Only ever reachable when config.DevAuth is
// true, which config.Load has already confirmed requires a localhost
// BASE_URL — see internal/config.requireLocalBaseURL. Not configurable by
// design: the /dev/login page's one-click buttons remove any need to know
// or type them.
const (
	UserEmail     = "dev-user@example.com"
	UserPassword  = "dev-user-password"
	AdminEmail    = "dev-admin@example.com"
	AdminPassword = "dev-admin-password"
)

// Setup enables password login on the users collection and seeds two fixed
// accounts (a regular user and an admin) so local development never needs a
// real OIDC provider. Idempotent: safe to call on every boot.
func Setup(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("loading users collection: %w", err)
	}

	users.PasswordAuth.Enabled = true
	if err := app.Save(users); err != nil {
		return fmt.Errorf("enabling password auth: %w", err)
	}

	if err := seedUser(app, users, UserEmail, UserPassword, false); err != nil {
		return err
	}
	if err := seedUser(app, users, AdminEmail, AdminPassword, true); err != nil {
		return err
	}

	return nil
}

func seedUser(app core.App, users *core.Collection, email, password string, isAdmin bool) error {
	if existing, err := app.FindAuthRecordByEmail(users, email); err == nil && existing != nil {
		return nil
	}

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword(password)
	record.SetVerified(true)
	record.Set("is_admin", isAdmin)

	if err := app.Save(record); err != nil {
		return fmt.Errorf("seeding dev user %q: %w", email, err)
	}
	return nil
}
