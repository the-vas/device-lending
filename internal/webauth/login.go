package webauth

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"golang.org/x/oauth2"
)

const PKCECookieName = "oidc_pkce"

func LoginHandler(baseURL string, signer Signer) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		users, err := e.App.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		providerConfig, ok := users.OAuth2.GetProviderConfig("oidc")
		if !ok {
			return e.InternalServerError("OIDC provider is not configured", nil)
		}

		provider, err := providerConfig.InitProvider()
		if err != nil {
			return e.InternalServerError("failed to initialize OIDC provider", err)
		}

		state := security.RandomString(30)
		codeVerifier := security.RandomString(43)
		codeChallenge := security.S256Challenge(codeVerifier)

		provider.SetRedirectURL(baseURL + "/oidc/callback")

		authURL := provider.BuildAuthURL(
			state,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)

		e.SetCookie(&http.Cookie{
			Name:     PKCECookieName,
			Value:    signer.Sign(state + "|" + codeVerifier),
			Path:     "/oidc/callback",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600,
		})

		return e.Redirect(http.StatusFound, authURL)
	}
}
