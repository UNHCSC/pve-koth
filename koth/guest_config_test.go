package koth

import (
	"encoding/json"
	"testing"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureTestVMTemplate(t *testing.T, vmid int, name, osType, shell, networkConfigurator string) {
	t.Helper()
	original := config.Config.VMRestrictions
	config.Config.VMRestrictions = config.VMRestrictionsConfig{Templates: []config.VMTemplateConfig{{
		VMID:                vmid,
		Name:                name,
		OS:                  osType,
		Shell:               shell,
		NetworkConfigurator: networkConfigurator,
		BootDisk:            "scsi0",
	}}}
	t.Cleanup(func() { config.Config.VMRestrictions = original })
}

func TestSchemaVersionTwoJSONDecoding(t *testing.T) {
	configureTestVMTemplate(t, 9002, "fedora-44", "linux", "bash", "networkmanager")
	raw := []byte(`{
		"schemaVersion": 2,
		"guestSpecTemplates": {
			"fedora": {
				"templateRef": "fedora-44",
				"storagePool": "team",
				"username": "root",
				"password": "temporary",
				"diskSizeGB": 20,
				"memoryMB": 2048,
				"cores": 2,
				"fullClone": false,
				"networkConfigurator": "networkmanager"
			}
		},
		"teamGuestConfigs": [{
			"name": "server",
			"lastOctetValue": 10,
			"guestSpecsTemplate": "fedora"
		}]
	}`)

	var request db.CreateCompetitionRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	require.NoError(t, NormalizeGuestConfiguration(&request))

	template := request.GuestSpecTemplates["fedora"]
	assert.Equal(t, 9002, template.TemplateVMID)
	assert.Equal(t, db.ScriptShellBash, template.Shell)
	assert.Equal(t, db.NetworkConfiguratorNetworkManager, template.NetworkConfigurator)
	assert.False(t, template.FullCloneEnabled())
	assert.Equal(t, "fedora", request.TeamGuestConfigs[0].GuestSpecsTemplate)
}

func TestNormalizeGuestConfigurationLegacy(t *testing.T) {
	request := &db.CreateCompetitionRequest{
		ContainerSpecsTemplates: map[string]db.ContainerSpecTemplate{
			"ubuntu": {
				TemplatePath:  " local:vztmpl/ubuntu.tar.zst ",
				StoragePool:   " team ",
				RootPassword:  "temporary",
				StorageSizeGB: 8,
				MemoryMB:      1024,
				Cores:         2,
			},
		},
		TeamContainerConfigs: []db.TeamContainerConfig{
			{Name: "web", LastOctetValue: 10, ContainerSpecsTemplate: "ubuntu"},
		},
	}

	require.NoError(t, NormalizeGuestConfiguration(request))
	require.NoError(t, NormalizeGuestConfiguration(request), "normalization must be idempotent")
	require.Len(t, request.GuestSpecTemplates, 1)
	require.Len(t, request.TeamGuestConfigs, 1)

	template := request.GuestSpecTemplates["ubuntu"]
	assert.Equal(t, db.GuestKindLXC, template.Kind)
	assert.Equal(t, db.GuestOSLinux, template.OS)
	assert.Equal(t, db.ScriptShellBash, template.Shell)
	assert.Equal(t, db.NetworkConfiguratorPVE, template.NetworkConfigurator)
	assert.Equal(t, "local:vztmpl/ubuntu.tar.zst", template.TemplatePath)
	assert.Equal(t, "team", template.StoragePool)
	assert.Equal(t, "root", template.Username)
	assert.Equal(t, "temporary", template.Password)
	assert.Equal(t, 8, template.DiskSizeGB)
	assert.Equal(t, "ubuntu", request.TeamGuestConfigs[0].GuestSpecsTemplate)
}

func TestNormalizeGuestConfigurationVersionTwoLXCProjectsLegacyConfig(t *testing.T) {
	request := &db.CreateCompetitionRequest{
		SchemaVersion: CompetitionSchemaVersion,
		GuestSpecTemplates: map[string]db.GuestSpecTemplate{
			"debian": {
				Kind:         db.GuestKindLXC,
				TemplatePath: "local:vztmpl/debian.tar.zst",
				StoragePool:  "team",
				Password:     "temporary",
				DiskSizeGB:   8,
				MemoryMB:     1024,
				Cores:        2,
			},
		},
		TeamGuestConfigs: []db.TeamGuestConfig{
			{Name: "web", LastOctetValue: 10, GuestSpecsTemplate: "debian"},
		},
	}

	require.NoError(t, NormalizeGuestConfiguration(request))
	require.NoError(t, NormalizeGuestConfiguration(request), "normalization must be idempotent")
	require.Len(t, request.ContainerSpecsTemplates, 1)
	require.Len(t, request.TeamContainerConfigs, 1)

	template := request.GuestSpecTemplates["debian"]
	assert.Equal(t, db.GuestOSLinux, template.OS)
	assert.Equal(t, db.ScriptShellBash, template.Shell)
	assert.Equal(t, db.NetworkConfiguratorPVE, template.NetworkConfigurator)
	assert.Equal(t, "temporary", request.ContainerSpecsTemplates["debian"].RootPassword)
	assert.Equal(t, "debian", request.TeamContainerConfigs[0].ContainerSpecsTemplate)
}

func TestNormalizeGuestConfigurationWindowsQEMU(t *testing.T) {
	configureTestVMTemplate(t, 9001, "windows-11", "windows", "powershell", "powershell")
	request := &db.CreateCompetitionRequest{
		SchemaVersion: CompetitionSchemaVersion,
		GuestSpecTemplates: map[string]db.GuestSpecTemplate{
			"windows-11": {
				TemplateRef:         "windows-11",
				StoragePool:         "team",
				Username:            "kothadmin",
				Password:            "temporary",
				DiskSizeGB:          40,
				MemoryMB:            4096,
				Cores:               2,
				NetworkConfigurator: db.NetworkConfiguratorPowerShell,
			},
		},
		TeamGuestConfigs: []db.TeamGuestConfig{
			{Name: "desktop", LastOctetValue: 20, GuestSpecsTemplate: "windows-11"},
		},
	}

	require.NoError(t, NormalizeGuestConfiguration(request))
	template := request.GuestSpecTemplates["windows-11"]
	assert.Equal(t, db.ScriptShellPowerShell, template.Shell)
	assert.Equal(t, "scsi0", template.BootDisk)
	assert.True(t, template.FullCloneEnabled())
	assert.Empty(t, request.ContainerSpecsTemplates, "QEMU guests must not be projected into the LXC provisioner")

	_, err := ensureTemplateLookup(request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "QEMU provisioning is not implemented yet")
}

func TestNormalizeGuestConfigurationRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		request db.CreateCompetitionRequest
		match   string
	}{
		{
			name: "new fields require version two",
			request: db.CreateCompetitionRequest{
				GuestSpecTemplates: map[string]db.GuestSpecTemplate{"windows": {}},
			},
			match: "require schemaVersion 2",
		},
		{
			name: "schema two cannot mix legacy fields",
			request: db.CreateCompetitionRequest{
				SchemaVersion:           CompetitionSchemaVersion,
				GuestSpecTemplates:      map[string]db.GuestSpecTemplate{"windows": {}},
				ContainerSpecsTemplates: map[string]db.ContainerSpecTemplate{"legacy": {}},
			},
			match: "cannot mix",
		},
		{
			name: "windows requires powershell networking",
			request: db.CreateCompetitionRequest{
				SchemaVersion: CompetitionSchemaVersion,
				GuestSpecTemplates: map[string]db.GuestSpecTemplate{
					"windows": {
						Kind:                db.GuestKindQEMU,
						OS:                  db.GuestOSWindows,
						TemplateRef:         "windows",
						TemplateVMID:        9001,
						StoragePool:         "team",
						Username:            "admin",
						Password:            "temporary",
						DiskSizeGB:          40,
						MemoryMB:            4096,
						Cores:               2,
						NetworkConfigurator: db.NetworkConfiguratorNetplan,
					},
				},
			},
			match: "networkConfigurator conflicts",
		},
		{
			name: "team template must exist",
			request: db.CreateCompetitionRequest{
				SchemaVersion: CompetitionSchemaVersion,
				GuestSpecTemplates: map[string]db.GuestSpecTemplate{
					"debian": {
						Kind:         db.GuestKindLXC,
						TemplatePath: "debian.tar.zst",
						StoragePool:  "team",
						Password:     "temporary",
						DiskSizeGB:   8,
						MemoryMB:     1024,
						Cores:        1,
					},
				},
				TeamGuestConfigs: []db.TeamGuestConfig{{Name: "web", GuestSpecsTemplate: "missing"}},
			},
			match: `template "missing" is not defined`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "windows requires powershell networking" {
				configureTestVMTemplate(t, 9001, "windows", "windows", "powershell", "powershell")
			}
			err := NormalizeGuestConfiguration(&test.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.match)
		})
	}
}
