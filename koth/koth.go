package koth

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/ssh"
	"github.com/z46-dev/go-logger"
)

var (
	api *proxmoxAPI.ProxmoxAPI
)

const (
	containerCreateRetries = 3
	containerStartRetries  = 3
	containerRetryDelay    = 5 * time.Second
)

var spinnerFrames = []string{"▁", "▃", "▄", "▅", "▆", "▇", "█", "▇", "▆", "▅", "▄", "▃"}

type threadSafeLogger struct {
	sink ProgressLogger
	mu   sync.Mutex
}

func (l *threadSafeLogger) Status(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sink.Status(message)
}

func (l *threadSafeLogger) Statusf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sink.Statusf(format, args...)
}

func (l *threadSafeLogger) Errorf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sink.Errorf(format, args...)
}

func (l *threadSafeLogger) Successf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sink.Successf(format, args...)
}

func wrapLoggerSafe(log ProgressLogger) ProgressLogger {
	if log == nil {
		return nil
	}
	if _, ok := log.(*threadSafeLogger); ok {
		return log
	}
	return &threadSafeLogger{sink: log}
}

func Init() (err error) {
	api, err = proxmoxAPI.InitProxmox()
	return
}

type ProgressLogger interface {
	Status(message string)
	Statusf(format string, args ...any)
	Errorf(format string, args ...any)
	Successf(format string, args ...any)
}

type containerPlan struct {
	team          *db.Team
	name          string
	sanitizedName string
	order         int
	ipAddress     string
	setupScripts  []string
	options       *proxmoxAPI.ContainerCreateOptions
}

type teamNetwork struct {
	ipsByName map[string]string
	ipOrder   []string
}

type provisionedContainer struct {
	plan     *containerPlan
	result   *proxmoxAPI.ProxmoxAPICreateResult
	recorded bool
}

func CreateNewComp(request *db.CreateCompetitionRequest) (comp *db.Competition, err error) {
	return CreateNewCompWithLogger(request, nil)
}

func CreateNewCompWithLogger(request *db.CreateCompetitionRequest, logSink ProgressLogger) (comp *db.Competition, err error) {
	var localLog ProgressLogger
	if logSink != nil {
		localLog = logSink
	} else {
		localLog = logger.NewLogger().SetPrefix(fmt.Sprintf("INIT %s", request.CompetitionName), logger.BoldCyan).IncludeTimestamp()
	}
	localLog = wrapLoggerSafe(localLog)

	localLog.Statusf("Creating new competition: %s\n", request.CompetitionName)

	// 1. Create structs & Data Dir(s)
	localLog.Status("Creating data directories...")
	var storageRoot = config.StorageBasePath()
	if err = os.MkdirAll(storageRoot, 0755); err != nil {
		localLog.Errorf("Failed to prepare storage base directory: %v\n", err)
		return
	}

	var dataDir = filepath.Clean(filepath.Join(storageRoot, "competitions", request.CompetitionID))
	if err = os.RemoveAll(dataDir); err != nil {
		localLog.Errorf("Failed to reset competition data directory: %v\n", err)
		return
	}
	if err = os.MkdirAll(filepath.Join(dataDir, "ssh"), 0755); err != nil {
		localLog.Errorf("Failed to create SSH data directory: %v\n", err)
		return
	}

	var packageRoot = filepath.Clean(request.PackagePath)
	if packageRoot == "" {
		err = fmt.Errorf("competition package path missing")
		localLog.Errorf("%v\n", err)
		return
	}

	if info, statErr := os.Stat(packageRoot); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = fmt.Errorf("package path is not a directory")
		}
		localLog.Errorf("Invalid package path %s: %v\n", packageRoot, statErr)
		err = statErr
		return
	}

	var publicFolderRel = sanitizeRelativePath(request.SetupPublicFolder)
	if publicFolderRel == "" {
		publicFolderRel = "public"
	}

	var publicSource = filepath.Join(packageRoot, publicFolderRel)
	if info, statErr := os.Stat(publicSource); statErr != nil {
		localLog.Errorf("Public folder %s unavailable: %v\n", publicSource, statErr)
		err = statErr
		return
	} else if !info.IsDir() {
		err = fmt.Errorf("public folder %s is not a directory", publicSource)
		localLog.Errorf("%v\n", err)
		return
	}

	localLog.Status("Allocating network resources...")
	var compSubnet *net.IPNet
	if compSubnet, err = allocateCompetitionSubnet(); err != nil {
		localLog.Errorf("Failed to allocate competition subnet: %v\n", err)
		return
	}

	localLog.Status("Creating competition record...")

	comp = &db.Competition{
		ID:             0,
		SystemID:       request.CompetitionID,
		Name:           request.CompetitionName,
		Description:    request.CompetitionDescription,
		Host:           request.CompetitionHost,
		TeamIDs:        []int64{},
		ContainerIDs:   []int64{},
		CreatedAt:      time.Now(),
		SSHPubKeyPath:  filepath.Join(dataDir, "ssh", "id_rsa.pub"),
		SSHPrivKeyPath: filepath.Join(dataDir, "ssh", "id_rsa"),
		ContainerRestrictions: db.ContainerRestrictions{
			HostnamePrefix: fmt.Sprintf("koth-%s", request.CompetitionID),
			RootPassword:   request.ContainerSpecs.RootPassword,
			Template:       request.ContainerSpecs.TemplatePath,
			StoragePool:    request.ContainerSpecs.StoragePool,
			GatewayIPv4:    request.ContainerSpecs.GatewayIPv4,
			Nameserver:     request.ContainerSpecs.NameServerIPv4,
			SearchDomain:   request.ContainerSpecs.SearchDomain,
			StorageGB:      request.ContainerSpecs.StorageSizeGB,
			MemoryMB:       request.ContainerSpecs.MemoryMB,
			Cores:          request.ContainerSpecs.Cores,
			IndividualCIDR: config.Config.Network.ContainerCIDR,
		},
		IsPrivate: !request.Privacy.Public,
		PrivateLDAPAllowedGroups: func() []string {
			if request.Privacy.Public {
				return []string{}
			}

			return []string(request.Privacy.LDAPAllowedGroupsFilter)
		}(),
		NetworkCIDR:        compSubnet.String(),
		SetupPublicFolder:  publicFolderRel,
		PackageStoragePath: packageRoot,
		ScoringActive:      false,
	}

	if err = db.Competitions.Insert(comp); err != nil {
		localLog.Errorf("Failed to create competition record: %v\n", err)
		return
	}

	localLog.Status("Generating SSH keypair...")
	var publicKey, privateKey string

	if publicKey, privateKey, err = ssh.CreateSSHKeyPair(filepath.Join(dataDir, "ssh")); err != nil {
		localLog.Errorf("Failed to generate SSH keypair: %v\n", err)
		return
	}

	if api == nil {
		err = fmt.Errorf("proxmox API is not initialized")
		localLog.Errorf("%v\n", err)
		return
	}

	// 3. Create containers for each team
	localLog.Status("Creating container templates...")
	var maxTeams = maxTeamsPerCompetition()
	if maxTeams == 0 || request.NumTeams > maxTeams {
		localLog.Errorf("Requested %d teams exceeds available /24 subnets (%d) in %s\n", request.NumTeams, maxTeams, comp.NetworkCIDR)
		return
	}

	var (
		plans        []*containerPlan
		teamNetworks = make(map[int64]*teamNetwork)
		teamLocks    = make(map[int64]*sync.Mutex)
		createdTeams []*db.Team
	)

	for teamIndex := 0; teamIndex < request.NumTeams; teamIndex++ {
		var teamSubnetBase uint32
		if teamSubnetBase, err = teamSubnetBaseIP(compSubnet, teamIndex); err != nil {
			localLog.Errorf("Failed to allocate subnet for team %d: %v\n", teamIndex+1, err)
			return
		}

		teamSubnet := buildSubnet(teamSubnetBase, config.Config.Network.TeamSubnetPrefix)
		if teamSubnet == nil {
			localLog.Errorf("Failed to determine subnet for team %d\n", teamIndex+1)
			return
		}

		var team *db.Team = &db.Team{
			ID:           0,
			Name:         fmt.Sprintf("Team %d", teamIndex+1),
			Score:        0,
			ContainerIDs: []int64{},
			LastUpdated:  time.Now(),
			CreatedAt:    time.Now(),
			NetworkCIDR:  teamSubnet.String(),
		}

		if err = db.Teams.Insert(team); err != nil {
			localLog.Errorf("Failed to create team record: %v\n", err)
			return
		}

		createdTeams = append(createdTeams, team)
		comp.TeamIDs = append(comp.TeamIDs, team.ID)
		teamNetworks[team.ID] = &teamNetwork{
			ipsByName: make(map[string]string),
			ipOrder:   make([]string, 0),
		}
		teamLocks[team.ID] = &sync.Mutex{}

		for templateOrder, templateCfg := range request.TeamContainerConfigs {
			var hostIP net.IP
			if hostIP, err = hostIPWithinSubnet(teamSubnetBase, config.Config.Network.TeamSubnetPrefix, templateCfg.LastOctetValue); err != nil {
				localLog.Errorf("Failed to allocate container IP for %s (team %d): %v\n", templateCfg.Name, teamIndex+1, err)
				return
			}

			var sanitizedName = sanitizeContainerName(templateCfg.Name)
			teamNetworks[team.ID].ipsByName[sanitizedName] = hostIP.String()
			teamNetworks[team.ID].ipOrder = append(teamNetworks[team.ID].ipOrder, hostIP.String())

			var plan = &containerPlan{
				team:          team,
				name:          templateCfg.Name,
				sanitizedName: sanitizedName,
				order:         templateOrder,
				ipAddress:     hostIP.String(),
				setupScripts:  append([]string(nil), templateCfg.SetupScript...),
				options: &proxmoxAPI.ContainerCreateOptions{
					TemplatePath:     request.ContainerSpecs.TemplatePath,
					StoragePool:      request.ContainerSpecs.StoragePool,
					Hostname:         fmt.Sprintf("%s-team-%d-%s", comp.ContainerRestrictions.HostnamePrefix, teamIndex+1, templateCfg.Name),
					RootPassword:     request.ContainerSpecs.RootPassword,
					RootSSHPublicKey: publicKey,
					StorageSizeGB:    request.ContainerSpecs.StorageSizeGB,
					MemoryMB:         request.ContainerSpecs.MemoryMB,
					Cores:            request.ContainerSpecs.Cores,
					GatewayIPv4:      request.ContainerSpecs.GatewayIPv4,
					IPv4Address:      hostIP.String(),
					CIDRBlock:        config.Config.Network.ContainerCIDR,
					NameServer:       request.ContainerSpecs.NameServerIPv4,
					SearchDomain:     request.ContainerSpecs.SearchDomain,
				},
			}

			plans = append(plans, plan)
		}
	}

	if len(plans) == 0 {
		localLog.Status("No team container configurations provided; skipping container provisioning.")
		if err = db.Competitions.Update(comp); err != nil {
			localLog.Errorf("Failed to update competition record: %v\n", err)
		}
		localLog.Successf("Successfully created competition: %s\n", request.CompetitionName)
		return
	}

	var (
		provisioned         []*provisionedContainer
		provisionedMu       sync.Mutex
		totalContainers     = len(plans)
		completedContainers int32
	)

	defer func() {
		if err != nil {
			cleanupProvisionedContainers(localLog, comp, provisioned)
			cleanupFailedCompetitionResources(localLog, comp, createdTeams, dataDir)
		}
	}()

	var compLock sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		publicFolderURL = competitionPublicFolderURL(comp)
		artifactBaseURL = buildCompetitionArtifactBase(externalBaseURL(), comp.SystemID)
	)

	localLog.Statusf("Provisioning progress: (0/%d) containers complete.", totalContainers)

	errCh := make(chan error, len(plans))
	var wg sync.WaitGroup

	for _, plan := range plans {
		wg.Add(1)
		network := teamNetworks[plan.team.ID]
		teamLock := teamLocks[plan.team.ID]

		go func(plan *containerPlan, network *teamNetwork, teamLock *sync.Mutex) {
			defer wg.Done()
			entry, perr := provisionContainerPlan(ctx, localLog, plan, comp, network, privateKey, publicFolderURL, artifactBaseURL, teamLock, &compLock)
			if entry != nil {
				provisionedMu.Lock()
				provisioned = append(provisioned, entry)
				provisionedMu.Unlock()
			}
			if perr == nil && entry != nil && entry.recorded {
				current := atomic.AddInt32(&completedContainers, 1)
				localLog.Statusf("Provisioning progress: (%d/%d) containers complete.", current, totalContainers)
			}
			if perr != nil {
				select {
				case errCh <- perr:
				default:
				}
				cancel()
			}
		}(plan, network, teamLock)
	}

	wg.Wait()

	select {
	case provisionErr := <-errCh:
		err = provisionErr
		return
	default:
	}

	if err = db.Competitions.Update(comp); err != nil {
		localLog.Errorf("Failed to update competition record: %v\n", err)
		return
	}

	// 4. Store in DB

	localLog.Successf("Successfully created competition: %s\n", request.CompetitionName)
	return
}

func provisionContainerPlan(ctx context.Context, log ProgressLogger, plan *containerPlan, comp *db.Competition, network *teamNetwork, privateKey, publicFolderURL, artifactBaseURL string, teamLock *sync.Mutex, compLock *sync.Mutex) (entry *provisionedContainer, err error) {
	if plan == nil {
		return nil, fmt.Errorf("container plan is nil")
	}

	log.Statusf("Provisioning container %s for %s...", plan.options.Hostname, plan.team.Name)
	var createResult *proxmoxAPI.ProxmoxAPICreateResult
	if err = retryWithDelay(ctx, containerCreateRetries, containerRetryDelay, func(attempt int) error {
		log.Statusf("Creating container %s (attempt %d/%d)...", plan.options.Hostname, attempt+1, containerCreateRetries)
		result, createErr := api.CreateContainer(api.NextNode(), plan.options)
		if createErr != nil {
			log.Errorf("Failed to create container %s on attempt %d: %v\n", plan.options.Hostname, attempt+1, createErr)
			return createErr
		}
		createResult = result
		return nil
	}); err != nil {
		return nil, err
	}

	entry = &provisionedContainer{
		plan:   plan,
		result: createResult,
	}

	if err = retryWithDelay(ctx, containerStartRetries, containerRetryDelay, func(attempt int) error {
		log.Statusf("Starting container %s (CTID: %d) attempt %d/%d...", plan.options.Hostname, createResult.CTID, attempt+1, containerStartRetries)
		startErr := api.StartContainer(createResult.Container)
		if startErr != nil {
			log.Errorf("Failed to start container %d on attempt %d: %v\n", createResult.CTID, attempt+1, startErr)
		}
		return startErr
	}); err != nil {
		return entry, err
	}

	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return entry, ctxErr
		}
	}

	log.Statusf("Waiting for container %d (%s) to come online...", createResult.CTID, plan.ipAddress)
	if err = ssh.WaitOnline(plan.ipAddress); err != nil {
		log.Errorf("Container %d did not come online: %v\n", createResult.CTID, err)
		return entry, err
	}

	var conn *ssh.SSHConnection
	if conn, err = ssh.ConnectOnceReadyWithRetry("root", plan.ipAddress, 22, 5, ssh.WithPrivateKey([]byte(privateKey))); err != nil {
		log.Errorf("Failed to connect to container %d via SSH: %v\n", createResult.CTID, err)
		return entry, err
	}
	defer conn.Close()

	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return entry, ctxErr
		}
	}

	if err = runSetupScripts(log, conn, comp, plan, network, publicFolderURL, artifactBaseURL, false); err != nil {
		return entry, err
	}

	var record *db.Container
	if record, err = recordProvisionedContainer(comp, plan.team, plan, createResult, plan.ipAddress, plan.options.StoragePool, createResult.Container.Node, teamLock, compLock); err != nil {
		log.Errorf("Failed to record container %d: %v\n", createResult.CTID, err)
		return entry, err
	}
	entry.recorded = true

	log.Statusf("Stopping container %s (CTID: %d) after provisioning...", plan.options.Hostname, createResult.CTID)
	if err = api.StopContainer(createResult.Container); err != nil {
		log.Errorf("Failed to stop container %d after provisioning: %v\n", createResult.CTID, err)
		return entry, err
	}

	record.Status = "stopped"
	record.LastUpdated = time.Now()
	if updateErr := db.Containers.Update(record); updateErr != nil {
		log.Errorf("Failed to update container %d metadata: %v\n", record.PVEID, updateErr)
	}

	log.Statusf("Container %s (CTID: %d) provisioned successfully.", plan.options.Hostname, createResult.CTID)
	return entry, nil
}

func runSetupScripts(log ProgressLogger, conn *ssh.SSHConnection, comp *db.Competition, plan *containerPlan, network *teamNetwork, publicFolderURL, artifactBaseURL string, logEnv bool) (err error) {
	if len(plan.setupScripts) == 0 {
		log.Statusf("No setup scripts defined for %s; skipping.", plan.options.Hostname)
		return nil
	}

	var envs = buildScriptEnv(comp, plan, network, publicFolderURL)
	const tokenTTL = 30 * time.Minute
	var token = IssueAccessToken(comp.SystemID, tokenTTL)
	envs["KOTH_ACCESS_TOKEN"] = token
	defer RevokeAccessToken(token)

	if logEnv {
		log.Statusf("Script environment: %s", formatScriptEnv(envs))
	}

	for _, scriptPath := range plan.setupScripts {
		var scriptURL = buildArtifactFileURL(artifactBaseURL, scriptPath)
		log.Statusf("Running setup script %s on container %s...", scriptPath, plan.options.Hostname)

		var (
			exitCode int
			command  = ssh.LoadAndRunScript(scriptURL, token, envs)
		)

		if exitCode, _, err = conn.SendWithOutput(command); err != nil {
			log.Errorf("Failed to execute setup script %s on %s: %v\n", scriptPath, plan.options.Hostname, err)
			return
		}

		log.Statusf("Setup script %s exited with %d.", scriptPath, exitCode)

		if exitCode != 0 {
			err = fmt.Errorf("setup script %s exited with code %d", scriptPath, exitCode)
			log.Errorf("%v\n", err)
			return
		}

		if err = conn.Reset(); err != nil {
			log.Errorf("Failed to reset SSH session for %s: %v\n", plan.options.Hostname, err)
			return
		}
	}

	return nil
}

func buildScriptEnv(comp *db.Competition, plan *containerPlan, network *teamNetwork, publicFolderURL string) map[string]any {
	var envs = map[string]any{
		"KOTH_COMP_ID":       comp.SystemID,
		"KOTH_TEAM_ID":       fmt.Sprintf("%d", plan.team.ID),
		"KOTH_HOSTNAME":      plan.options.Hostname,
		"KOTH_IP":            plan.ipAddress,
		"KOTH_PUBLIC_FOLDER": publicFolderURL,
	}

	if network != nil {
		envs["KOTH_CONTAINER_IPS"] = strings.Join(network.ipOrder, ",")

		var names []string
		for name := range network.ipsByName {
			names = append(names, name)
		}

		sort.Strings(names)
		for _, name := range names {
			envs[fmt.Sprintf("KOTH_CONTAINER_IPS_%s", name)] = network.ipsByName[name]
		}
	} else {
		envs["KOTH_CONTAINER_IPS"] = plan.ipAddress
		envs[fmt.Sprintf("KOTH_CONTAINER_IPS_%s", plan.sanitizedName)] = plan.ipAddress
	}

	return envs
}

func formatScriptEnv(envs map[string]any) string {
	if len(envs) == 0 {
		return ""
	}

	var keys = make([]string, 0, len(envs))
	for key := range envs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		value := envs[key]
		text := fmt.Sprintf("%v", value)
		if key == "KOTH_ACCESS_TOKEN" && text != "" {
			if len(text) > 8 {
				text = fmt.Sprintf("%s...", text[:8])
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, text))
	}

	return strings.Join(parts, " ")
}

func recordProvisionedContainer(comp *db.Competition, team *db.Team, plan *containerPlan, result *proxmoxAPI.ProxmoxAPICreateResult, ip, storagePool, nodeName string, teamLock *sync.Mutex, compLock *sync.Mutex) (record *db.Container, err error) {
	if teamLock != nil {
		teamLock.Lock()
		defer teamLock.Unlock()
	}
	if compLock != nil {
		compLock.Lock()
		defer compLock.Unlock()
	}
	record = &db.Container{
		PVEID:       int64(result.CTID),
		IPAddress:   ip,
		Status:      "running",
		TeamID:      plan.team.ID,
		ConfigName:  plan.name,
		StoragePool: storagePool,
		NodeName:    nodeName,
		LastUpdated: time.Now(),
		CreatedAt:   time.Now(),
	}

	if err = db.Containers.Insert(record); err != nil {
		return nil, err
	}

	team.ContainerIDs = append(team.ContainerIDs, record.PVEID)
	team.LastUpdated = time.Now()
	if err = db.Teams.Update(team); err != nil {
		return nil, err
	}

	comp.ContainerIDs = append(comp.ContainerIDs, record.PVEID)
	return record, nil
}

func cleanupProvisionedContainers(log ProgressLogger, comp *db.Competition, provisioned []*provisionedContainer) {
	for i := len(provisioned) - 1; i >= 0; i-- {
		var entry = provisioned[i]
		if entry == nil || entry.result == nil || entry.result.Container == nil {
			continue
		}

		log.Errorf("Cleaning up container %d after failure...\n", entry.result.CTID)
		if err := api.StopContainer(entry.result.Container); err != nil {
			log.Errorf("Failed to stop container %d: %v\n", entry.result.CTID, err)
		}

		if err := api.DeleteContainer(entry.result.Container); err != nil {
			log.Errorf("Failed to delete container %d: %v\n", entry.result.CTID, err)
		}

		if entry.recorded {
			var ctID = int64(entry.result.CTID)
			if err := db.Containers.Delete(ctID); err != nil {
				log.Errorf("Failed to remove container record %d: %v\n", ctID, err)
			}

			if entry.plan != nil && entry.plan.team != nil {
				entry.plan.team.ContainerIDs = removeIDFromSlice(entry.plan.team.ContainerIDs, ctID)
				entry.plan.team.LastUpdated = time.Now()
				if err := db.Teams.Update(entry.plan.team); err != nil {
					log.Errorf("Failed to update team %d during cleanup: %v\n", entry.plan.team.ID, err)
				}
			}

			if comp != nil {
				comp.ContainerIDs = removeIDFromSlice(comp.ContainerIDs, ctID)
			}
		}
	}
}

func removeIDFromSlice(source []int64, target int64) []int64 {
	if len(source) == 0 {
		return source
	}

	var result = make([]int64, 0, len(source))
	for _, id := range source {
		if id != target {
			result = append(result, id)
		}
	}

	return result
}

func cleanupFailedCompetitionResources(log ProgressLogger, comp *db.Competition, teams []*db.Team, dataDir string) {
	for _, team := range teams {
		if team == nil || team.ID == 0 {
			continue
		}
		if err := db.Teams.Delete(team.ID); err != nil {
			log.Errorf("Failed to remove team record %d during cleanup: %v\n", team.ID, err)
		}
	}

	if comp != nil && comp.ID != 0 {
		if err := db.Competitions.Delete(comp.ID); err != nil {
			log.Errorf("Failed to remove competition record %d during cleanup: %v\n", comp.ID, err)
		}
	}

	if dataDir != "" {
		if err := os.RemoveAll(dataDir); err != nil {
			log.Errorf("Failed to remove competition data directory %s: %v\n", dataDir, err)
		}
	}
}

func retryWithDelay(ctx context.Context, attempts int, delay time.Duration, fn func(attempt int) error) error {
	var err error
	for attempt := range attempts {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		if err = fn(attempt); err == nil {
			return nil
		}

		if attempt == attempts-1 {
			break
		}

		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		} else {
			time.Sleep(delay)
		}
	}

	return err
}

func externalBaseURL() string {
	if custom := strings.TrimSpace(config.Config.WebServer.PublicURL); custom != "" {
		return strings.TrimRight(custom, "/")
	}

	var scheme = "http"
	if config.Config.WebServer.TLSDir != "" {
		scheme = "https"
	}

	var host = ssh.MustLocalIP()
	if addr := strings.TrimSpace(config.Config.WebServer.Address); addr != "" {
		if h, port, err := net.SplitHostPort(addr); err == nil {
			if h != "" && h != "0.0.0.0" && h != "::" {
				host = h
			}

			if (scheme == "http" && port != "80") || (scheme == "https" && port != "443") {
				host = net.JoinHostPort(host, port)
			}
		}
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func buildCompetitionPublicBase(baseURL, competitionID string) string {
	var trimmed = strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/api/competitions/%s/public", trimmed, url.PathEscape(competitionID))
}

func buildCompetitionArtifactBase(baseURL, competitionID string) string {
	var trimmed = strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/api/competitions/%s/artifacts", trimmed, url.PathEscape(competitionID))
}

func buildArtifactFileURL(base, relativePath string) string {
	base = strings.TrimRight(base, "/")
	var encoded = encodeRelativePath(relativePath)
	if encoded == "" {
		return base
	}
	return base + "/" + encoded
}

func encodeRelativePath(relative string) string {
	relative = strings.TrimSpace(relative)
	relative = strings.TrimPrefix(relative, "/")
	relative = path.Clean(relative)
	if relative == "." || relative == "/" {
		return ""
	}

	relative = strings.TrimPrefix(relative, "../")
	relative = strings.TrimPrefix(relative, "./")
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" {
		return ""
	}

	var segments = strings.Split(relative, "/")
	var encoded []string
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		encoded = append(encoded, url.PathEscape(seg))
	}

	return strings.Join(encoded, "/")
}

func competitionPublicFolderURL(comp *db.Competition) string {
	if comp == nil {
		return ""
	}
	return buildCompetitionPublicBase(externalBaseURL(), comp.SystemID)
}

func sanitizeContainerName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "container"
	}

	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}

	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "container"
	}
	return result
}

func summarizeScriptOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "<no output>"
	}

	const limit = 1024
	if len(output) > limit {
		return output[:limit] + "..."
	}

	return output
}

func sanitizeRelativePath(relative string) string {
	relative = strings.TrimSpace(relative)
	relative = strings.TrimPrefix(relative, "/")
	relative = path.Clean(relative)
	if relative == "." || relative == "/" {
		return ""
	}

	for strings.HasPrefix(relative, "../") {
		relative = strings.TrimPrefix(relative, "../")
	}

	relative = strings.TrimPrefix(relative, "./")
	relative = strings.Trim(relative, "/")

	return relative
}
