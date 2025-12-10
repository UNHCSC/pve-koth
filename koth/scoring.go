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

	return &req, nil
}

func scoreCompetition(comp *db.Competition) (err error) {
	var (
		req       *db.CreateCompetitionRequest
		privKey   []byte
		compNet   *net.IPNet
		logPrefix = fmt.Sprintf("competition %s", comp.SystemID)
	)

	if !comp.ScoringActive {
		return nil
	}

	if req, err = loadCompetitionDefinition(comp); err != nil {
		return fmt.Errorf("%s: %w", logPrefix, err)
	}

	if len(req.TeamContainerConfigs) == 0 || len(comp.TeamIDs) == 0 {
		return nil
	}

	if comp.NetworkCIDR == "" {
		return fmt.Errorf("%s has no network CIDR", logPrefix)
	}

	if _, compNet, err = net.ParseCIDR(comp.NetworkCIDR); err != nil {
		return fmt.Errorf("%s network invalid: %w", logPrefix, err)
	}

	if comp.SSHPrivKeyPath == "" {
		return fmt.Errorf("%s missing SSH private key path", logPrefix)
	}

	if privKey, err = os.ReadFile(comp.SSHPrivKeyPath); err != nil {
		return fmt.Errorf("%s failed to read SSH key: %w", logPrefix, err)
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

			network, netErr := buildTeamNetwork(compNet, teamIndex, req.TeamContainerConfigs)
			if netErr != nil {
				scoringLog.Errorf("failed to build network for %s team %d: %v\n", comp.SystemID, team.ID, netErr)
				return
			}

			teamScore, containerResults, teamErr := scoreTeam(comp, team, teamIndex, req.TeamContainerConfigs, network, publicFolderURL, artifactBaseURL, privKey)
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

func buildTeamNetwork(compSubnet *net.IPNet, teamIndex int, configs []db.TeamContainerConfig) (*teamNetwork, error) {
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

func scoreTeam(comp *db.Competition, team *db.Team, teamIndex int, configs []db.TeamContainerConfig, network *teamNetwork, publicFolderURL, artifactBaseURL string, privateKey []byte) (int, []containerScoreResult, error) {
	if team == nil || len(configs) == 0 {
		return 0, nil, nil
	}

	var (
		total   int
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []containerScoreResult
	)

	for order, containerCfg := range configs {
		sanitized := sanitizeContainerName(containerCfg.Name)
		ipAddress := network.ipsByName[sanitized]
		if ipAddress == "" {
			continue
		}

		plan := &containerPlan{
			team:          team,
			name:          containerCfg.Name,
			sanitizedName: sanitized,
			order:         order,
			ipAddress:     ipAddress,
			options: &proxmoxAPI.ContainerCreateOptions{
				Hostname: fmt.Sprintf("%s-team-%d-%s", comp.ContainerRestrictions.HostnamePrefix, teamIndex+1, containerCfg.Name),
			},
		}

		status, statusErr := containerStatusForTeam(team.ID, containerCfg.Name)
		if statusErr != nil {
			scoringLog.Errorf("failed to fetch status for %s (%s): %v\n", plan.options.Hostname, containerCfg.Name, statusErr)
		}
		if strings.EqualFold(status, "redeploying") {
			scoringLog.Statusf("Skipping scoring for %s while redeploying\n", plan.options.Hostname)
			continue
		}

		wg.Add(1)
		go func(cfg db.TeamContainerConfig, plan *containerPlan) {
			defer wg.Done()
			score, detail := scoreContainer(comp, plan, network, publicFolderURL, artifactBaseURL, cfg.ScoringScript, cfg.ScoringSchema, privateKey)
			mu.Lock()
			total += score
			results = append(results, detail)
			mu.Unlock()
		}(containerCfg, plan)
	}

	wg.Wait()
	return total, results, nil
}

func scoreContainer(comp *db.Competition, plan *containerPlan, network *teamNetwork, publicFolderURL, artifactBaseURL string, scoringScripts []string, checks []db.ScoringCheck, privateKey []byte) (int, containerScoreResult) {
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
			scoringLog.Statusf("duplicate scoring check %s defined for %s; ignoring duplicate entry", id, plan.options.Hostname)
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

	var conn *ssh.SSHConnection
	var err error

	if len(scoringScripts) > 0 {
		if conn, err = connectForScoring(plan.ipAddress, privateKey); err != nil {
			scoringLog.Errorf("failed to connect to %s (%s): %v\n", plan.options.Hostname, plan.ipAddress, err)
		}
		if conn == nil {
			scoringLog.Statusf("Container %s (%s) is offline or unreachable; scoring will treat checks as failed\n", plan.options.Hostname, plan.ipAddress)
		}
	}

	if conn != nil {
		defer conn.Close()
		for _, scriptPath := range scoringScripts {
			scriptPath = strings.TrimSpace(scriptPath)
			if scriptPath == "" {
				continue
			}

			scriptURL := buildArtifactFileURL(artifactBaseURL, scriptPath)
			command := ssh.LoadAndRunScript(scriptURL, token, envs)

			exitCode, output, execErr := conn.SendWithOutput(command)
			if execErr != nil {
				scoringLog.Errorf("failed to execute scoring script %s on %s: %v\n", scriptPath, plan.options.Hostname, execErr)
			} else if exitCode != 0 {
				scoringLog.Errorf("scoring script %s exited %d on %s\nOutput:\n%s\n", scriptPath, exitCode, plan.options.Hostname, summarizeScriptOutput(string(output)))
			} else if payload, parseErr := parseCheckPayload(output); parseErr != nil {
				scoringLog.Errorf("invalid scoring payload from %s (%s): %v\n", plan.options.Hostname, scriptPath, parseErr)
			} else {
				for rawID, passed := range payload {
					id := strings.TrimSpace(rawID)
					if id == "" {
						continue
					}
					index, known := schemaIndex[id]
					if !known {
						scoringLog.Statusf("scoring script %s reported unknown check %s on %s; ignoring\n", scriptPath, id, plan.options.Hostname)
						continue
					}
					if reported[id] {
						scoringLog.Statusf("scoring script %s reported duplicate result for check %s on %s; keeping first result\n", scriptPath, id, plan.options.Hostname)
						continue
					}
					reported[id] = true
					result.Checks[index].Passed = passed
				}
			}

			if resetErr := conn.Reset(); resetErr != nil {
				scoringLog.Errorf("failed to reset SSH session on %s: %v\n", plan.options.Hostname, resetErr)
				break
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

func connectForScoring(ip string, privateKey []byte) (*ssh.SSHConnection, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		var conn *ssh.SSHConnection
		if conn, lastErr = ssh.Connect("root", ip, 22, ssh.WithPrivateKey(privateKey)); lastErr == nil {
			return conn, nil
		}
		time.Sleep(time.Second * time.Duration(attempt+1))
	}
	return nil, lastErr
}
