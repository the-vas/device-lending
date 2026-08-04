package main

import (
	"io/fs"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
	_ "github.com/the-vas/device-lending/internal/migrations"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/web"
	"github.com/the-vas/device-lending/internal/webauth"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	log.Printf("config loaded: base_url=%s public_read=%v", cfg.BaseURL, cfg.PublicRead)

	app := pocketbase.New()

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: true,
	})

	staticSub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		signer := webauth.NewSigner(cfg.SessionSecret)

		se.Router.BindFunc(webauth.LoadSession(signer))
		se.Router.GET("/oidc/login", webauth.LoginHandler(cfg.BaseURL, signer))
		se.Router.GET("/oidc/callback", webauth.CallbackHandler(cfg.BaseURL, signer, webauth.RouterExchanger))
		se.Router.POST("/logout", webauth.LogoutHandler())

		se.Router.GET("/static/{path...}", apis.Static(staticSub, false))

		se.Router.GET("/healthz", func(e *core.RequestEvent) error {
			return e.String(200, "ok")
		})

		return se.Next()
	})

	authsetup.RegisterOIDCScopes("groups")

	devices.BindStateFieldGuard(app)

	notifier := mail.New(app, cfg.BaseURL)
	devices.BindDeleteCascade(app, notifier)

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := authsetup.ConfigureOAuth2(e.App, cfg, oidcdiscovery.Fetch); err != nil {
			return err
		}
		authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		if err := authsetup.ApplyReadRules(e.App, cfg.PublicRead); err != nil {
			return err
		}
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
