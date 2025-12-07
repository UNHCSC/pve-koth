package app

import (
	"archive/zip"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/gofiber/fiber/v2"
)

type competitionSummary struct {
	ID             int64     `json:"id"`
	CompetitionID  string    `json:"competitionID"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Host           string    `json:"host"`
	TeamCount      int       `json:"teamCount"`
	ContainerCount int       `json:"containerCount"`
	IsPrivate      bool      `json:"isPrivate"`
	CreatedAt      time.Time `json:"createdAt"`
}

type scoreboardTeam struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Score       int       `json:"score"`
	LastUpdated time.Time `json:"lastUpdated"`
}

type scoreboardCompetition struct {
	CompetitionID  string           `json:"competitionID"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Host           string           `json:"host"`
	TeamCount      int              `json:"teamCount"`
	ContainerCount int              `json:"containerCount"`
	IsPrivate      bool             `json:"isPrivate"`
	Teams          []scoreboardTeam `json:"teams"`
}

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
		"Title":      "Login",
		"LoginError": err.Error(),
	}, "layout")
}

func apiLogout(c *fiber.Ctx) (err error) {
	c.ClearCookie("Authorization")
	return
}

func apiGetCompetitions(c *fiber.Ctx) (err error) {
	var (
		retrievedCompetitions []*db.Competition
		visible               []competitionSummary
		user                  *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if retrievedCompetitions, err = db.Competitions.SelectAll(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competitions")
	}

	var groups []string = fetchUserGroups(user)

	for _, comp := range retrievedCompetitions {
		if !userCanViewCompetition(user, groups, comp) {
			continue
		}

		visible = append(visible, summarizeCompetition(comp))
	}

	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].CreatedAt.After(visible[j].CreatedAt)
	})

	return c.JSON(fiber.Map{
		"competitions": visible,
	})
}

func apiCreateCompetition(c *fiber.Ctx) (err error) {
	var (
		fHeader       *multipart.FileHeader
		zipReadCloser *zip.ReadCloser
	)

	if fHeader, err = c.FormFile("file"); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}

	var tmpPath = filepath.Join(os.TempDir(), fmt.Sprintf("pve-koth-upload-%s", fHeader.Filename))

	if err = c.SaveFile(fHeader, tmpPath); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save upload")
	}

	defer os.Remove(tmpPath)

	if zipReadCloser, err = zip.OpenReader(tmpPath); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is not a valid zip")
	}

	defer zipReadCloser.Close()

	return
}

func apiGetPublicFile(c *fiber.Ctx) (err error) {
	// Just return JSON of the param and path
	var (
		competitionID = c.Params("competitionID")
		filepath      = c.Params("*")
	)

	return c.JSON(fiber.Map{
		"competitionID": competitionID,
		"filepath":      filepath,
	})
}

func apiGetScoreboard(c *fiber.Ctx) (err error) {
	var (
		user    *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		records []*db.Competition
		payload []scoreboardCompetition
	)

	if records, err = db.Competitions.SelectAll(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competitions")
	}

	var groups []string = fetchUserGroups(user)

	for _, comp := range records {
		if !userCanViewCompetition(user, groups, comp) {
			continue
		}

		var scoreComp scoreboardCompetition
		if scoreComp, err = buildScoreboardCompetition(comp); err != nil {
			appLog.Errorf("scoreboard build failed for %s: %v\n", comp.Name, err)
			return fiber.NewError(fiber.StatusInternalServerError, "failed to build scoreboard")
		}

		payload = append(payload, scoreComp)
	}

	sort.SliceStable(payload, func(i, j int) bool {
		return payload[i].Name < payload[j].Name
	})

	return c.JSON(fiber.Map{
		"competitions": payload,
	})
}

func apiGetScoreboardCompetition(c *fiber.Ctx) (err error) {
	var (
		competitionSlug = c.Params("competitionID")
		records         []*db.Competition
		user            *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if competitionSlug == "" {
		return fiber.NewError(fiber.StatusBadRequest, "competition identifier required")
	}

	if records, err = db.Competitions.SelectAll(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competitions")
	}

	var (
		groups []string = fetchUserGroups(user)
		match  *db.Competition
	)

	for _, comp := range records {
		if strings.EqualFold(comp.SystemID, competitionSlug) || fmt.Sprint(comp.ID) == competitionSlug {
			match = comp
			break
		}
	}

	if match == nil {
		return fiber.ErrNotFound
	}

	if !userCanViewCompetition(user, groups, match) {
		return fiber.NewError(fiber.StatusForbidden, "competition is restricted")
	}

	var payload scoreboardCompetition
	if payload, err = buildScoreboardCompetition(match); err != nil {
		appLog.Errorf("scoreboard build failed for %s: %v\n", match.Name, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to build scoreboard")
	}

	return c.JSON(payload)
}

func summarizeCompetition(comp *db.Competition) competitionSummary {
	return competitionSummary{
		ID:             comp.ID,
		CompetitionID:  comp.SystemID,
		Name:           comp.Name,
		Description:    comp.Description,
		Host:           comp.Host,
		TeamCount:      len(comp.TeamIDs),
		ContainerCount: len(comp.ContainerIDs),
		IsPrivate:      comp.IsPrivate,
		CreatedAt:      comp.CreatedAt,
	}
}

func buildScoreboardCompetition(comp *db.Competition) (scoreboardCompetition, error) {
	var scoreboard scoreboardCompetition = scoreboardCompetition{
		CompetitionID:  comp.SystemID,
		Name:           comp.Name,
		Description:    comp.Description,
		Host:           comp.Host,
		TeamCount:      len(comp.TeamIDs),
		ContainerCount: len(comp.ContainerIDs),
		IsPrivate:      comp.IsPrivate,
		Teams:          []scoreboardTeam{},
	}

	for _, teamID := range comp.TeamIDs {
		team, err := db.Teams.Select(teamID)
		if err != nil {
			return scoreboard, err
		}
		if team == nil {
			continue
		}

		scoreboard.Teams = append(scoreboard.Teams, scoreboardTeam{
			ID:          team.ID,
			Name:        team.Name,
			Score:       team.Score,
			LastUpdated: team.LastUpdated,
		})
	}

	sort.SliceStable(scoreboard.Teams, func(i, j int) bool {
		if scoreboard.Teams[i].Score == scoreboard.Teams[j].Score {
			return scoreboard.Teams[i].LastUpdated.After(scoreboard.Teams[j].LastUpdated)
		}
		return scoreboard.Teams[i].Score > scoreboard.Teams[j].Score
	})

	return scoreboard, nil
}

func fetchUserGroups(user *auth.AuthUser) []string {
	if user == nil || user.LDAPConn == nil {
		return nil
	}

	groups, err := user.LDAPConn.Groups()
	if err != nil {
		appLog.Errorf("failed to load LDAP groups for %s: %v\n", user.LDAPConn.Username, err)
		return nil
	}

	return groups
}

func userCanViewCompetition(user *auth.AuthUser, groups []string, comp *db.Competition) bool {
	if comp == nil {
		return false
	}

	if !comp.IsPrivate {
		return true
	}

	if user == nil {
		return false
	}

	if user.Permissions() >= auth.AuthPermsAdministrator {
		return true
	}

	if len(comp.PrivateLDAPAllowedGroups) == 0 || len(groups) == 0 {
		return false
	}

	for _, allowed := range comp.PrivateLDAPAllowedGroups {
		var cleanAllowed = strings.TrimSpace(strings.ToLower(allowed))
		for _, userGroup := range groups {
			if strings.TrimSpace(strings.ToLower(userGroup)) == cleanAllowed {
				return true
			}
		}
	}

	return false
}
