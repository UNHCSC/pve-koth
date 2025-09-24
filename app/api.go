package app

import (
	"archive/zip"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/gofiber/fiber/v2"
)

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
		competitions, retrievedCompetitions []*db.Competition
		user                                *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if retrievedCompetitions, err = db.Competitions.SelectAll(); err != nil {
		return
	}

	for _, comp := range retrievedCompetitions {
		if user.Permissions() >= auth.AuthPermsAdministrator || !comp.IsPrivate {
			competitions = append(competitions, comp)
		}
	}

	return c.JSON(fiber.Map{
		"competitions": competitions,
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
