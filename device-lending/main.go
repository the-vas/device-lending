package main

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	_ "github.com/the-vas/device-lending/internal/migrations"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
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

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/healthz", func(e *core.RequestEvent) error {
			return e.String(200, "ok")
		})
		return se.Next()
	})

	authsetup.RegisterOIDCScopes("groups")

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
