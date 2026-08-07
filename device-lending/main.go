package main

import (
	"context"
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
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/web"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

// bindBootstrap registers the app's OnBootstrap handler: it applies the app's
// own migrations and then the runtime collection configuration that depends on
// them (OIDC provider config, admin sync, public read rules).
func bindBootstrap(
	app core.App,
	cfg config.Config,
	discover func(ctx context.Context, issuer string) (oidcdiscovery.Document, error),
) {
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		// e.Next() performs the actual bootstrap (opens the databases,
		// initializes the logger, runs the *system* migrations). It has to run
		// first: before it there is no database connection at all.
		if err := e.Next(); err != nil {
			return err
		}

		// core.Bootstrap only runs core.SystemMigrations, so on a genuinely
		// fresh pb_data directory this app's own migrations (categories,
		// devices, lending_requests, users.is_admin) have not been applied
		// yet — they would otherwise only run later, from apis.Serve. Every
		// call below needs those collections to exist, so apply them here.
		// Already-applied migrations are recorded in the _migrations table,
		// so apis.Serve re-running this later is a no-op.
		if err := e.App.RunAllMigrations(); err != nil {
			return err
		}

		if err := authsetup.ConfigureOAuth2(e.App, cfg, discover); err != nil {
			return err
		}
		authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		return authsetup.ApplyReadRules(e.App, cfg.PublicRead)
	})
}

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	app := pocketbase.New()

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: true,
	})

	authsetup.RegisterOIDCScopes("groups")

	bindBootstrap(app, cfg, oidcdiscovery.Fetch)

	devices.BindStateFieldGuard(app)
	authsetup.BindAnonymousDeviceRedaction(app)

	notifier := mail.New(app, cfg.BaseURL)
	devices.BindDeleteCascade(app, notifier)

	staticSub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		signer := webauth.NewSigner(cfg.SessionSecret)

		se.Router.BindFunc(webauth.LoadSession(signer))

		se.Router.GET("/healthz", func(e *core.RequestEvent) error {
			return e.String(200, "ok")
		})
		se.Router.GET("/static/{path...}", apis.Static(staticSub, false))

		se.Router.GET("/oidc/login", webauth.LoginHandler(cfg.BaseURL, signer))
		se.Router.GET("/oidc/callback", webauth.CallbackHandler(cfg.BaseURL, signer, webauth.RouterExchanger))
		se.Router.POST("/logout", webauth.LogoutHandler())

		se.Router.GET("/", web.BrowseHandler(app, cfg.PublicRead))
		se.Router.GET("/devices/new", web.NewDeviceFormHandler(app))
		se.Router.POST("/devices/new", web.CreateDeviceHandler(app))
		se.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, cfg.PublicRead))
		se.Router.GET("/devices/{id}/edit", web.EditDeviceFormHandler(app))
		se.Router.POST("/devices/{id}/edit", web.UpdateDeviceHandler(app))
		se.Router.POST("/devices/{id}/delete", web.DeleteDeviceHandler(app))
		se.Router.POST("/devices/{id}/request", web.RequestDeviceHandler(app, notifier))
		se.Router.POST("/devices/{id}/requests/{reqId}/withdraw", web.WithdrawRequestHandler(app, notifier))
		se.Router.POST("/devices/{id}/requests/{reqId}/reject", web.RejectRequestHandler(app, notifier))
		se.Router.POST("/devices/{id}/handover", web.HandoverHandler(app, notifier))
		se.Router.POST("/devices/{id}/return", web.MarkReturnedHandler(app, notifier))
		se.Router.POST("/devices/{id}/toggle-unavailable", web.ToggleUnavailableHandler(app))

		se.Router.GET("/my/devices", web.MyDevicesHandler(app))
		se.Router.GET("/my/requests", web.MyRequestsHandler(app))

		se.Router.GET("/admin", web.AdminHandler(app))
		se.Router.POST("/admin/devices/{id}/delete", web.AdminDeleteDeviceHandler(app))
		se.Router.POST("/admin/requests/{id}/delete", web.AdminDeleteRequestHandler(app))

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
