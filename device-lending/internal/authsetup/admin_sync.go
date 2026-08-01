// internal/authsetup/admin_sync.go
package authsetup

import "github.com/pocketbase/pocketbase/core"

func HasAdminGroup(rawUser map[string]any, adminGroup string) bool {
	if adminGroup == "" {
		return false
	}

	raw, ok := rawUser["groups"]
	if !ok {
		return false
	}

	groups, ok := raw.([]any)
	if !ok {
		return false
	}

	for _, g := range groups {
		if s, ok := g.(string); ok && s == adminGroup {
			return true
		}
	}

	return false
}

func BindAdminSync(app core.App, adminGroup string) {
	app.OnRecordAuthWithOAuth2Request("users").BindFunc(func(e *core.RecordAuthWithOAuth2RequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}

		if e.Record == nil {
			return nil
		}

		isAdmin := HasAdminGroup(e.OAuth2User.RawUser, adminGroup)
		if e.Record.GetBool("is_admin") == isAdmin {
			return nil
		}

		e.Record.Set("is_admin", isAdmin)
		return e.App.Save(e.Record)
	})
}
