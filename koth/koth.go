package koth

import (
	"fmt"
	"os"
	"time"

	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/sshcomm"
	"github.com/luthermonson/go-proxmox"
	"github.com/z46-dev/go-logger"
)

var api *proxmoxAPI.ProxmoxAPI

func init() {
	var err error

	if api, err = proxmoxAPI.InitProxmox(); err != nil {
		panic(fmt.Sprintf("failed to initialize Proxmox API: %v", err))
	}
}

func CreateNewComp(request *db.CreateCompetitionRequest) (comp *db.Competition, err error) {
	var localLog *logger.Logger = logger.NewLogger().SetPrefix(fmt.Sprintf("INIT %s", request.CompetitionName), logger.BoldCyan).IncludeTimestamp()

	localLog.Statusf("Creating new competition: %s\n", request.CompetitionName)

	// 1. Create structs & Data Dir(s)
	localLog.Status("Creating data directories...")
	if err = os.MkdirAll(fmt.Sprintf("./data/%s/ssh", request.CompetitionID), 0755); err != nil {
		localLog.Errorf("Failed to create SSH data directory: %s", err.Error())
		return
	}

	if err = os.MkdirAll(fmt.Sprintf("./data/%s/public", request.CompetitionID), 0755); err != nil {
		localLog.Errorf("Failed to create public data directory: %s", err.Error())
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
		SSHPubKeyPath:  fmt.Sprintf("./data/%s/ssh/id_rsa.pub", request.CompetitionID),
		SSHPrivKeyPath: fmt.Sprintf("./data/%s/ssh/id_rsa", request.CompetitionID),
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
			IndividualCIDR: request.ContainerSpecs.CIDRBlock,
		},
		IsPrivate: !request.Privacy.Public,
		PrivateLDAPAllowedGroups: func() []string {
			if request.Privacy.Public {
				return []string{}
			}

			return request.Privacy.LDAPAllowedGroupsFilter
		}(),
	}

	for i := range request.NumTeams {
		var team *db.Team = &db.Team{
			ID:           0,
			Name:         fmt.Sprintf("Team %d", i+1),
			Score:        0,
			ContainerIDs: []int64{},
			LastUpdated:  time.Now(),
			CreatedAt:    time.Now(),
		}

		if err = db.Teams.Insert(team); err != nil {
			localLog.Errorf("Failed to create team record: %s", err.Error())
			return
		}

		comp.TeamIDs = append(comp.TeamIDs, team.ID)
	}

	if err = db.Competitions.Insert(comp); err != nil {
		localLog.Errorf("Failed to create competition record: %s", err.Error())
		return
	}

	// 2. Write files to data dir
	localLog.Status("Writing files to data directory...")

	for _, file := range request.AttachedFiles {
		localLog.Statusf("Writing file: %s (%d bytes)", file.SourceFilePath, len(file.FileContent))
		if err = os.WriteFile(fmt.Sprintf("./data/%s/public/%s", request.CompetitionID, file.SourceFilePath), file.FileContent, 0644); err != nil {
			localLog.Errorf("Failed to write file (%s): %s", file.SourceFilePath, err.Error())
			return
		}
	}

	localLog.Status("Generating SSH keypair...")
	var publicKey, privateKey string
	if publicKey, privateKey, err = sshcomm.CreateSSHKeyPair(fmt.Sprintf("./data/%s/ssh", request.CompetitionID)); err != nil {
		localLog.Errorf("Failed to generate SSH keypair: %s", err.Error())
		return
	}

	// 3. Create templates, templatize them
	localLog.Status("Creating container templates...")

	for _, templateCfg := range request.TeamContainerConfigs {
		localLog.Statusf("Creating template for container: %s...", templateCfg.Name)

		var (
			ct              *proxmox.Container
			ctID            int
			conn            *sshcomm.SSHConnection
			containerConfig *proxmoxAPI.ContainerCreateOptions = &proxmoxAPI.ContainerCreateOptions{
				TemplatePath:     request.ContainerSpecs.TemplatePath,
				StoragePool:      request.ContainerSpecs.StoragePool,
				Hostname:         fmt.Sprintf("%s-template-%s", comp.ContainerRestrictions.HostnamePrefix, templateCfg.Name),
				RootPassword:     request.ContainerSpecs.RootPassword,
				RootSSHPublicKey: publicKey,
				StorageSizeGB:    request.ContainerSpecs.StorageSizeGB,
				MemoryMB:         request.ContainerSpecs.MemoryMB,
				Cores:            request.ContainerSpecs.Cores,
				GatewayIPv4:      request.ContainerSpecs.GatewayIPv4,
				IPv4Address:      fmt.Sprintf("10.0.%d.%d", 224, templateCfg.LastOctetValue),
				CIDRBlock:        request.ContainerSpecs.CIDRBlock,
				NameServer:       request.ContainerSpecs.NameServerIPv4,
				SearchDomain:     request.ContainerSpecs.SearchDomain,
			}
		)

		if ct, ctID, err = api.CreateContainer(api.NextNode(), containerConfig); err != nil {
			localLog.Errorf("Failed to create container: %s", err.Error())
			return
		}

		localLog.Statusf("Container %s created (CTID: %d). Beginning setup...\n", containerConfig.Hostname, ctID)

		if err = api.StartContainer(ct); err != nil {
			localLog.Errorf("Failed to start container (CTID: %d): %s", ctID, err.Error())
			return
		}

		if err = sshcomm.WaitOnline(containerConfig.IPv4Address); err != nil {
			localLog.Errorf("Container (CTID: %d) failed to come online: %s", ctID, err.Error())
			return
		}

		localLog.Statusf("Container (CTID: %d) is online. Running setup script(s)...", ctID)
		if conn, err = sshcomm.Connect("root", containerConfig.IPv4Address, 22, sshcomm.WithPrivateKey([]byte(privateKey))); err != nil {
			localLog.Errorf("Failed to connect to container (CTID: %d) via SSH: %s", ctID, err.Error())
			return
		}

		defer conn.Close()

		// var envs = map[string]any{
		// 	"KOTH_COMP_ID":               "testcomp",
		// 	"KOTH_ACCESS_TOKEN":          "test_token",
		// 	"KOTH_PUBLIC_FOLDER":         fmt.Sprintf("http://%s:%d/", sshcomm.MustLocalIP(), 8080),
		// 	"KOTH_TEAM_ID":               "team1",
		// 	"KOTH_HOSTNAME":              "koth-test-ct",
		// 	"KOTH_IP":                    containerConfig.IPv4Address,
		// 	"KOTH_CONTAINER_IPS_grafana": containerConfig.IPv4Address,
		// 	"KOTH_CONTAINER_IPS":         containerConfig.IPv4Address,
		// }
	}

	// 4. Store in DB

	localLog.Successf("Successfully created competition: %s", request.CompetitionName)
	return
}
