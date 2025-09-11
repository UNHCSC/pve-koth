package app

import (
	"github.com/UNHCSC/pve-koth/auth"
	"github.com/gofiber/fiber/v2"
)

func showLogin(c *fiber.Ctx) error {
	return c.Render("login", fiber.Map{"Title": "Login"}, "layout")
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

func showAdminDashboard(c *fiber.Ctx) (err error) {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string
	)

	if displayName, err = user.LDAPConn.DisplayName(); err != nil {
		return c.Render("admin", fiber.Map{
			"Title": "Dashboard",
			"User":  user.LDAPConn.Username,
			"Error": err.Error(),
		}, "layout")
	}

	return c.Render("admin", fiber.Map{
		"Title": "Dashboard",
		"User":  displayName,
	}, "layout")
}

func showUnauthorized(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	return c.Render("unauthorized", fiber.Map{
		"Title": "Unauthorized",
		"User":  user.LDAPConn.Username,
	}, "layout")
}
