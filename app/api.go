package app

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
	"github.com/gofiber/fiber/v2"
	"github.com/z46-dev/gomysql"
)

type competitionSummary struct {
	ID             int64     `json:"id"`
	CompetitionID  string    `json:"competitionID"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Host           string    `json:"host"`
	TeamCount      int       `json:"teamCount"`
	ContainerCount int       `json:"containerCount"`
	NetworkCIDR    string    `json:"networkCIDR"`
	IsPrivate      bool      `json:"isPrivate"`
	ScoringActive  bool      `json:"scoringActive"`
	CreatedAt      time.Time `json:"createdAt"`
}

type scoreboardTeam struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	Score       int                   `json:"score"`
	LastUpdated time.Time             `json:"lastUpdated"`
	NetworkCIDR string                `json:"networkCIDR"`
	Containers  []scoreboardContainer `json:"containers"`
}

type scoreboardContainer struct {
	Name   string            `json:"name"`
	Checks []scoreboardCheck `json:"checks"`
}

type scoreboardCheck struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	PassPoints int    `json:"passPoints"`
	FailPoints int    `json:"failPoints"`
}

type scoreboardCompetition struct {
	CompetitionID  string           `json:"competitionID"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Host           string           `json:"host"`
	TeamCount      int              `json:"teamCount"`
	ContainerCount int              `json:"containerCount"`
	NetworkCIDR    string           `json:"networkCIDR"`
	IsPrivate      bool             `json:"isPrivate"`
	ScoringActive  bool             `json:"scoringActive"`
	Teams          []scoreboardTeam `json:"teams"`
}

type teamAdminSummary struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Score       int       `json:"score"`
	LastUpdated time.Time `json:"lastUpdated"`
	NetworkCIDR string    `json:"networkCIDR"`
}

type containerTeamSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type containerCompetitionSummary struct {
	ID            int64  `json:"id"`
	CompetitionID string `json:"competitionID"`
	Name          string `json:"name"`
}

type containerAdminSummary struct {
	ID          int64                        `json:"id"`
	Name        string                       `json:"name"`
	IPv4        string                       `json:"ipAddress"`
	Node        string                       `json:"node"`
	Status      string                       `json:"status"`
	ConfigName  string                       `json:"containerConfigName"`
	LastUpdated time.Time                    `json:"lastUpdated"`
	Team        *containerTeamSummary        `json:"team,omitempty"`
	Competition *containerCompetitionSummary `json:"competition,omitempty"`
}

type containerPowerRequest struct {
	IDs    []int64 `json:"ids"`
	Action string  `json:"action"`
}

type containerRedeployRequest struct {
	IDs                   []int64 `json:"ids"`
	StartAfter            bool    `json:"startAfter"`
	EnableAdvancedLogging bool    `json:"enableAdvancedLogging"`
}

var (
	errCompetitionIDMissing  = errors.New("competitionID is required")
	errCompetitionIDConflict = errors.New("competitionID already exists")
)

type uploadContext struct {
	user *auth.AuthUser
	logs []string
}

func newUploadContext(user *auth.AuthUser) *uploadContext {
	return &uploadContext{
		user: user,
		logs: []string{},
	}
}

func (u *uploadContext) logf(format string, args ...any) {
	var message = fmt.Sprintf(format, args...)
	u.logs = append(u.logs, message)
	// logUploadInfo(u.user, "%s", message)
}

func (u *uploadContext) fail(c *fiber.Ctx, status int, message string, cause error) error {
	var actor = uploadActor(u.user)

	if cause != nil {
		appLog.Errorf("upload[%s] %s: %v\n", actor, message, cause)
	} else {
		appLog.Errorf("upload[%s] %s\n", actor, message)
	}

	var payload = fiber.Map{
		"error": message,
		"logs":  u.logs,
	}

	if cause != nil {
		payload["detail"] = cause.Error()
	}

	return c.Status(status).JSON(payload)
}

func (u *uploadContext) success(c *fiber.Ctx, payload fiber.Map) error {
	if payload == nil {
		payload = fiber.Map{}
	}

	payload["logs"] = u.logs
	return c.JSON(payload)
}

func uploadActor(user *auth.AuthUser) string {
	if user != nil && user.LDAPConn != nil && user.LDAPConn.Username != "" {
		return user.LDAPConn.Username
	}

	return "anonymous"
}

func logUploadInfo(user *auth.AuthUser, format string, args ...any) {
	appLog.Basicf("upload[%s] %s\n", uploadActor(user), fmt.Sprintf(format, args...))
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
		user          *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		fHeader       *multipart.FileHeader
		zipReadCloser *zip.ReadCloser
	)

	var ctx *uploadContext = newUploadContext(user)

	if user == nil {
		return ctx.fail(c, fiber.StatusUnauthorized, "authentication required", nil)
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return ctx.fail(c, fiber.StatusForbidden, "insufficient permissions", nil)
	}

	ctx.logf("user %s authorized to manage competitions", user.LDAPConn.Username)

	if fHeader, err = c.FormFile("file"); err != nil {
		return ctx.fail(c, fiber.StatusBadRequest, "file is required", err)
	}

	if fHeader.Size <= 0 {
		return ctx.fail(c, fiber.StatusBadRequest, "file is empty", nil)
	}

	if fHeader.Size > 75*1024*1024 {
		return ctx.fail(c, fiber.StatusBadRequest, "file exceeds 75MB limit", nil)
	}

	ctx.logf("zip received: %s (%d bytes)", fHeader.Filename, fHeader.Size)

	var tmpPath = filepath.Join(os.TempDir(), fmt.Sprintf("pve-koth-upload-%s", fHeader.Filename))

	if err = c.SaveFile(fHeader, tmpPath); err != nil {
		return ctx.fail(c, fiber.StatusInternalServerError, "failed to save upload", err)
	}

	ctx.logf("zip stored temporarily at %s", tmpPath)

	defer os.Remove(tmpPath)

	if zipReadCloser, err = zip.OpenReader(tmpPath); err != nil {
		return ctx.fail(c, fiber.StatusBadRequest, "file is not a valid zip", err)
	}

	defer zipReadCloser.Close()
	ctx.logf("zip opened, scanning contents")

	var (
		configFound   bool
		configData    []byte
		compReq       db.CreateCompetitionRequest
		rootCandidate string
		rootAmbiguous bool
	)

	for _, zippedFile := range zipReadCloser.File {
		if zippedFile.FileInfo().IsDir() {
			continue
		}

		var cleanedName = path.Clean(filepath.ToSlash(zippedFile.Name))
		cleanedName = strings.TrimPrefix(cleanedName, "./")
		if strings.HasPrefix(cleanedName, "../") || cleanedName == ".." {
			return ctx.fail(c, fiber.StatusBadRequest, "zip contains invalid file paths", fmt.Errorf("entry %s", zippedFile.Name))
		}

		var parts = strings.Split(cleanedName, "/")
		if len(parts) > 1 {
			if rootCandidate == "" {
				rootCandidate = parts[0]
			} else if rootCandidate != parts[0] {
				rootAmbiguous = true
			}
		} else if rootCandidate != "" && cleanedName != rootCandidate {
			rootAmbiguous = true
		}

		ctx.logf("processing archive entry: %s (%d bytes)", cleanedName, zippedFile.UncompressedSize64)

		var zippedContent io.ReadCloser
		if zippedContent, err = zippedFile.Open(); err != nil {
			return ctx.fail(c, fiber.StatusBadRequest, "failed to open archive entry", fmt.Errorf("%s: %w", cleanedName, err))
		}

		var data []byte
		if data, err = io.ReadAll(zippedContent); err != nil {
			zippedContent.Close()
			return ctx.fail(c, fiber.StatusBadRequest, "failed to read archive entry", fmt.Errorf("%s: %w", cleanedName, err))
		}
		zippedContent.Close()

		if strings.EqualFold(path.Base(cleanedName), "config.json") {
			var configDir = path.Dir(cleanedName)
			if configDir != "." && configDir != "" {
				rootCandidate = configDir
			}

			ctx.logf("parsing config.json at %s", cleanedName)
			configData = append([]byte(nil), data...)
			if err = json.Unmarshal(data, &compReq); err != nil {
				return ctx.fail(c, fiber.StatusBadRequest, "config.json is invalid", err)
			}
			configFound = true
			ctx.logf("config.json parsed for %s (%s)", compReq.CompetitionName, compReq.CompetitionID)
			continue
		}

		compReq.AttachedFiles = append(compReq.AttachedFiles, struct {
			SourceFilePath string `json:"sourceFilePath"`
			FileContent    []byte `json:"fileContent"`
		}{
			SourceFilePath: cleanedName,
			FileContent:    data,
		})
	}

	if !configFound {
		return ctx.fail(c, fiber.StatusBadRequest, "config.json missing from archive", nil)
	}

	if idErr := ensureCompetitionIDAvailable(compReq.CompetitionID); idErr != nil {
		var status = fiber.StatusInternalServerError
		var message = "failed to validate competition ID"
		var cause error = idErr

		if errors.Is(idErr, errCompetitionIDMissing) {
			status = fiber.StatusBadRequest
			message = "competitionID is required"
			cause = nil
		} else if errors.Is(idErr, errCompetitionIDConflict) {
			status = fiber.StatusConflict
			message = idErr.Error()
			cause = nil
		}

		ctx.logf("validation failed: %s", message)
		return ctx.fail(c, status, message, cause)
	} else {
		ctx.logf("competition ID '%s' validated and available", compReq.CompetitionID)
	}

	var rootPrefix string
	if rootCandidate != "" && !rootAmbiguous && rootCandidate != "." {
		rootPrefix = strings.TrimSuffix(rootCandidate, "/")
		if rootPrefix != "" {
			ctx.logf("detected archive root '%s', trimming attachment paths", rootPrefix)
			for idx := range compReq.AttachedFiles {
				compReq.AttachedFiles[idx].SourceFilePath = strings.TrimPrefix(compReq.AttachedFiles[idx].SourceFilePath, rootPrefix+"/")
			}
		}
	}

	enableAdvancedLogging := false
	if raw := strings.TrimSpace(c.FormValue("enableAdvancedLogging")); raw != "" {
		if parsed, parseErr := strconv.ParseBool(raw); parseErr == nil {
			enableAdvancedLogging = parsed
		}
	}
	compReq.EnableAdvancedLogging = enableAdvancedLogging
	ctx.logf("advanced logging: %t", enableAdvancedLogging)

	var packageRecord *db.CompetitionPackage
	if packageRecord, err = persistCompetitionPackage(&compReq, configData, fHeader.Filename); err != nil {
		return ctx.fail(c, fiber.StatusInternalServerError, "failed to store competition package", err)
	}

	ctx.logf("stored package at %s (packageID=%d)", packageRecord.StoragePath, packageRecord.ID)

	var job *uploadJob = newUploadJob(user)
	job.appendLogs(ctx.logs)
	job.log("waiting for provisioning to start")

	compReq.PackagePath = packageRecord.StoragePath
	startProvisioningJob(job, compReq)

	var compCopy db.CreateCompetitionRequest = compReq
	compCopy.AttachedFiles = nil

	ctx.logf("creating competition pipeline (pending)")
	ctx.logf("creating environment (pending)")
	ctx.logf("upload complete for %s", compReq.CompetitionID)

	return ctx.success(c, fiber.Map{
		"message":         "competition package parsed",
		"competitionID":   compReq.CompetitionID,
		"competitionName": compReq.CompetitionName,
		"attachmentCount": len(compReq.AttachedFiles),
		"packageID":       packageRecord.ID,
		"jobID":           job.ID,
	})
}

func apiGetPublicFile(c *fiber.Ctx) (err error) {
	var (
		competitionID = c.Params("competitionID")
		relativePath  = strings.TrimPrefix(c.Params("*"), "/")
	)

	if competitionID == "" {
		return fiber.ErrBadRequest
	}

	comp, err := db.GetCompetitionBySystemID(competitionID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
	}

	if comp == nil {
		return fiber.ErrNotFound
	}

	var authorized bool
	if koth.ValidateAccessToken(competitionID, c.Cookies("Authorization", "")) {
		authorized = true
	} else if auth.IsAuthenticated(c, jwtSigningKey) != nil {
		authorized = true
	}

	if !authorized {
		return fiber.ErrUnauthorized
	}

	relativePath = sanitizeRelativePathComponent(relativePath)
	if relativePath == "" {
		return fiber.ErrNotFound
	}

	var baseDir = comp.PackageStoragePath
	if baseDir == "" {
		return fiber.ErrNotFound
	}

	if publicFolder := strings.TrimSpace(comp.SetupPublicFolder); publicFolder != "" {
		baseDir = filepath.Join(baseDir, publicFolder)
	}

	var targetPath = filepath.Join(baseDir, relativePath)
	if !pathWithinBase(baseDir, targetPath) {
		return fiber.ErrForbidden
	}

	var info os.FileInfo
	if info, err = os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	if info.IsDir() {
		return fiber.ErrNotFound
	}

	return c.SendFile(targetPath, false)
}

func apiGetArtifactFile(c *fiber.Ctx) (err error) {
	var (
		competitionID = c.Params("competitionID")
		relativePath  = strings.TrimPrefix(c.Params("*"), "/")
	)

	if competitionID == "" {
		return fiber.ErrBadRequest
	}

	comp, err := db.GetCompetitionBySystemID(competitionID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
	}
	if comp == nil {
		return fiber.ErrNotFound
	}

	var authorized bool
	if koth.ValidateAccessToken(competitionID, c.Cookies("Authorization", "")) {
		authorized = true
	} else if auth.IsAuthenticated(c, jwtSigningKey) != nil {
		authorized = true
	}

	if !authorized {
		return fiber.ErrUnauthorized
	}

	relativePath = sanitizeRelativePathComponent(relativePath)
	if relativePath == "" {
		return fiber.ErrNotFound
	}

	var baseDir = comp.PackageStoragePath
	if baseDir == "" {
		return fiber.ErrNotFound
	}

	var targetPath = filepath.Join(baseDir, relativePath)
	if !pathWithinBase(baseDir, targetPath) {
		return fiber.ErrForbidden
	}

	var info os.FileInfo
	if info, err = os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fiber.ErrNotFound
		}

		return fiber.ErrInternalServerError
	}

	if info.IsDir() {
		return fiber.ErrNotFound
	}

	return c.SendFile(targetPath, false)
}

func apiStreamUploadJob(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	var jobID = c.Params("jobID")
	var job *uploadJob = getUploadJob(jobID)
	if job == nil {
		return fiber.ErrNotFound
	}

	if !job.canView(user) {
		return fiber.ErrForbidden
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	var listener = job.subscribe()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer job.unsubscribe(listener)
		for message := range listener {
			fmt.Fprintf(w, "data: %s\n\n", sanitizeLogMessage(message))
			w.Flush()
		}
	})

	return nil
}

func apiStreamRedeployJob(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	var jobID = c.Params("jobID")
	var job *redeployJob = getRedeployJob(jobID)
	if job == nil {
		return fiber.ErrNotFound
	}

	if !job.canView(user) {
		return fiber.ErrForbidden
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	var listener = job.subscribe()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer job.unsubscribe(listener)
		for message := range listener {
			fmt.Fprintf(w, "data: %s\n\n", sanitizeLogMessage(message))
			w.Flush()
		}
	})

	return nil
}

func apiStreamTeardownJob(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	var jobID = c.Params("jobID")
	var job *teardownJob = getTeardownJob(jobID)
	if job == nil {
		return fiber.ErrNotFound
	}

	if !job.canView(user) {
		return fiber.ErrForbidden
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	var listener = job.subscribe()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer job.unsubscribe(listener)
		for message := range listener {
			fmt.Fprintf(w, "data: %s\n\n", sanitizeLogMessage(message))
			w.Flush()
		}
	})

	return nil
}

func apiTeardownCompetition(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	identifier := strings.TrimSpace(c.Params("competitionID"))
	if identifier == "" {
		return fiber.NewError(fiber.StatusBadRequest, "competition identifier required")
	}

	var comp *db.Competition
	if comp, err = loadCompetitionByIdentifier(identifier); err != nil {
		appLog.Errorf("failed to resolve competition %q: %v\n", identifier, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
	}

	if comp == nil {
		return fiber.ErrNotFound
	}

	job := newTeardownJob(user, comp.SystemID)
	startTeardownJob(job)

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("teardown queued (%s)", job.ID),
		"jobID":   job.ID,
	})
}

type scoringToggleRequest struct {
	Active bool `json:"active"`
}

func apiSetCompetitionScoring(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	identifier := strings.TrimSpace(c.Params("competitionID"))
	if identifier == "" {
		return fiber.NewError(fiber.StatusBadRequest, "competition identifier required")
	}

	var comp *db.Competition
	if comp, err = loadCompetitionByIdentifier(identifier); err != nil {
		appLog.Errorf("failed to resolve competition %q: %v\n", identifier, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
	}

	if comp == nil {
		return fiber.ErrNotFound
	}

	var payload scoringToggleRequest
	if err = c.BodyParser(&payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request payload")
	}

	comp.ScoringActive = payload.Active
	if err = db.Competitions.Update(comp); err != nil {
		appLog.Errorf("failed to update scoring flag for %s: %v\n", comp.SystemID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update competition")
	}

	action := "paused"
	if comp.ScoringActive {
		action = "started"
	}

	return c.JSON(fiber.Map{
		"message":       fmt.Sprintf("scoring %s for %s", action, comp.SystemID),
		"scoringActive": comp.ScoringActive,
	})
}

func apiListContainers(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	var (
		filter = strings.TrimSpace(c.Query("competition"))
		comps  []*db.Competition
	)

	if filter != "" {
		var comp *db.Competition
		if comp, err = loadCompetitionByIdentifier(filter); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
		}

		if comp == nil {
			return fiber.ErrNotFound
		}

		comps = []*db.Competition{comp}
	} else {
		if comps, err = db.Competitions.SelectAll(); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load competitions")
		}
	}

	if len(comps) == 0 {
		return c.JSON(fiber.Map{"containers": []containerAdminSummary{}})
	}

	ctSet := make(map[int64]struct{})
	for _, comp := range comps {
		if comp == nil {
			continue
		}
		for _, id := range comp.ContainerIDs {
			if id <= 0 {
				continue
			}
			ctSet[id] = struct{}{}
		}
	}

	if len(ctSet) == 0 {
		return c.JSON(fiber.Map{"containers": []containerAdminSummary{}})
	}

	teamIDs := make(map[int64]struct{})
	for _, comp := range comps {
		for _, teamID := range comp.TeamIDs {
			teamIDs[teamID] = struct{}{}
		}
	}

	teamLookup := make(map[int64]*db.Team, len(teamIDs))
	if len(teamIDs) > 0 {
		var teams []*db.Team
		if teams, err = db.Teams.SelectAll(); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load teams")
		}

		for _, team := range teams {
			if team == nil {
				continue
			}
			if _, needed := teamIDs[team.ID]; needed {
				teamLookup[team.ID] = team
			}
		}
	}

	ctToTeam := make(map[int64]*db.Team)
	for _, team := range teamLookup {
		for _, ctID := range team.ContainerIDs {
			if _, needed := ctSet[ctID]; needed {
				ctToTeam[ctID] = team
			}
		}
	}

	ctToComp := make(map[int64]*db.Competition)
	for _, comp := range comps {
		if comp == nil {
			continue
		}
		for _, ctID := range comp.ContainerIDs {
			if _, needed := ctSet[ctID]; needed {
				ctToComp[ctID] = comp
			}
		}
	}

	var runtime map[int64]koth.ContainerRuntime
	var runtimeErr error
	ctIDs := make([]int64, 0, len(ctSet))
	for id := range ctSet {
		ctIDs = append(ctIDs, id)
	}

	if runtime, runtimeErr = koth.ContainerRuntimeSnapshot(ctIDs); runtimeErr != nil {
		runtime = make(map[int64]koth.ContainerRuntime)
		appLog.Errorf("failed to fetch container runtime data: %v\n", runtimeErr)
	}

	allContainers, err := db.Containers.SelectAll()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load containers")
	}

	recordLookup := make(map[int64]*db.Container)
	for _, record := range allContainers {
		if record == nil {
			continue
		}
		if _, needed := ctSet[record.PVEID]; needed {
			recordLookup[record.PVEID] = record
		}
	}

	summaries := make([]containerAdminSummary, 0, len(ctSet))
	for _, id := range ctIDs {
		record := recordLookup[id]
		if record == nil {
			continue
		}

		comp := ctToComp[id]
		team := ctToTeam[id]
		rt := runtime[id]
		name := rt.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("CT-%d", record.PVEID)
		}

		status := strings.ToLower(strings.TrimSpace(record.Status))
		if status == "" {
			status = "unknown"
		}

		summary := containerAdminSummary{
			ID:          record.PVEID,
			Name:        name,
			IPv4:        record.IPAddress,
			Status:      status,
			Node:        rt.Node,
			ConfigName:  record.ConfigName,
			LastUpdated: record.LastUpdated,
		}

		if team != nil {
			summary.Team = &containerTeamSummary{ID: team.ID, Name: team.Name}
		}

		if comp != nil {
			summary.Competition = &containerCompetitionSummary{
				ID:            comp.ID,
				CompetitionID: comp.SystemID,
				Name:          comp.Name,
			}
		}

		summaries = append(summaries, summary)
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		compI := ""
		compJ := ""
		if summaries[i].Competition != nil {
			compI = strings.ToLower(summaries[i].Competition.Name)
		}
		if summaries[j].Competition != nil {
			compJ = strings.ToLower(summaries[j].Competition.Name)
		}
		if compI != compJ {
			return compI < compJ
		}

		teamI := ""
		teamJ := ""
		if summaries[i].Team != nil {
			teamI = strings.ToLower(summaries[i].Team.Name)
		}
		if summaries[j].Team != nil {
			teamJ = strings.ToLower(summaries[j].Team.Name)
		}
		if teamI != teamJ {
			return teamI < teamJ
		}

		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})

	return c.JSON(fiber.Map{
		"containers": summaries,
	})
}

type teamScoreMutationRequest struct {
	Action string `json:"action"`
	Amount int    `json:"amount"`
}

func apiGetCompetitionTeams(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	identifier := strings.TrimSpace(c.Params("competitionID"))
	if identifier == "" {
		return fiber.NewError(fiber.StatusBadRequest, "competition identifier required")
	}

	var comp *db.Competition
	if comp, err = loadCompetitionByIdentifier(identifier); err != nil {
		appLog.Errorf("failed to resolve competition %q: %v\n", identifier, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
	}

	if comp == nil {
		return fiber.ErrNotFound
	}

	summaries := make([]teamAdminSummary, 0, len(comp.TeamIDs))
	for teamIndex, teamID := range comp.TeamIDs {
		var team *db.Team
		if team, err = db.Teams.Select(teamID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load teams")
		}
		if team == nil {
			continue
		}

		network := strings.TrimSpace(team.NetworkCIDR)
		if network == "" {
			if computed, netErr := koth.TeamSubnetCIDR(comp, teamIndex); netErr == nil {
				network = computed
			}
		}

		summaries = append(summaries, teamAdminSummary{
			ID:          team.ID,
			Name:        team.Name,
			Score:       team.Score,
			LastUpdated: team.LastUpdated,
			NetworkCIDR: network,
		})
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Score == summaries[j].Score {
			return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
		}
		return summaries[i].Score > summaries[j].Score
	})

	return c.JSON(fiber.Map{
		"teams": summaries,
	})
}

func apiModifyTeamScore(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	identifier := strings.TrimSpace(c.Params("competitionID"))
	if identifier == "" {
		return fiber.NewError(fiber.StatusBadRequest, "competition identifier required")
	}

	var comp *db.Competition
	if comp, err = loadCompetitionByIdentifier(identifier); err != nil {
		appLog.Errorf("failed to resolve competition %q: %v\n", identifier, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load competition")
	}

	if comp == nil {
		return fiber.ErrNotFound
	}

	teamIDParam := strings.TrimSpace(c.Params("teamID"))
	if teamIDParam == "" {
		return fiber.NewError(fiber.StatusBadRequest, "team identifier required")
	}

	teamID, convErr := strconv.ParseInt(teamIDParam, 10, 64)
	if convErr != nil {
		return fiber.NewError(fiber.StatusBadRequest, "team identifier invalid")
	}

	var belongs bool
	for _, id := range comp.TeamIDs {
		if id == teamID {
			belongs = true
			break
		}
	}
	if !belongs {
		return fiber.NewError(fiber.StatusNotFound, "team not found in competition")
	}

	var team *db.Team
	if team, err = db.Teams.Select(teamID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load team")
	}
	if team == nil {
		return fiber.ErrNotFound
	}

	var payload teamScoreMutationRequest
	if err = c.BodyParser(&payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request payload")
	}

	action := strings.ToLower(strings.TrimSpace(payload.Action))
	switch action {
	case "reset":
		team.Score = 0
	case "adjust":
		if payload.Amount == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "amount must be non-zero")
		}
		team.Score += payload.Amount
	default:
		return fiber.NewError(fiber.StatusBadRequest, "action must be 'reset' or 'adjust'")
	}

	team.LastUpdated = time.Now().UTC()
	if err = db.Teams.Update(team); err != nil {
		appLog.Errorf("failed to update team %d score: %v\n", team.ID, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update team score")
	}

	message := "team score updated"
	if action == "reset" {
		message = "team score reset"
	}

	return c.JSON(fiber.Map{
		"message":     message,
		"score":       team.Score,
		"lastUpdated": team.LastUpdated,
	})
}

func apiSetContainerPower(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	var payload containerPowerRequest
	if err = c.BodyParser(&payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request payload")
	}

	ids := normalizeRequestedContainers(payload.IDs)
	if len(ids) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "container IDs required")
	}

	action := strings.ToLower(strings.TrimSpace(payload.Action))
	if action != "start" && action != "stop" {
		return fiber.NewError(fiber.StatusBadRequest, "action must be 'start' or 'stop'")
	}

	for _, id := range ids {
		if record, selErr := db.Containers.Select(id); selErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to validate containers")
		} else if record == nil {
			return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("container %d not found", id))
		}
	}

	switch action {
	case "start":
		if err = koth.BulkStartContainers(ids); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to start containers")
		}
	case "stop":
		if err = koth.BulkStopContainers(ids); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to stop containers")
		}
	}

	if refreshErr := koth.RefreshContainerStatuses(ids); refreshErr != nil {
		appLog.Errorf("failed to refresh container statuses after %s request: %v\n", action, refreshErr)
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("containers queued to %s", action),
		"updated": len(ids),
	})
}

func apiRedeployContainers(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	if user.Permissions() < auth.AuthPermsAdministrator {
		return fiber.NewError(fiber.StatusForbidden, "administrator access required")
	}

	var payload containerRedeployRequest
	if err = c.BodyParser(&payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request payload")
	}

	ids := normalizeRequestedContainers(payload.IDs)
	if len(ids) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "container IDs required")
	}

	for _, id := range ids {
		record, selErr := db.Containers.Select(id)
		if selErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to validate containers")
		}
		if record == nil {
			return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("container %d not found", id))
		}
	}

	job := newRedeployJob(user, ids, payload.StartAfter, payload.EnableAdvancedLogging)
	startRedeployJob(job)

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("redeploy queued (%s)", job.ID),
		"jobID":   job.ID,
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

func normalizeRequestedContainers(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}

	return result
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
		NetworkCIDR:    comp.NetworkCIDR,
		IsPrivate:      comp.IsPrivate,
		ScoringActive:  comp.ScoringActive,
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
		NetworkCIDR:    comp.NetworkCIDR,
		IsPrivate:      comp.IsPrivate,
		ScoringActive:  comp.ScoringActive,
		Teams:          []scoreboardTeam{},
	}

	for teamIndex, teamID := range comp.TeamIDs {
		team, err := db.Teams.Select(teamID)
		if err != nil {
			return scoreboard, err
		}
		if team == nil {
			continue
		}

		containers, err := loadTeamScoreResults(team.ID)
		if err != nil {
			return scoreboard, err
		}

		network := strings.TrimSpace(team.NetworkCIDR)
		if network == "" {
			if computed, netErr := koth.TeamSubnetCIDR(comp, teamIndex); netErr == nil {
				network = computed
			}
		}

		scoreboard.Teams = append(scoreboard.Teams, scoreboardTeam{
			ID:          team.ID,
			Name:        team.Name,
			Score:       team.Score,
			LastUpdated: team.LastUpdated,
			NetworkCIDR: network,
			Containers:  containers,
		})
	}

	sort.SliceStable(scoreboard.Teams, func(i, j int) bool {
		if scoreboard.Teams[i].Score == scoreboard.Teams[j].Score {
			nameI := strings.ToLower(scoreboard.Teams[i].Name)
			nameJ := strings.ToLower(scoreboard.Teams[j].Name)
			if nameI == nameJ {
				return scoreboard.Teams[i].LastUpdated.After(scoreboard.Teams[j].LastUpdated)
			}
			return nameI < nameJ
		}
		return scoreboard.Teams[i].Score > scoreboard.Teams[j].Score
	})

	return scoreboard, nil
}

func loadTeamScoreResults(teamID int64) ([]scoreboardContainer, error) {
	filter := gomysql.NewFilter().KeyCmp(db.ScoreResults.FieldBySQLName("team_id"), gomysql.OpEqual, teamID)
	records, err := db.ScoreResults.SelectAllWithFilter(filter)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []scoreboardContainer{}, nil
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].ContainerOrder == records[j].ContainerOrder {
			if strings.EqualFold(records[i].ContainerName, records[j].ContainerName) {
				return records[i].CheckOrder < records[j].CheckOrder
			}
			return strings.ToLower(records[i].ContainerName) < strings.ToLower(records[j].ContainerName)
		}
		return records[i].ContainerOrder < records[j].ContainerOrder
	})

	var (
		containers []scoreboardContainer
		current    *scoreboardContainer
		lastName   string
	)

	for _, record := range records {
		if current == nil || !strings.EqualFold(lastName, record.ContainerName) {
			containers = append(containers, scoreboardContainer{
				Name:   record.ContainerName,
				Checks: []scoreboardCheck{},
			})
			current = &containers[len(containers)-1]
			lastName = record.ContainerName
		}

		current.Checks = append(current.Checks, scoreboardCheck{
			ID:         record.CheckID,
			Name:       record.CheckName,
			Passed:     record.Passed,
			PassPoints: record.PassPoints,
			FailPoints: record.FailPoints,
		})
	}

	return containers, nil
}

func persistCompetitionPackage(req *db.CreateCompetitionRequest, configBytes []byte, originalFilename string) (record *db.CompetitionPackage, err error) {
	if req == nil {
		return nil, fmt.Errorf("competition request is nil")
	}

	var basePath = filepath.Join(config.StorageBasePath(), "packages")

	if err = os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("prepare storage base: %w", err)
	}

	var sanitizedID = sanitizeIdentifier(req.CompetitionID)
	if sanitizedID == "" {
		sanitizedID = "competition"
	}

	var timestamp = time.Now().UTC()
	var packageDir = filepath.Join(basePath, fmt.Sprintf("%s-%d", sanitizedID, timestamp.UnixNano()))
	if err = os.MkdirAll(packageDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare package directory: %w", err)
	}

	if len(configBytes) == 0 {
		if configBytes, err = json.MarshalIndent(req, "", "  "); err != nil {
			return nil, fmt.Errorf("marshal config json: %w", err)
		}
	}

	if err = os.WriteFile(filepath.Join(packageDir, "config.json"), configBytes, 0600); err != nil {
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	for _, attachment := range req.AttachedFiles {
		var relPath = strings.TrimLeft(attachment.SourceFilePath, "/")
		if relPath == "" {
			continue
		}

		var destination = filepath.Join(packageDir, relPath)
		if !pathWithinBase(packageDir, destination) {
			return nil, fmt.Errorf("attachment path escapes storage directory: %s", relPath)
		}

		if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return nil, fmt.Errorf("prepare directory for %s: %w", relPath, err)
		}

		if err = os.WriteFile(destination, attachment.FileContent, 0644); err != nil {
			return nil, fmt.Errorf("write attachment %s: %w", relPath, err)
		}
	}

	record = &db.CompetitionPackage{
		CompetitionID:    req.CompetitionID,
		CompetitionName:  req.CompetitionName,
		OriginalFilename: originalFilename,
		StoragePath:      packageDir,
		ConfigJSON:       append([]byte(nil), configBytes...),
		AttachmentCount:  len(req.AttachedFiles),
		CreatedAt:        timestamp,
	}

	if err = db.CompetitionPackages.Insert(record); err != nil {
		return nil, fmt.Errorf("record package metadata: %w", err)
	}

	return record, nil
}

func sanitizeIdentifier(value string) string {
	var cleanValue = strings.ToLower(strings.TrimSpace(value))
	if cleanValue == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range cleanValue {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		case r == ' ' || r == '/' || r == '\\':
			builder.WriteRune('-')
		}
	}

	return strings.Trim(builder.String(), "-")
}

func pathWithinBase(base, target string) bool {
	var cleanBase = filepath.Clean(base)
	var cleanTarget = filepath.Clean(target)

	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func sanitizeRelativePathComponent(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "/")
	rel = path.Clean(rel)
	if rel == "." || rel == "/" {
		return ""
	}

	for strings.HasPrefix(rel, "../") {
		rel = strings.TrimPrefix(rel, "../")
	}

	rel = strings.TrimPrefix(rel, "./")
	rel = strings.Trim(rel, "/")
	return rel
}

func ensureCompetitionIDAvailable(compID string) error {
	var trimmed = strings.TrimSpace(compID)
	if trimmed == "" {
		return errCompetitionIDMissing
	}

	competitions, err := db.Competitions.SelectAll()
	if err != nil {
		return fmt.Errorf("check competitions: %w", err)
	}

	for _, comp := range competitions {
		if strings.EqualFold(comp.SystemID, trimmed) {
			return fmt.Errorf("%w: %s is already active", errCompetitionIDConflict, trimmed)
		}
	}

	packages, err := db.CompetitionPackages.SelectAll()
	if err != nil {
		return fmt.Errorf("check stored packages: %w", err)
	}

	for _, pkg := range packages {
		if strings.EqualFold(pkg.CompetitionID, trimmed) {
			return fmt.Errorf("%w: %s has already been uploaded", errCompetitionIDConflict, trimmed)
		}
	}

	return nil
}

func loadCompetitionByIdentifier(identifier string) (*db.Competition, error) {
	if identifier == "" {
		return nil, nil
	}

	if comp, err := db.GetCompetitionBySystemID(identifier); err != nil {
		return nil, err
	} else if comp != nil {
		return comp, nil
	}

	if numericID, convErr := strconv.ParseInt(identifier, 10, 64); convErr == nil {
		return db.Competitions.Select(numericID)
	}

	return nil, nil
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
