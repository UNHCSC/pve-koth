package proxmoxAPI

import (
	"testing"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVirtualMachineDiskSizeGB(t *testing.T) {
	vm := &proxmox.VirtualMachine{VirtualMachineConfig: &proxmox.VirtualMachineConfig{SCSI0: "team:vm-100-disk-0,cache=writeback,size=64G"}}
	size, err := VirtualMachineDiskSizeGB(vm, "scsi0")
	require.NoError(t, err)
	assert.Equal(t, float64(64), size)
}

func TestVirtualMachineDiskSizeGBRejectsInvalidDisk(t *testing.T) {
	vm := &proxmox.VirtualMachine{VirtualMachineConfig: &proxmox.VirtualMachineConfig{SCSI0: "team:vm-100-disk-0"}}
	_, err := VirtualMachineDiskSizeGB(vm, "scsi0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no parseable size")
}
