// internal/authsetup/oauth2_timeout_test.go
package authsetup

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

// TestConfigureOAuth2_DiscoveryTimesOut asserts the bootstrap-time discovery
// call is bounded. Without a timeout a hung IdP holds the OnBootstrap hook (and
// therefore startup) open forever instead of failing fast.
func TestConfigureOAuth2_DiscoveryTimesOut(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	released := make(chan struct{})

	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hang until the client gives up (or the test ends).
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	// Release the handler *before* closing the server: httptest.Server.Close
	// waits for outstanding requests, so on failure (no timeout, request never
	// cancelled) the reverse order would deadlock instead of reporting.
	defer func() {
		close(released)
		idp.Close()
	}()

	original := discoveryTimeout
	discoveryTimeout = 150 * time.Millisecond
	defer func() { discoveryTimeout = original }()

	cfg := config.Config{
		OIDCIssuer:       idp.URL,
		OIDCClientID:     "abc",
		OIDCClientSecret: "secret",
	}

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- ConfigureOAuth2(app, cfg, oidcdiscovery.Fetch)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected ConfigureOAuth2 to fail against a hanging IdP")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("expected a bounded wait, took %s", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ConfigureOAuth2 hung: discovery is not bounded by a timeout")
	}
}

// TestConfigureOAuth2_DefaultTimeoutIsBounded pins the production default, which
// the timeout test above deliberately overrides.
func TestConfigureOAuth2_DefaultTimeoutIsBounded(t *testing.T) {
	if discoveryTimeout <= 0 || discoveryTimeout > 30*time.Second {
		t.Errorf("expected a small positive default discovery timeout, got %s", discoveryTimeout)
	}
}
