package app

import (
	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/gofiber/fiber/v2"
	"strings"
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

	comps, compsErr := db.Competitions.SelectAll()
	if compsErr != nil {
		appLog.Errorf("failed to load competitions for dashboard resources: %v\n", compsErr)
		comps = nil
	}

	return c.Render("dashboard", bindWithLocals(c, fiber.Map{
		"Title":        "Dashboard",
		"User":         displayName,
		"LoggedIn":     user != nil,
		"CanManage":    canManage,
		"ResourceInfo": fiber.Map{"Restrictions": config.Config.ContainerRestrictions, "Network": buildNetworkResourceStats(comps)},
	}), "layout")
}

func buildNetworkResourceStats(comps []*db.Competition) fiber.Map {
	network := config.Config.Network
	info := fiber.Map{
		"PoolCIDR":          network.PoolCIDR,
		"CompetitionPrefix": network.CompetitionSubnetPrefix,
		"TeamPrefix":        network.TeamSubnetPrefix,
		"ContainerCIDR":     network.ContainerCIDR,
		"Gateway":           network.ContainerGateway,
		"Nameserver":        network.ContainerNameserver,
		"SearchDomain":      network.ContainerSearchDomain,
		"TotalSubnets":      0,
		"UsedSubnets":       0,
		"FreeSubnets":       0,
		"UsagePercent":      0.0,
	}

	pool := network.ParsedPool()
	if pool == nil {
		return info
	}

	maskOnes, _ := pool.Mask.Size()
	diff := network.CompetitionSubnetPrefix - maskOnes
	total := 0
	if diff >= 0 && diff < 63 {
		total = 1 << diff
	}

	seen := map[string]struct{}{}
	for _, comp := range comps {
		if comp == nil {
			continue
		}
		cidr := strings.TrimSpace(comp.NetworkCIDR)
		if cidr == "" {
			continue
		}
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
	}

	used := len(seen)
	free := total - used
	if free < 0 {
		free = 0
	}

	usage := 0.0
	if total > 0 {
		usage = (float64(used) / float64(total)) * 100
		if usage > 100 {
			usage = 100
		}
	}

	info["TotalSubnets"] = total
	info["UsedSubnets"] = used
	info["FreeSubnets"] = free
	info["UsagePercent"] = usage
	return info
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
