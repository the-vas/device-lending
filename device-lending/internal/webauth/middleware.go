// internal/webauth/middleware.go
package webauth

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// LoadSession reads the session cookie (if present and valid) and sets
// e.Auth to the corresponding user record, so downstream handlers and
// templates can tell who's logged in. Never blocks the request.
func LoadSession(signer Signer) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		cookie, err := e.Request.Cookie(SessionCookieName)
		if err != nil {
			return e.Next()
		}

		token, err := signer.Verify(cookie.Value)
		if err != nil {
			return e.Next()
		}

		record, err := e.App.FindAuthRecordByToken(token, core.TokenTypeAuth)
		if err != nil {
			return e.Next()
		}

		e.Auth = record
		return e.Next()
	}
}

// RequireWebAuth redirects to loginPath if e.Auth is nil.
func RequireWebAuth(loginPath string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, loginPath)
		}
		return e.Next()
	}
}

func LogoutHandler() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		e.SetCookie(&http.Cookie{
			Name:   SessionCookieName,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		return e.Redirect(http.StatusFound, "/")
	}
}
