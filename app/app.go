package app

import (
	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

func StartApp() error {
	var app = fiber.New(fiber.Config{
		Views: html.New("./public/views", ".html"),
	})

	// Pages
	app.Static("/static", "./public/static")
	app.Get("/", func(c *fiber.Ctx) error {
		if auth.IsAuthenticated(c, jwtSigningKey) != nil {
			return c.Redirect("/dashboard")
		}

		return c.Redirect("/login")
	})

	app.Get("/login", showLogin)
	app.Get("/logout", showLogout)
	app.Get("/dashboard", mustBeLoggedIn, showDashboard)
	app.Get("/admin", mustBeLoggedIn, mustBeAdmin, showAdminDashboard)
	app.Get("/unauthorized", mustBeLoggedIn, showUnauthorized)

	// API
	app.Post("/api/auth/login", apiLogin)
	app.Post("/api/auth/logout", mustBeLoggedIn, apiLogout)

	if config.Config.WebServer.TlsDir != "" {
		return app.ListenTLS(config.Config.WebServer.Address, config.Config.WebServer.TlsDir+"/fullchain.pem", config.Config.WebServer.TlsDir+"/privkey.pem")
	}

	return app.Listen(config.Config.WebServer.Address)
}
