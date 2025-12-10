package app

import (
	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/gofiber/fiber/v2"
)

func showLanding(c *fiber.Ctx) (err error) {
	var (
		user  *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		comps []*db.Competition
	)

	if comps, err = db.Competitions.SelectAll(); err != nil {
		return c.Render("landing", bindWithLocals(c, fiber.Map{
			"Title": "Home",
			"Error": "Failed to load competitions. Check the server logs.",
		}), "layout")
	}

	var (
		totalTeams          int
		publicCompetitions  int
		privateCompetitions int
	)

	for _, comp := range comps {
		totalTeams += len(comp.TeamIDs)
		if comp.IsPrivate {
			privateCompetitions++
		} else {
			publicCompetitions++
		}
	}

	var displayName string = ""
	if user != nil {
		if displayName, err = user.LDAPConn.DisplayName(); err != nil {
			displayName = user.LDAPConn.Username
		}
	}

	return c.Render("landing", bindWithLocals(c, fiber.Map{
		"Title":    "Home",
		"LoggedIn": user != nil,
		"User":     displayName,
		"Stats": fiber.Map{
			"TotalCompetitions":   len(comps),
			"PublicCompetitions":  publicCompetitions,
			"PrivateCompetitions": privateCompetitions,
			"TotalTeams":          totalTeams,
		},
	}), "layout")
}

func showLogin(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	return c.Render("login", bindWithLocals(c, fiber.Map{
		"Title":    "Login",
		"LoggedIn": user != nil,
		"User": func() string {
			if user == nil {
				return ""
			}

			if display, err := user.LDAPConn.DisplayName(); err == nil {
				return display
			}

			return user.LDAPConn.Username
		}(),
	}), "layout")
}

func showLogout(c *fiber.Ctx) error {
	c.ClearCookie("Authorization")
	return c.Redirect("/login")
}

func showDashboard(c *fiber.Ctx) (err error) {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string
		canManage   bool
	)

	if user != nil {
		canManage = user.Permissions() >= auth.AuthPermsAdministrator
		if displayName, err = user.LDAPConn.DisplayName(); err != nil {
			return c.Render("dashboard", bindWithLocals(c, fiber.Map{
				"Title": "Dashboard",
				"User":  user.LDAPConn.Username,
				"Error": err.Error(),
			}), "layout")
		}
	} else {
		displayName = "Guest"
	}

	return c.Render("dashboard", bindWithLocals(c, fiber.Map{
		"Title":     "Dashboard",
		"User":      displayName,
		"LoggedIn":  user != nil,
		"CanManage": canManage,
	}), "layout")
}

func showUnauthorized(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	return c.Render("unauthorized", bindWithLocals(c, fiber.Map{
		"Title":    "Unauthorized",
		"LoggedIn": user != nil,
		"User": func() string {
			if user == nil || user.LDAPConn == nil {
				return "Guest"
			}
			return user.LDAPConn.Username
		}(),
	}), "layout")
}

func showScoreboard(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	var displayName string
	if user != nil {
		if displayName, err = user.LDAPConn.DisplayName(); err != nil {
			displayName = user.LDAPConn.Username
		}
	}

	return c.Render("scoreboard", bindWithLocals(c, fiber.Map{
		"Title":                 "Scoreboard",
		"LoggedIn":              user != nil,
		"User":                  displayName,
		"CanManage":             user != nil && user.Permissions() >= auth.AuthPermsAdministrator,
		"SelectedCompetitionID": c.Params("competitionID"),
	}), "layout")
}
