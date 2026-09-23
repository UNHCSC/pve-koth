package tests

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsVMCloneRespecAndPowerShell(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}
	templateConfig, ok := config.Config.VMRestrictions.Template("koth-template-windows-11-pro-n-workstation")
	if !ok {
		t.Skip("Windows VM template is not configured")
	}

	api := initProxmox(t)
	source, err := api.VirtualMachine(templateConfig.VMID)
	require.NoError(t, err)
	sourceDiskGB, err := proxmoxAPI.VirtualMachineDiskSizeGB(source, templateConfig.BootDisk)
	require.NoError(t, err)
	targetDiskGB := int(math.Ceil(sourceDiskGB)) + 1
	require.LessOrEqual(t, targetDiskGB*1024, config.Config.VMRestrictions.MaxDiskMB)

	cloneName := fmt.Sprintf("koth-test-windows-%d", time.Now().Unix())
	vm, cloneErr := api.CloneVirtualMachine(proxmoxAPI.VMCloneOptions{
		TemplateVMID: templateConfig.VMID,
		TemplateName: templateConfig.Name,
		Name:         cloneName,
		StoragePool:  config.Config.Proxmox.Testing.Storage,
		Full:         true,
		Cores:        2,
		MemoryMB:     4096,
		BootDisk:     templateConfig.BootDisk,
		DiskSizeGB:   targetDiskGB,
	})
	if vm != nil {
		t.Cleanup(func() {
			if err := api.StopVirtualMachine(vm); err != nil {
				t.Errorf("stop test VM %d: %v", vm.VMID, err)
			}
			if err := api.DeleteVirtualMachine(vm); err != nil {
				t.Errorf("delete test VM %d: %v", vm.VMID, err)
			}
		})
	}
	require.NoError(t, cloneErr)
	require.NotNil(t, vm)
	t.Logf("Created Windows test VM %d from template %d", vm.VMID, templateConfig.VMID)

	vm, err = api.VirtualMachine(int(vm.VMID))
	require.NoError(t, err)
	assert.Equal(t, cloneName, vm.VirtualMachineConfig.Name)
	assert.Equal(t, 2, vm.VirtualMachineConfig.Cores)
	assert.Equal(t, 4096, int(vm.VirtualMachineConfig.Memory))
	resizedDiskGB, err := proxmoxAPI.VirtualMachineDiskSizeGB(vm, templateConfig.BootDisk)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resizedDiskGB, float64(targetDiskGB))

	require.NoError(t, api.StartVirtualMachine(vm))
	require.NoError(t, waitForWindowsOOBE(api, vm, 30*time.Minute))

	script := `$ErrorActionPreference = "Stop"
$testPath = Join-Path $env:TEMP "pve-koth-qga-test.txt"
Set-Content -LiteralPath $testPath -Value "PVE_KOTH_POWERSHELL_OK" -Encoding ASCII
$value = Get-Content -LiteralPath $testPath -Raw
Remove-Item -LiteralPath $testPath -Force
Write-Output $value.Trim()
`
	result, err := api.ExecuteVirtualMachineCommand(vm, []string{
		"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "-",
	}, script, 2*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode, result.Stderr)
	assert.Contains(t, result.Stdout, "PVE_KOTH_POWERSHELL_OK")
	assert.False(t, result.Truncated)
}

func waitForWindowsOOBE(api *proxmoxAPI.ProxmoxAPI, vm *proxmox.VirtualMachine, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	command := []string{
		"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-ItemProperty -LiteralPath "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State").ImageState`,
	}
	for time.Now().Before(deadline) {
		if err := api.WaitForVirtualMachineAgent(vm, 2*time.Minute, 10*time.Second); err == nil {
			result, execErr := api.ExecuteVirtualMachineCommand(vm, command, "", time.Minute)
			if execErr == nil && result.ExitCode == 0 && strings.Contains(result.Stdout, "IMAGE_STATE_COMPLETE") {
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("Windows OOBE did not complete within %s", timeout)
}
