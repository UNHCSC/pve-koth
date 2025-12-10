package koth

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/ssh"
	"github.com/luthermonson/go-proxmox"
)

// RedeployContainers deletes and rebuilds the requested containers using the original competition plan.
func RedeployContainers(ids []int64) error {
	return RedeployContainersWithLogger(ids, containerLog, false)
}

// RedeployContainersWithLogger behaves like RedeployContainers but routes logs through the provided logger.
// The startAfter flag will restart each container after redeployment if true.
func RedeployContainersWithLogger(ids []int64, log ProgressLogger, startAfter bool) error {
	normalized := normalizeContainerIDs(ids)
	if len(normalized) == 0 {
		return fmt.Errorf("no container IDs supplied")
	}

	if api == nil {
		return fmt.Errorf("proxmox API is not initialized")
	}

	var localLog = wrapLoggerSafe(log)
	if localLog == nil {
		localLog = wrapLoggerSafe(containerLog)
	}

	for _, id := range normalized {
		localLog.Statusf("Redeploying container %d...", id)
		if err := redeployContainer(localLog, id, startAfter); err != nil {
			return fmt.Errorf("container %d: %w", id, err)
		}
		localLog.Successf("Container %d redeployed successfully.", id)
	}

	return nil
}

func redeployContainer(log ProgressLogger, id int64, startAfter bool) (err error) {
	var record *db.Container
	if record, err = db.Containers.Select(id); err != nil {
		return fmt.Errorf("lookup container: %w", err)
	} else if record == nil {
		return fmt.Errorf("container %d not found", id)
	}

	var comp *db.Competition
	if comp, err = findCompetitionForContainer(id); err != nil {
		return err
	}

	var req *db.CreateCompetitionRequest
	if req, err = loadCompetitionDefinition(comp); err != nil {
		return fmt.Errorf("load competition definition: %w", err)
	}
	if len(req.TeamContainerConfigs) == 0 {
		return fmt.Errorf("competition %s has no container configurations", comp.SystemID)
	}

	var team *db.Team
	var teamIndex int
	if team, teamIndex, err = resolveTeamForContainer(comp, record); err != nil {
		return err
	}

	var cfg db.TeamContainerConfig
	var cfgIndex int
	if cfg, cfgIndex, err = resolveContainerConfig(req.TeamContainerConfigs, record); err != nil {
		return err
	}

	var publicKeyData []byte
	if comp.SSHPubKeyPath == "" {
		return fmt.Errorf("competition %s missing SSH public key", comp.SystemID)
	}
	if publicKeyData, err = os.ReadFile(comp.SSHPubKeyPath); err != nil {
		return fmt.Errorf("read ssh public key: %w", err)
	}
	var publicKey = strings.TrimSpace(string(publicKeyData))

	var privateKey []byte
	if comp.SSHPrivKeyPath == "" {
		return fmt.Errorf("competition %s missing SSH private key", comp.SystemID)
	}
	if privateKey, err = os.ReadFile(comp.SSHPrivKeyPath); err != nil {
		return fmt.Errorf("read ssh private key: %w", err)
	}

	if strings.TrimSpace(record.IPAddress) == "" {
		return fmt.Errorf("container %d missing recorded IP address", record.PVEID)
	}

	if strings.TrimSpace(comp.NetworkCIDR) == "" {
		return fmt.Errorf("competition %s missing network CIDR", comp.SystemID)
	}

	var _, compNet *net.IPNet
	if _, compNet, err = net.ParseCIDR(comp.NetworkCIDR); err != nil {
		return fmt.Errorf("parse competition network: %w", err)
	}

	var network *teamNetwork
	if network, err = buildTeamNetwork(compNet, teamIndex, req.TeamContainerConfigs); err != nil {
		return fmt.Errorf("build team network: %w", err)
	}

	plan := &containerPlan{
		team:          team,
		name:          cfg.Name,
		sanitizedName: sanitizeContainerName(cfg.Name),
		order:         cfgIndex,
		ipAddress:     record.IPAddress,
		setupScripts:  append([]string(nil), cfg.SetupScript...),
		options: &proxmoxAPI.ContainerCreateOptions{
			TemplatePath:     comp.ContainerRestrictions.Template,
			StoragePool:      comp.ContainerRestrictions.StoragePool,
			Hostname:         fmt.Sprintf("%s-team-%d-%s", comp.ContainerRestrictions.HostnamePrefix, teamIndex+1, cfg.Name),
			RootPassword:     comp.ContainerRestrictions.RootPassword,
			RootSSHPublicKey: publicKey,
			StorageSizeGB:    comp.ContainerRestrictions.StorageGB,
			MemoryMB:         comp.ContainerRestrictions.MemoryMB,
			Cores:            comp.ContainerRestrictions.Cores,
			GatewayIPv4:      comp.ContainerRestrictions.GatewayIPv4,
			IPv4Address:      record.IPAddress,
			CIDRBlock:        config.Config.Network.ContainerCIDR,
			NameServer:       comp.ContainerRestrictions.Nameserver,
			SearchDomain:     comp.ContainerRestrictions.SearchDomain,
		},
	}

	var publicFolderURL = competitionPublicFolderURL(comp)
	var artifactBaseURL = buildCompetitionArtifactBase(externalBaseURL(), comp.SystemID)

	var node *proxmox.Node
	if trimmedNode := strings.TrimSpace(record.NodeName); trimmedNode != "" {
		node = api.NodeByName(trimmedNode)
	}
	if node == nil {
		node = api.NextNode()
	}
	if node == nil {
		return fmt.Errorf("no proxmox nodes available for redeploy")
	}

	if err = deleteExistingContainer(record.PVEID); err != nil {
		return err
	}

	var createResult *proxmoxAPI.ProxmoxAPICreateResult
	if createResult, err = api.CreateContainerWithID(node, plan.options, int(record.PVEID)); err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	var newContainer = createResult.Container

	defer func() {
		if newContainer == nil {
			return
		}
		if err == nil {
			return
		}
		if stopErr := api.StopContainer(newContainer); stopErr != nil {
			log.Errorf("Failed to stop container %d after failed redeploy: %v\n", record.PVEID, stopErr)
		}
		if delErr := api.DeleteContainer(newContainer); delErr != nil {
			log.Errorf("Failed to clean up container %d after failed redeploy: %v\n", record.PVEID, delErr)
		}
	}()

	if err = api.StartContainer(newContainer); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	if err = ssh.WaitOnline(plan.ipAddress); err != nil {
		return fmt.Errorf("container %d did not come online: %w", record.PVEID, err)
	}

	var conn *ssh.SSHConnection
	if conn, err = ssh.ConnectOnceReadyWithRetry("root", plan.ipAddress, 22, 5, ssh.WithPrivateKey(privateKey)); err != nil {
		return fmt.Errorf("failed to connect to container %d via SSH: %w", record.PVEID, err)
	}
	defer conn.Close()

	if err = runSetupScripts(log, conn, comp, plan, network, publicFolderURL, artifactBaseURL, true); err != nil {
		return err
	}

	if stopErr := api.StopContainer(newContainer); stopErr != nil {
		return fmt.Errorf("failed to stop container after redeploy: %w", stopErr)
	}

	record.NodeName = newContainer.Node
	record.StoragePool = plan.options.StoragePool
	record.Status = "stopped"
	record.TeamID = team.ID
	record.ConfigName = strings.TrimSpace(cfg.Name)
	record.LastUpdated = time.Now()
	if updateErr := db.Containers.Update(record); updateErr != nil {
		log.Errorf("failed to update container %d metadata: %v\n", record.PVEID, updateErr)
	}

	team.LastUpdated = time.Now()
	if updateErr := db.Teams.Update(team); updateErr != nil {
		log.Errorf("failed to update team %d metadata: %v\n", team.ID, updateErr)
	}

	if startAfter {
		if err = api.StartContainer(newContainer); err != nil {
			return fmt.Errorf("failed to start container after redeploy: %w", err)
		}

		record.Status = "running"
		record.LastUpdated = time.Now()
		if updateErr := db.Containers.Update(record); updateErr != nil {
			log.Errorf("failed to update container %d metadata after start: %v\n", record.PVEID, updateErr)
		}

		team.LastUpdated = time.Now()
		if updateErr := db.Teams.Update(team); updateErr != nil {
			log.Errorf("failed to update team %d metadata after start: %v\n", team.ID, updateErr)
		}
	}

	return nil
}

func deleteExistingContainer(ctID int64) error {
	if api == nil {
		return fmt.Errorf("proxmox API is not initialized")
	}

	var existing *proxmox.Container
	var err error
	if existing, err = api.Container(int(ctID)); err != nil {
		if !errors.Is(err, proxmox.ErrNotFound) {
			return fmt.Errorf("lookup existing container: %w", err)
		}
		return nil
	}

	_ = api.StopContainer(existing)
	if err = api.DeleteContainer(existing); err != nil {
		return fmt.Errorf("delete existing container %d: %w", ctID, err)
	}

	return nil
}

func findCompetitionForContainer(ctID int64) (*db.Competition, error) {
	comps, err := db.Competitions.SelectAll()
	if err != nil {
		return nil, fmt.Errorf("fetch competitions: %w", err)
	}

	for _, comp := range comps {
		if comp == nil {
			continue
		}

		for _, existing := range comp.ContainerIDs {
			if existing == ctID {
				return comp, nil
			}
		}
	}

	return nil, fmt.Errorf("container %d is not assigned to a competition", ctID)
}

func resolveTeamForContainer(comp *db.Competition, record *db.Container) (*db.Team, int, error) {
	if comp == nil || record == nil {
		return nil, 0, fmt.Errorf("competition or container is nil")
	}

	if record.TeamID != 0 {
		if team, err := db.Teams.Select(record.TeamID); err == nil && team != nil {
			if containsContainerID(team.ContainerIDs, record.PVEID) {
				if idx := findTeamIndex(comp.TeamIDs, team.ID); idx >= 0 {
					return team, idx, nil
				}
			}
		}
	}

	for idx, teamID := range comp.TeamIDs {
		team, err := db.Teams.Select(teamID)
		if err != nil {
			return nil, 0, fmt.Errorf("load team %d: %w", teamID, err)
		}
		if team == nil {
			continue
		}
		if containsContainerID(team.ContainerIDs, record.PVEID) {
			record.TeamID = team.ID
			return team, idx, nil
		}
	}

	return nil, 0, fmt.Errorf("container %d is not associated with any team", record.PVEID)
}

func resolveContainerConfig(configs []db.TeamContainerConfig, record *db.Container) (db.TeamContainerConfig, int, error) {
	if len(configs) == 0 {
		return db.TeamContainerConfig{}, 0, fmt.Errorf("no container configs available")
	}

	name := strings.TrimSpace(record.ConfigName)
	if name != "" {
		for idx, cfg := range configs {
			if strings.EqualFold(strings.TrimSpace(cfg.Name), name) {
				return cfg, idx, nil
			}
		}
	}

	if record.IPAddress != "" {
		if offset := lastOctet(record.IPAddress); offset >= 0 {
			for idx, cfg := range configs {
				if cfg.LastOctetValue == offset {
					return cfg, idx, nil
				}
			}
		}
	}

	return db.TeamContainerConfig{}, 0, fmt.Errorf("unable to match container %d to a config", record.PVEID)
}

func lastOctet(address string) int {
	ip := net.ParseIP(strings.TrimSpace(address))
	if ip == nil {
		return -1
	}
	bytes := ip.To4()
	if bytes == nil {
		return -1
	}
	return int(bytes[3])
}

func containsContainerID(slice []int64, target int64) bool {
	for _, entry := range slice {
		if entry == target {
			return true
		}
	}
	return false
}

func findTeamIndex(teamIDs []int64, teamID int64) int {
	for idx, id := range teamIDs {
		if id == teamID {
			return idx
		}
	}
	return -1
}
