package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVMRestrictionsInitializeAndLookup(t *testing.T) {
	restrictions := VMRestrictionsConfig{Templates: []VMTemplateConfig{{
		VMID:                155,
		Name:                " koth-template-windows-11-pro-n-workstation ",
		OS:                  "Windows",
		Shell:               "PowerShell",
		NetworkConfigurator: "PowerShell",
	}}}

	require.NoError(t, restrictions.initialize())
	template, ok := restrictions.Template("koth-template-windows-11-pro-n-workstation")
	require.True(t, ok)
	assert.Equal(t, 155, template.VMID)
	assert.Equal(t, "windows", template.OS)
	assert.Equal(t, "powershell", template.Shell)
	assert.Equal(t, "scsi0", template.BootDisk)
}

func TestVMRestrictionsRejectInvalidTemplates(t *testing.T) {
	tests := []struct {
		name      string
		templates []VMTemplateConfig
		match     string
	}{
		{name: "missing vmid", templates: []VMTemplateConfig{{Name: "windows", OS: "windows", Shell: "powershell", NetworkConfigurator: "powershell"}}, match: "invalid vmid"},
		{name: "duplicate name", templates: []VMTemplateConfig{
			{VMID: 155, Name: "windows", OS: "windows", Shell: "powershell", NetworkConfigurator: "powershell"},
			{VMID: 156, Name: "windows", OS: "windows", Shell: "powershell", NetworkConfigurator: "powershell"},
		}, match: "duplicate VM template name"},
		{name: "duplicate vmid", templates: []VMTemplateConfig{
			{VMID: 155, Name: "windows-11", OS: "windows", Shell: "powershell", NetworkConfigurator: "powershell"},
			{VMID: 155, Name: "windows-server", OS: "windows", Shell: "powershell", NetworkConfigurator: "powershell"},
		}, match: "reuses vmid"},
		{name: "wrong shell", templates: []VMTemplateConfig{{VMID: 155, Name: "windows", OS: "windows", Shell: "bash", NetworkConfigurator: "powershell"}}, match: "must use powershell"},
		{name: "wrong network configurator", templates: []VMTemplateConfig{{VMID: 155, Name: "windows", OS: "windows", Shell: "powershell", NetworkConfigurator: "netplan"}}, match: "incompatible network_configurator"},
		{name: "bad boot disk", templates: []VMTemplateConfig{{VMID: 155, Name: "windows", OS: "windows", Shell: "powershell", NetworkConfigurator: "powershell", BootDisk: "disk0"}}, match: "invalid boot_disk"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restrictions := VMRestrictionsConfig{Templates: test.templates}
			err := restrictions.initialize()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.match)
		})
	}
}
