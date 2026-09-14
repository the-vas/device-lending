// internal/webauth/callback.go
package webauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const SessionCookieName = "pb_session"

type ExchangeResult struct {
	Token  string
	Record map[string]any
}

// Exchanger exchanges an authorization code for a token + user record.
type Exchanger func(app core.App, code, codeVerifier, redirectURL string) (ExchangeResult, error)

// RouterExchanger dispatches the auth-with-oauth2 exchange in-process
// through PocketBase's own router, reusing its official record
// find-or-create and verification logic without a real network call.
func RouterExchanger(app core.App, code, codeVerifier, redirectURL string) (ExchangeResult, error) {
	router, err := apis.NewRouter(app)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("building router: %w", err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("building mux: %w", err)
	}

	body, err := json.Marshal(map[string]string{
		"provider":     "oidc",
		"code":         code,
		"codeVerifier": codeVerifier,
		"redirectURL":  redirectURL,
	})
	if err != nil {
		return ExchangeResult{}, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/auth-with-oauth2", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return ExchangeResult{}, fmt.Errorf("oauth2 exchange failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		Token  string         `json:"token"`
		Record map[string]any `json:"record"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		return ExchangeResult{}, fmt.Errorf("parsing oauth2 exchange response: %w", err)
	}

	return ExchangeResult{Token: parsed.Token, Record: parsed.Record}, nil
}

func CallbackHandler(baseURL string, signer Signer, exchange Exchanger) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		code := e.Request.URL.Query().Get("code")
		state := e.Request.URL.Query().Get("state")
		if code == "" || state == "" {
			return e.BadRequestError("missing code or state", nil)
		}

		cookie, err := e.Request.Cookie(PKCECookieName)
		if err != nil {
			return e.BadRequestError("missing pkce cookie; please retry login", nil)
		}

		raw, err := signer.Verify(cookie.Value)
		if err != nil {
			return e.BadRequestError("invalid pkce cookie", nil)
		}

		parts := strings.SplitN(raw, "|", 2)
		if len(parts) != 2 || parts[0] != state {
			return e.BadRequestError("state mismatch", nil)
		}
		codeVerifier := parts[1]

		result, err := exchange(e.App, code, codeVerifier, baseURL+"/oidc/callback")
		if err != nil {
			return e.InternalServerError("oidc token exchange failed", err)
		}
		if result.Token == "" {
			return e.InternalServerError("oidc exchange returned no token", nil)
		}

		e.SetCookie(&http.Cookie{
			Name:     SessionCookieName,
			Value:    signer.Sign(result.Token),
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24 * 5,
		})

		e.SetCookie(&http.Cookie{
			Name:   PKCECookieName,
			Value:  "",
			Path:   "/oidc/callback",
			MaxAge: -1,
		})

		return e.Redirect(http.StatusFound, "/")
	}
}
