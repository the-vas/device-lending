package authsetup

import (
	"context"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/auth"

	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
)

func RegisterOIDCScopes(extra ...string) {
	auth.Providers[auth.NameOIDC] = func() auth.Provider {
		p := auth.NewOIDCProvider()
		p.SetScopes(append(p.Scopes(), extra...))
		return p
	}
}

// discoveryTimeout bounds the synchronous OIDC discovery call made during
// bootstrap. Discovery failing is deliberately fatal — the app cannot serve
// logins without a provider config — but a hung or very slow IdP must not
// block startup indefinitely, which a bare context.Background() would allow.
//
// It is a variable only so the tests can shorten it.
var discoveryTimeout = 15 * time.Second

func ConfigureOAuth2(
	app core.App,
	cfg config.Config,
	discover func(ctx context.Context, issuer string) (oidcdiscovery.Document, error),
) error {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()

	doc, err := discover(ctx, cfg.OIDCIssuer)
	if err != nil {
		return fmt.Errorf("discovering OIDC issuer %q: %w", cfg.OIDCIssuer, err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("loading users collection: %w", err)
	}

	users.OAuth2.Enabled = true

	newProvider := core.OAuth2ProviderConfig{
		Name:         "oidc",
		ClientId:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		AuthURL:      doc.AuthorizationEndpoint,
		TokenURL:     doc.TokenEndpoint,
		UserInfoURL:  doc.UserinfoEndpoint,
		DisplayName:  "Log in",
	}

	replaced := false
	for i, p := range users.OAuth2.Providers {
		if p.Name == "oidc" {
			users.OAuth2.Providers[i] = newProvider
			replaced = true
			break
		}
	}
	if !replaced {
		users.OAuth2.Providers = append(users.OAuth2.Providers, newProvider)
	}

	return app.Save(users)
}
