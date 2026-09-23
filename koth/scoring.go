package koth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/ssh"
	"github.com/luthermonson/go-proxmox"
	"github.com/z46-dev/go-logger"
	"github.com/z46-dev/gomysql"
)

const scoringInterval = time.Minute

var (
	scoringLog      *logger.Logger = logger.NewLogger().SetPrefix("[SCORE]", logger.BoldYellow).IncludeTimestamp()
	scoringLoopOnce sync.Once
)

type containerScoreResult struct {
	Name   string
	Order  int
	Checks []checkScoreResult
}

type checkScoreResult struct {
	ID         string
	Name       string
	Order      int
	PassPoints int
	FailPoints int
	Passed     bool
}

func StartScoringLoop() {
	scoringLoopOnce.Do(func() {
		go scoringLoop()
	})
}

func scoringLoop() {
	scoringLog.Basicf("scoring loop started (interval %s)\n", scoringInterval)
	runScoringPass()

	ticker := time.NewTicker(scoringInterval)
	defer ticker.Stop()

	for range ticker.C {
		runScoringPass()
	}
}

func runScoringPass() {
	comps, err := db.Competitions.SelectAll()
	if err != nil {
		scoringLog.Errorf("failed to load competitions for scoring: %v\n", err)
		return
	}

	if len(comps) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, comp := range comps {
		if comp == nil || !comp.ScoringActive {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := scoreCompetition(comp); err != nil {
				scoringLog.Errorf("scoring failed for %s: %v\n", comp.SystemID, err)
			}
		}()
	}

	wg.Wait()
}

func ScoreCompetitionNow(comp *db.Competition) error {
	return scoreCompetition(comp)
}

func loadCompetitionDefinition(comp *db.Competition) (*db.CreateCompetitionRequest, error) {
	if comp == nil {
		return nil, fmt.Errorf("competition is nil")
	}

	var (
		configBytes []byte
		err         error
	)

	if comp.PackageStoragePath != "" {
		configPath := filepath.Join(comp.PackageStoragePath, "config.json")
		if configBytes, err = os.ReadFile(configPath); err != nil {
			scoringLog.Errorf("failed to read %s for %s: %v\n", configPath, comp.SystemID, err)
		}
	}

	if len(configBytes) == 0 {
		var pkg *db.CompetitionPackage
		if pkg, err = db.GetCompetitionPackageBySystemID(comp.SystemID); err != nil {
			return nil, err
		}
		if pkg == nil {
			return nil, fmt.Errorf("competition package missing for %s", comp.SystemID)
		}
		configBytes = append([]byte(nil), pkg.ConfigJSON...)
	}

	var req db.CreateCompetitionRequest
	if err = json.Unmarshal(configBytes, &req); err != nil {
		return nil, fmt.Errorf("parse competition config: %w", err)
	}

	if _, err = ensureTemplateLookup(&req); err != nil {
		return nil, fmt.Errorf("template lookup: %w", err)
	}

	return &req, nil
}

func scoreCompetition(comp *db.Competition) (err error) {
	var (
		req       *db.CreateCompetitionRequest
		compNet   *net.IPNet
		logPrefix = fmt.Sprintf("competition %s", comp.SystemID)
	)

	if !comp.ScoringActive {
		return nil
	}

	if req, err = loadCompetitionDefinition(comp); err != nil {
		return fmt.Errorf("%s: %w", logPrefix, err)
	}

	if len(req.TeamGuestConfigs) == 0 || len(comp.TeamIDs) == 0 {
		return nil
	}

	if comp.NetworkCIDR == "" {
		return fmt.Errorf("%s has no network CIDR", logPrefix)
	}

	if _, compNet, err = net.ParseCIDR(comp.NetworkCIDR); err != nil {
		return fmt.Errorf("%s network invalid: %w", logPrefix, err)
	}

	publicFolderURL := competitionPublicFolderURL(comp)
	artifactBaseURL := buildCompetitionArtifactBase(externalBaseURL(), comp.SystemID)

	var wg sync.WaitGroup
	for idx, teamID := range comp.TeamIDs {
		wg.Add(1)
		go func(teamIndex int, dbTeamID int64) {
			defer wg.Done()

			team, teamErr := db.Teams.Select(dbTeamID)
			if teamErr != nil {
				scoringLog.Errorf("failed to load team %d: %v\n", dbTeamID, teamErr)
				return
			}
			if team == nil {
				return
			}

			network, netErr := buildTeamNetwork(compNet, teamIndex, req.TeamGuestConfigs)
			if netErr != nil {
				scoringLog.Errorf("failed to build network for %s team %d: %v\n", comp.SystemID, team.ID, netErr)
				return
			}

			teamScore, containerResults, teamErr := scoreTeam(comp, team, teamIndex, req.TeamGuestConfigs, network, publicFolderURL, artifactBaseURL, req)
			if teamErr != nil {
				scoringLog.Errorf("team %s scoring had errors: %v\n", team.Name, teamErr)
			}

			persistScoreResults(team.ID, containerResults)

			team.Score += teamScore
			team.LastUpdated = time.Now()
			if dbErr := db.Teams.Update(team); dbErr != nil {
				scoringLog.Errorf("failed to update team %d: %v\n", team.ID, dbErr)
			}
		}(idx, teamID)
	}

	wg.Wait()

	return nil
}

func buildTeamNetwork(compSubnet *net.IPNet, teamIndex int, configs []db.TeamGuestConfig) (*teamNetwork, error) {
	network := &teamNetwork{
		ipsByName: make(map[string]string),
		ipOrder:   make([]string, 0),
	}

	teamSubnetBase, err := teamSubnetBaseIP(compSubnet, teamIndex)
	if err != nil {
		return nil, err
	}

	for _, cfg := range configs {
		hostIP, hostErr := hostIPWithinSubnet(teamSubnetBase, config.Config.Network.TeamSubnetPrefix, cfg.LastOctetValue)
		if hostErr != nil {
			return nil, hostErr
		}

		sanitizedName := sanitizeContainerName(cfg.Name)
		network.ipsByName[sanitizedName] = hostIP.String()
		network.ipOrder = append(network.ipOrder, hostIP.String())
	}

	return network, nil
}

func scoreTeam(comp *db.Competition, team *db.Team, teamIndex int, configs []db.TeamGuestConfig, network *teamNetwork, publicFolderURL, artifactBaseURL string, req *db.CreateCompetitionRequest) (int, []containerScoreResult, error) {
	if team == nil || len(configs) == 0 {
		return 0, nil, nil
	}

	var (
		total   int
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []containerScoreResult
	)

	for order, guestCfg := range configs {
		sanitized := sanitizeContainerName(guestCfg.Name)
		ipAddress := network.ipsByName[sanitized]
		if ipAddress == "" {
			continue
		}

		guestSpec, specErr := ResolveGuestSpecTemplate(req.GuestTemplateLookup, guestCfg.GuestSpecsTemplate)
		if specErr != nil {
			scoringLog.Errorf("failed to resolve template %s for %s: %v\n", guestCfg.GuestSpecsTemplate, guestCfg.Name, specErr)
			continue
		}
		hostname := fmt.Sprintf("%s-team-%d-%s", comp.ContainerRestrictions.HostnamePrefix, teamIndex+1, guestCfg.Name)

		plan := &guestPlan{
			team:          team,
			name:          guestCfg.Name,
			sanitizedName: sanitized,
			order:         order,
			ipAddress:     ipAddress,
			guestKind:     guestSpec.Kind,
			guestOS:       guestSpec.OS,
			scriptShell:   guestSpec.Shell,
			templateRef:   strings.TrimSpace(guestCfg.GuestSpecsTemplate),
			templateVMID:  guestSpec.TemplateVMID,
			hostname:      hostname,
			username:      guestSpec.Username,
			password:      guestSpec.Password,
			storagePool:   guestSpec.StoragePool,
		}

		status, statusErr := containerStatusForTeam(team.ID, guestCfg.Name)
		if statusErr != nil {
			scoringLog.Errorf("failed to fetch status for %s (%s): %v\n", plan.hostname, guestCfg.Name, statusErr)
		}
		if strings.EqualFold(status, "redeploying") {
			scoringLog.Statusf("Skipping scoring for %s while redeploying\n", plan.hostname)
			continue
		}

		wg.Add(1)
		go func(cfg db.TeamGuestConfig, plan *guestPlan) {
			defer wg.Done()
			score, detail := scoreContainer(comp, plan, network, publicFolderURL, artifactBaseURL, cfg.ScoringScript, cfg.ScoringSchema)
			mu.Lock()
			total += score
			results = append(results, detail)
			mu.Unlock()
		}(guestCfg, plan)
	}

	wg.Wait()
	return total, results, nil
}

func scoreContainer(comp *db.Competition, plan *guestPlan, network *teamNetwork, publicFolderURL, artifactBaseURL string, scoringScripts []string, checks []db.ScoringCheck) (int, containerScoreResult) {
	var result containerScoreResult
	if plan != nil {
		result.Name = plan.name
		result.Order = plan.order
	}

	if plan == nil || len(checks) == 0 {
		return 0, result
	}

	var (
		schemaIndex = make(map[string]int)
		reported    = make(map[string]bool)
	)

	for idx, check := range checks {
		id := strings.TrimSpace(check.ID)
		if id == "" {
			continue
		}
		if _, exists := schemaIndex[id]; exists {
			scoringLog.Statusf("duplicate scoring check %s defined for %s; ignoring duplicate entry", id, plan.hostname)
			continue
		}

		schemaIndex[id] = len(result.Checks)
		result.Checks = append(result.Checks, checkScoreResult{
			ID:         id,
			Name:       check.Name,
			Order:      idx,
			PassPoints: check.PassPoints,
			FailPoints: check.FailPoints,
		})
	}

	if len(result.Checks) == 0 {
		return 0, result
	}

	var (
		envs  map[string]any
		token string
	)

	if len(scoringScripts) > 0 {
		envs = buildScriptEnv(comp, plan, network, publicFolderURL)
		const tokenTTL = 5 * time.Minute
		token = IssueAccessToken(comp.SystemID, tokenTTL)
		envs["KOTH_ACCESS_TOKEN"] = token
		defer RevokeAccessToken(token)
	}

	record, recErr := containerRecordForTeam(plan.team.ID, plan.name)
	if recErr != nil {
		scoringLog.Errorf("failed to load guest record for %s: %v\n", plan.hostname, recErr)
		return 0, result
	}
	if record == nil {
		scoringLog.Statusf("Guest %s not provisioned; treating checks as failed\n", plan.hostname)
		return 0, result
	}

	for _, scriptPath := range scoringScripts {
		scriptPath = strings.TrimSpace(scriptPath)
		if scriptPath == "" {
			continue
		}

		scriptURL := buildArtifactFileURL(artifactBaseURL, scriptPath)
		var stdout, stderr string
		var exitCode int
		var execErr error
		if record.GuestKind == db.GuestKindQEMU {
			var vm *proxmox.VirtualMachine
			if vm, execErr = api.VirtualMachine(int(record.PVEID)); execErr == nil {
				var commandResult proxmoxAPI.VMCommandResult
				commandResult, execErr = api.ExecuteVirtualMachineCommand(vm, powershellCommand(powershellScriptInvocation(scriptURL, token, envs)), "", 5*time.Minute)
				stdout, stderr, exitCode = commandResult.Stdout, commandResult.Stderr, commandResult.ExitCode
			}
		} else {
			var ct *proxmox.Container
			if ct, execErr = api.Container(int(record.PVEID)); execErr == nil {
				command := ssh.LoadAndRunScript(scriptURL, token, envs)
				stdout, stderr, exitCode, execErr = api.RawExecuteWithRetries(ct, "root", plan.password, command, 2)
			}
		}
		if execErr != nil {
			scoringLog.Errorf("failed to execute scoring script %s on %s: %v\n", scriptPath, plan.hostname, execErr)
		} else if exitCode != 0 {
			scoringLog.Errorf("scoring script %s exited %d on %s\nStdout:\n%s\nStderr:\n%s\n", scriptPath, exitCode, plan.hostname, summarizeScriptOutput(stdout), summarizeScriptOutput(stderr))
		} else if payload, parseErr := parseCheckPayload([]byte(stdout)); parseErr != nil {
			scoringLog.Errorf("invalid scoring payload from %s (%s): %v\nStdout:\n%s\nStderr:\n%s\n", plan.hostname, scriptPath, parseErr, summarizeScriptOutput(stdout), summarizeScriptOutput(stderr))
		} else {
			for rawID, passed := range payload {
				id := strings.TrimSpace(rawID)
				if id == "" {
					continue
				}
				index, known := schemaIndex[id]
				if !known {
					scoringLog.Statusf("scoring script %s reported unknown check %s on %s; ignoring\n", scriptPath, id, plan.hostname)
					continue
				}
				if reported[id] {
					scoringLog.Statusf("scoring script %s reported duplicate result for check %s on %s; keeping first result\n", scriptPath, id, plan.hostname)
					continue
				}
				reported[id] = true
				result.Checks[index].Passed = passed
			}
		}
	}

	var total int
	for _, check := range result.Checks {
		if reported[check.ID] && check.Passed {
			total += check.PassPoints
		} else {
			total += check.FailPoints
		}
	}

	return total, result
}

func persistScoreResults(teamID int64, containers []containerScoreResult) {
	filter := gomysql.NewFilter().KeyCmp(db.ScoreResults.FieldBySQLName("team_id"), gomysql.OpEqual, teamID)
	if previous, err := db.ScoreResults.SelectAllWithFilter(filter); err == nil {
		for _, entry := range previous {
			_ = db.ScoreResults.Delete(entry.ID)
		}
	} else {
		scoringLog.Errorf("failed to load score results for team %d: %v\n", teamID, err)
	}

	timestamp := time.Now()
	for _, container := range containers {
		for _, check := range container.Checks {
			record := &db.ScoreResult{
				TeamID:         teamID,
				ContainerName:  container.Name,
				ContainerOrder: container.Order,
				CheckID:        check.ID,
				CheckName:      check.Name,
				CheckOrder:     check.Order,
				PassPoints:     check.PassPoints,
				FailPoints:     check.FailPoints,
				Passed:         check.Passed,
				UpdatedAt:      timestamp,
			}

			if err := db.ScoreResults.Insert(record); err != nil {
				scoringLog.Errorf("failed to persist score result for team %d: %v\n", teamID, err)
			}
		}
	}
}

func parseCheckPayload(raw []byte) (map[string]bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err == nil && len(generic) > 0 {
		results := make(map[string]bool)
		for key, value := range generic {
			if val, ok := value.(bool); ok {
				results[key] = val
			}
		}
		if len(results) > 0 {
			return results, nil
		}
	}

	var legacy struct {
		Checks map[string]bool `json:"checks"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil && legacy.Checks != nil {
		return legacy.Checks, nil
	}

	return nil, fmt.Errorf("payload missing boolean check data")
}

func containerStatusForTeam(teamID int64, configName string) (string, error) {
	filter := gomysql.NewFilter().
		KeyCmp(db.Containers.FieldBySQLName("team_id"), gomysql.OpEqual, teamID).
		And().
		KeyCmp(db.Containers.FieldBySQLName("container_config_name"), gomysql.OpEqual, strings.TrimSpace(configName))

	results, err := db.Containers.SelectAllWithFilter(filter)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	return strings.TrimSpace(results[0].Status), nil
}

func containerRecordForTeam(teamID int64, configName string) (*db.Container, error) {
	filter := gomysql.NewFilter().
		KeyCmp(db.Containers.FieldBySQLName("team_id"), gomysql.OpEqual, teamID).
		And().
		KeyCmp(db.Containers.FieldBySQLName("container_config_name"), gomysql.OpEqual, strings.TrimSpace(configName))

	results, err := db.Containers.SelectAllWithFilter(filter)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}
