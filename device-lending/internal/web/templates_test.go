// internal/web/templates_test.go
package web_test

import (
	"strings"
	"testing"

	"github.com/the-vas/device-lending/internal/web"
)

func TestRender_LayoutWithContent(t *testing.T) {
	// index.html isn't written until Task 24; this test exercises the
	// layout alone via a minimal inline content template registered
	// through the same embedded filesystem is not possible here, so it
	// instead asserts on the layout's always-present chrome by rendering
	// with no CurrentUser and checking the anonymous nav state.
	html, err := web.Render(nil, map[string]any{
		"Title":       "Browse",
		"CurrentUser": nil,
		"IsAdmin":     false,
		"LoginPath":   "/oidc/login",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "Device Lending") {
		t.Error("expected title to contain 'Device Lending'")
	}
	if !strings.Contains(html, `href="/oidc/login"`) {
		t.Error("expected anonymous nav to show a login link")
	}
	if strings.Contains(html, "/my/devices") {
		t.Error("expected anonymous nav to NOT show 'My devices'")
	}
}

func TestRender_LayoutHonorsConfiguredLoginPath(t *testing.T) {
	defer func() { web.LoginPath = "/oidc/login" }()
	web.LoginPath = "/dev/login"

	html, err := web.Render(nil, map[string]any{
		"Title":       "Browse",
		"CurrentUser": nil,
		"IsAdmin":     false,
		"LoginPath":   web.LoginPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `href="/dev/login"`) {
		t.Error("expected anonymous nav to link to the configured LoginPath")
	}
}
