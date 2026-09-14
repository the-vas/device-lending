package devauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/the-vas/device-lending/internal/webauth"
)

// AuthWithPassword dispatches PocketBase's own auth-with-password endpoint
// in-process, the same technique webauth.RouterExchanger uses for
// auth-with-oauth2 — reusing PocketBase's official password-verification
// logic without a real network call.
func AuthWithPassword(app core.App, identity, password string) (webauth.ExchangeResult, error) {
	router, err := apis.NewRouter(app)
	if err != nil {
		return webauth.ExchangeResult{}, fmt.Errorf("building router: %w", err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		return webauth.ExchangeResult{}, fmt.Errorf("building mux: %w", err)
	}

	body, err := json.Marshal(map[string]string{
		"identity": identity,
		"password": password,
	})
	if err != nil {
		return webauth.ExchangeResult{}, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/auth-with-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return webauth.ExchangeResult{}, fmt.Errorf("password auth failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		Token  string         `json:"token"`
		Record map[string]any `json:"record"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		return webauth.ExchangeResult{}, fmt.Errorf("parsing auth-with-password response: %w", err)
	}

	return webauth.ExchangeResult{Token: parsed.Token, Record: parsed.Record}, nil
}

const devLoginPage = `<!DOCTYPE html>
<html>
<head><title>Dev login</title></head>
<body>
<h1>Dev login (DEV_AUTH mode)</h1>
<form method="post" action="/dev/login/user"><button type="submit">Log in as regular user</button></form>
<form method="post" action="/dev/login/admin"><button type="submit">Log in as admin</button></form>
</body>
</html>`

// LoginPageHandler renders the two-button dev login page.
func LoginPageHandler() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		return e.HTML(http.StatusOK, devLoginPage)
	}
}

// LoginAsHandler logs in as the given fixed identity/password via
// AuthWithPassword and sets the same signed session cookie the real OIDC
// callback (webauth.CallbackHandler) sets, so downstream session handling is
// identical regardless of how the token was obtained.
func LoginAsHandler(identity, password string, signer webauth.Signer) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		result, err := AuthWithPassword(e.App, identity, password)
		if err != nil {
			return e.InternalServerError("dev login failed", err)
		}
		if result.Token == "" {
			return e.InternalServerError("dev login returned no token", nil)
		}

		e.SetCookie(&http.Cookie{
			Name:     webauth.SessionCookieName,
			Value:    signer.Sign(result.Token),
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24 * 5,
		})

		return e.Redirect(http.StatusFound, "/")
	}
}
