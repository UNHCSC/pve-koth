package app

import (
	"testing"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCompetitionTemplatesAcceptsLegacyConfiguration(t *testing.T) {
	original := config.Config.ContainerRestrictions
	config.Config.ContainerRestrictions = config.ContainerRestrictionsConfig{}
	t.Cleanup(func() { config.Config.ContainerRestrictions = original })

	request := &db.CreateCompetitionRequest{
		ContainerSpecsTemplates: map[string]db.ContainerSpecTemplate{
			"ubuntu": {
				TemplatePath:  "local:vztmpl/ubuntu.tar.zst",
				StoragePool:   "team",
				RootPassword:  "temporary",
				StorageSizeGB: 8,
				MemoryMB:      1024,
				Cores:         1,
			},
		},
		TeamContainerConfigs: []db.TeamContainerConfig{
			{Name: "web", ContainerSpecsTemplate: "ubuntu"},
		},
	}

	require.NoError(t, validateCompetitionTemplates(request))
	assert.Equal(t, db.GuestKindLXC, request.GuestSpecTemplates["ubuntu"].Kind)
}

func TestValidateCompetitionTemplatesRejectsQEMUUntilProviderExists(t *testing.T) {
	original := config.Config.ContainerRestrictions
	config.Config.ContainerRestrictions = config.ContainerRestrictionsConfig{}
	t.Cleanup(func() { config.Config.ContainerRestrictions = original })

	request := &db.CreateCompetitionRequest{
		SchemaVersion: koth.CompetitionSchemaVersion,
		GuestSpecTemplates: map[string]db.GuestSpecTemplate{
			"windows": {
				Kind:                db.GuestKindQEMU,
				OS:                  db.GuestOSWindows,
				TemplateVMID:        9001,
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
			{Name: "desktop", GuestSpecsTemplate: "windows"},
		},
	}

	err := validateCompetitionTemplates(request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "full-VM provisioning is not implemented yet")
}
