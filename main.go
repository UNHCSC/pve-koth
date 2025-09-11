package main

import (
	"crypto/rand"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/z46-dev/go-logger"
)

var mainLog *logger.Logger = logger.NewLogger().SetPrefix("[KOTH]", logger.BoldBlue)
var jwtSigningKey []byte = make([]byte, 64)

func main() {
	var err error

	if _, err = rand.Read(jwtSigningKey); err != nil {
		mainLog.Errorf("failed to generate JWT signing key: %v\n", err)
		return
	}

	if err = config.InitEnv(".env"); err != nil {
		mainLog.Errorf("failed to initialize environment: %v\n", err)
		return
	}

	if err = db.Init(); err != nil {
		mainLog.Errorf("failed to initialize database: %v\n", err)
		return
	}

	var app = fiber.New(fiber.Config{
		Views: html.New("./public/views", ".html"),
	})

	// Pages
	app.Static("/static", "./public/static")
	app.Get("/login", showLogin)
	app.Get("/logout", showLogout)
	app.Get("/dashboard", mustBeLoggedIn, showDashboard)

	// API
	app.Post("/api/auth/login", apiLogin)
	app.Post("/api/auth/logout", mustBeLoggedIn, apiLogout)

	mainLog.Errorf("fiber error: %v\n", app.Listen(config.Config.WebServer.Address))
}

// Middleware

func mustBeLoggedIn(c *fiber.Ctx) error {
	if auth.IsAuthenticated(c, jwtSigningKey) == nil {
		return c.Redirect("/login")
	}

	return c.Next()
}

// Page Handlers

func showLogin(c *fiber.Ctx) error {
	return c.Render("login", fiber.Map{
		"Title": "Login",
	}, "layout")
}

func showLogout(c *fiber.Ctx) error {
	c.ClearCookie("Authorization")
	return c.Redirect("/login")
}

func showDashboard(c *fiber.Ctx) (err error) {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string
	)

	if displayName, err = user.LDAPConn.DisplayName(); err != nil {
		return c.Render("dashboard", fiber.Map{
			"Title": "Dashboard",
			"User":  user.LDAPConn.Username,
			"Error": err.Error(),
		}, "layout")
	}

	return c.Render("dashboard", fiber.Map{
		"Title": "Dashboard",
		"User":  displayName,
	}, "layout")
}

// API Handlers

func apiLogin(c *fiber.Ctx) (err error) {
	var (
		username, password string = c.FormValue("username"), c.FormValue("password")
		user               *auth.AuthUser
		token              string
	)

	if user, err = auth.Authenticate(username, password); err == nil {
		if token, err = user.Token.SignedString(jwtSigningKey); err == nil {
			c.Cookie(&fiber.Cookie{
				Name:  "Authorization",
				Value: token,
			})

			return c.Redirect("/dashboard")
		}
	}

	return c.Render("login", fiber.Map{
		"Title": "Login",
		"Error": err.Error(),
	}, "layout")
}

func apiLogout(c *fiber.Ctx) (err error) {
	c.ClearCookie("Authorization")
	return
}
