// internal/authsetup/admin_sync_test.go
package authsetup_test

import (
	"testing"

	"github.com/the-vas/device-lending/internal/authsetup"
)

func TestHasAdminGroup(t *testing.T) {
	cases := []struct {
		name       string
		rawUser    map[string]any
		adminGroup string
		want       bool
	}{
		{"empty admin group disables check", map[string]any{"groups": []any{"admin"}}, "", false},
		{"no groups claim", map[string]any{}, "admin", false},
		{"groups claim wrong type", map[string]any{"groups": "admin"}, "admin", false},
		{"group present", map[string]any{"groups": []any{"users", "admin"}}, "admin", true},
		{"group absent", map[string]any{"groups": []any{"users"}}, "admin", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := authsetup.HasAdminGroup(c.rawUser, c.adminGroup)
			if got != c.want {
				t.Errorf("HasAdminGroup(%v, %q) = %v, want %v", c.rawUser, c.adminGroup, got, c.want)
			}
		})
	}
}
