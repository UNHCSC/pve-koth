package koth

import (
	"fmt"
	"os"
	"time"

	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/sshcomm"
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
		localLog.Errorf("Failed to create SSH data directory: %v\n", err)
		return
	}

	if err = os.MkdirAll(fmt.Sprintf("./data/%s/public", request.CompetitionID), 0755); err != nil {
		localLog.Errorf("Failed to create public data directory: %v\n", err)
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

	if err = db.Competitions.Insert(comp); err != nil {
		localLog.Errorf("Failed to create competition record: %v\n", err)
		return
	}

	// 2. Write files to data dir
	localLog.Status("Writing files to data directory...")

	for _, file := range request.AttachedFiles {
		localLog.Statusf("Writing file: %s (%d bytes)", file.SourceFilePath, len(file.FileContent))
		if err = os.WriteFile(fmt.Sprintf("./data/%s/public/%s", request.CompetitionID, file.SourceFilePath), file.FileContent, 0644); err != nil {
			localLog.Errorf("Failed to write file (%s): %v\n", file.SourceFilePath, err)
			return
		}
	}

	localLog.Status("Generating SSH keypair...")
	var publicKey, privateKey string
	if publicKey, privateKey, err = sshcomm.CreateSSHKeyPair(fmt.Sprintf("./data/%s/ssh", request.CompetitionID)); err != nil {
		localLog.Errorf("Failed to generate SSH keypair: %v\n", err)
		return
	}

	// 3. Create templates, templatize them
	localLog.Status("Creating container templates...")

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
			localLog.Errorf("Failed to create team record: %v\n", err)
			return
		}

		comp.TeamIDs = append(comp.TeamIDs, team.ID)

		var configs []*proxmoxAPI.ContainerCreateOptions

		for _, templateCfg := range request.TeamContainerConfigs {
			for i := range request.NumTeams {
				// need way to track jobs with meta data.
				configs = append(configs, &proxmoxAPI.ContainerCreateOptions{
					TemplatePath:     request.ContainerSpecs.TemplatePath,
					StoragePool:      request.ContainerSpecs.StoragePool,
					Hostname:         fmt.Sprintf("%s-team-%d-%s", comp.ContainerRestrictions.HostnamePrefix, i+1, templateCfg.Name),
					RootPassword:     request.ContainerSpecs.RootPassword,
					RootSSHPublicKey: publicKey,
					StorageSizeGB:    request.ContainerSpecs.StorageSizeGB,
					MemoryMB:         request.ContainerSpecs.MemoryMB,
					Cores:            request.ContainerSpecs.Cores,
					GatewayIPv4:      request.ContainerSpecs.GatewayIPv4,
					IPv4Address:      fmt.Sprintf("10.224.%d.%d", i+1, templateCfg.LastOctetValue),
					CIDRBlock:        request.ContainerSpecs.CIDRBlock,
					NameServer:       request.ContainerSpecs.NameServerIPv4,
					SearchDomain:     request.ContainerSpecs.SearchDomain,
				})
			}
		}

		// for _, templateCfg := range request.TeamContainerConfigs {
		// 	localLog.Statusf("Creating template for container: %s...\n", templateCfg.Name)

		// 	var (
		// 		ct              *proxmox.Container
		// 		ctID            int
		// 		conn            *sshcomm.SSHConnection
		// 		containerConfig *proxmoxAPI.ContainerCreateOptions = &proxmoxAPI.ContainerCreateOptions{
		// 			TemplatePath:     request.ContainerSpecs.TemplatePath,
		// 			StoragePool:      request.ContainerSpecs.StoragePool,
		// 			Hostname:         fmt.Sprintf("%s-template-%s", comp.ContainerRestrictions.HostnamePrefix, templateCfg.Name),
		// 			RootPassword:     request.ContainerSpecs.RootPassword,
		// 			RootSSHPublicKey: publicKey,
		// 			StorageSizeGB:    request.ContainerSpecs.StorageSizeGB,
		// 			MemoryMB:         request.ContainerSpecs.MemoryMB,
		// 			Cores:            request.ContainerSpecs.Cores,
		// 			GatewayIPv4:      request.ContainerSpecs.GatewayIPv4,
		// 			IPv4Address:      fmt.Sprintf("10.0.%d.%d", 224, templateCfg.LastOctetValue),
		// 			CIDRBlock:        request.ContainerSpecs.CIDRBlock,
		// 			NameServer:       request.ContainerSpecs.NameServerIPv4,
		// 			SearchDomain:     request.ContainerSpecs.SearchDomain,
		// 		}
		// 	)

		// 	if ct, ctID, err = api.CreateContainer(api.NextNode(), containerConfig); err != nil {
		// 		localLog.Errorf("Failed to create container: %v\n", err)
		// 		return
		// 	}

		// 	localLog.Statusf("Container %s created (CTID: %d). Beginning setup...\n", containerConfig.Hostname, ctID)

		// 	if err = api.StartContainer(ct); err != nil {
		// 		localLog.Errorf("Failed to start container (CTID: %d): %v\n", ctID, err)
		// 		return
		// 	}

		// 	if err = sshcomm.WaitOnline(containerConfig.IPv4Address); err != nil {
		// 		localLog.Errorf("Container (CTID: %d) failed to come online: %v", ctID, err)
		// 		return
		// 	}

		// 	localLog.Statusf("Container (CTID: %d) is online. Running setup script(s)...", ctID)
		// 	if conn, err = sshcomm.Connect("root", containerConfig.IPv4Address, 22, sshcomm.WithPrivateKey([]byte(privateKey))); err != nil {
		// 		localLog.Errorf("Failed to connect to container (CTID: %d) via SSH: %v\n", ctID, err)
		// 		return
		// 	}

		// 	defer conn.Close()

		// 	var envs = map[string]any{
		// 		"KOTH_COMP_ID":               request.CompetitionID,
		// 		"KOTH_ACCESS_TOKEN":          "test_token",
		// 		"KOTH_PUBLIC_FOLDER":         fmt.Sprintf("http://%s:%d/api/competitions/%s/public", sshcomm.MustLocalIP(), strings.Split(config.Config.WebServer.Address, ":")[1], request.CompetitionID),
		// 		"KOTH_TEAM_ID":               team.ID,
		// 		"KOTH_HOSTNAME":              containerConfig.Hostname,
		// 		"KOTH_IP":                    containerConfig.IPv4Address,
		// 		"KOTH_CONTAINER_IPS_grafana": containerConfig.IPv4Address,
		// 		"KOTH_CONTAINER_IPS":         containerConfig.IPv4Address,
		// 	}
		// }
	}

	// 4. Store in DB

	localLog.Successf("Successfully created competition: %s\n", request.CompetitionName)
	return
}
