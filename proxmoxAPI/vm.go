package proxmoxAPI

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
)

const (
	defaultVMTaskTimeout    = 15 * time.Minute
	defaultVMCommandTimeout = 10 * time.Minute
)

var diskSizePattern = regexp.MustCompile(`(?:^|,)size=([0-9]+(?:\.[0-9]+)?)([KMGT])(?:,|$)`)

type VMCloneOptions struct {
	TemplateVMID int
	TemplateName string
	Name         string
	StoragePool  string
	TargetNode   string
	Full         bool
	Cores        int
	MemoryMB     int
	BootDisk     string
	DiskSizeGB   int
}

type VMCommandResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Truncated bool
}

func (api *ProxmoxAPI) VirtualMachine(vmid int) (*proxmox.VirtualMachine, error) {
	if vmid <= 0 {
		return nil, fmt.Errorf("invalid VMID %d", vmid)
	}
	for _, node := range api.Nodes {
		vm, err := node.VirtualMachine(api.bg, vmid)
		if err == nil {
			return vm, nil
		}
	}
	return nil, proxmox.ErrNotFound
}

func (api *ProxmoxAPI) CloneVirtualMachine(options VMCloneOptions) (*proxmox.VirtualMachine, error) {
	api.createLock.Lock()
	defer api.createLock.Unlock()

	if options.TemplateVMID <= 0 {
		return nil, fmt.Errorf("invalid template VMID %d", options.TemplateVMID)
	}
	if strings.TrimSpace(options.Name) == "" {
		return nil, fmt.Errorf("VM name is required")
	}

	template, err := api.VirtualMachine(options.TemplateVMID)
	if err != nil {
		return nil, fmt.Errorf("load VM template %d: %w", options.TemplateVMID, err)
	}
	if template.VirtualMachineConfig == nil || template.VirtualMachineConfig.Template != 1 {
		return nil, fmt.Errorf("VM %d is not a template", options.TemplateVMID)
	}
	if expected := strings.TrimSpace(options.TemplateName); expected != "" && template.Name != expected && template.VirtualMachineConfig.Name != expected {
		return nil, fmt.Errorf("VM template %d name %q does not match configured name %q", options.TemplateVMID, template.Name, expected)
	}

	newID, err := api.Cluster.NextID(api.bg)
	if err != nil {
		return nil, fmt.Errorf("allocate VMID: %w", err)
	}
	full := uint8(0)
	if options.Full {
		full = 1
	}
	cloneOptions := &proxmox.VirtualMachineCloneOptions{
		NewID:   newID,
		Full:    full,
		Name:    strings.TrimSpace(options.Name),
		Storage: strings.TrimSpace(options.StoragePool),
		Target:  strings.TrimSpace(options.TargetNode),
	}
	_, task, err := template.Clone(api.bg, cloneOptions)
	if err != nil {
		return nil, fmt.Errorf("clone VM template %d: %w", options.TemplateVMID, err)
	}
	if err = waitVMTask(api.bg, task, defaultVMTaskTimeout); err != nil {
		return nil, fmt.Errorf("clone VM template %d: %w", options.TemplateVMID, err)
	}

	vm, err := api.VirtualMachine(newID)
	if err != nil {
		return nil, fmt.Errorf("load cloned VM %d: %w", newID, err)
	}
	if err = api.RespecVirtualMachine(vm, options.Cores, options.MemoryMB, options.BootDisk, options.DiskSizeGB); err != nil {
		return vm, err
	}
	return vm, nil
}

func (api *ProxmoxAPI) RespecVirtualMachine(vm *proxmox.VirtualMachine, cores, memoryMB int, bootDisk string, diskSizeGB int) error {
	if vm == nil {
		return fmt.Errorf("VM is nil")
	}
	var configOptions []proxmox.VirtualMachineOption
	if cores > 0 {
		configOptions = append(configOptions, proxmox.VirtualMachineOption{Name: "cores", Value: cores})
	}
	if memoryMB > 0 {
		configOptions = append(configOptions, proxmox.VirtualMachineOption{Name: "memory", Value: memoryMB})
	}
	if len(configOptions) > 0 {
		task, err := vm.Config(api.bg, configOptions...)
		if err != nil {
			return fmt.Errorf("configure VM %d resources: %w", vm.VMID, err)
		}
		if err = waitVMTask(api.bg, task, defaultVMTaskTimeout); err != nil {
			return fmt.Errorf("configure VM %d resources: %w", vm.VMID, err)
		}
	}

	if diskSizeGB > 0 {
		currentGB, err := VirtualMachineDiskSizeGB(vm, bootDisk)
		if err != nil {
			return fmt.Errorf("inspect VM %d boot disk: %w", vm.VMID, err)
		}
		if float64(diskSizeGB) < currentGB {
			return fmt.Errorf("cannot shrink VM %d disk %s from %.2f GB to %d GB", vm.VMID, bootDisk, currentGB, diskSizeGB)
		}
		if float64(diskSizeGB) > currentGB {
			task, resizeErr := vm.ResizeDisk(api.bg, bootDisk, fmt.Sprintf("%dG", diskSizeGB))
			if resizeErr != nil {
				return fmt.Errorf("resize VM %d disk %s: %w", vm.VMID, bootDisk, resizeErr)
			}
			if resizeErr = waitVMTask(api.bg, task, defaultVMTaskTimeout); resizeErr != nil {
				return fmt.Errorf("resize VM %d disk %s: %w", vm.VMID, bootDisk, resizeErr)
			}
		}
	}
	return nil
}

func VirtualMachineDiskSizeGB(vm *proxmox.VirtualMachine, disk string) (float64, error) {
	if vm == nil || vm.VirtualMachineConfig == nil {
		return 0, fmt.Errorf("VM configuration is unavailable")
	}
	disk = strings.ToLower(strings.TrimSpace(disk))
	value, ok := vm.VirtualMachineConfig.MergeDisks()[disk]
	if !ok {
		return 0, fmt.Errorf("unsupported boot disk %q", disk)
	}
	match := diskSizePattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return 0, fmt.Errorf("disk %q has no parseable size in %q", disk, value)
	}
	size, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse disk size %q: %w", match[1], err)
	}
	multiplier := map[string]float64{"K": 1.0 / (1024 * 1024), "M": 1.0 / 1024, "G": 1, "T": 1024}[match[2]]
	return size * multiplier, nil
}

func (api *ProxmoxAPI) StartVirtualMachine(vm *proxmox.VirtualMachine) error {
	if vm == nil {
		return fmt.Errorf("VM is nil")
	}
	if err := vm.Ping(api.bg); err == nil && vm.IsRunning() {
		return nil
	}
	task, err := vm.Start(api.bg)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already running") {
			return nil
		}
		return fmt.Errorf("start VM %d: %w", vm.VMID, err)
	}
	return waitVMTask(api.bg, task, 5*time.Minute)
}

func (api *ProxmoxAPI) StopVirtualMachine(vm *proxmox.VirtualMachine) error {
	if vm == nil {
		return fmt.Errorf("VM is nil")
	}
	if err := vm.Ping(api.bg); err == nil && vm.IsStopped() {
		return nil
	}
	task, err := vm.Stop(api.bg)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not running") {
			return nil
		}
		return fmt.Errorf("stop VM %d: %w", vm.VMID, err)
	}
	return waitVMTask(api.bg, task, 5*time.Minute)
}

func (api *ProxmoxAPI) DeleteVirtualMachine(vm *proxmox.VirtualMachine) error {
	if vm == nil {
		return fmt.Errorf("VM is nil")
	}
	task, err := vm.Delete(api.bg)
	if err != nil {
		return fmt.Errorf("delete VM %d: %w", vm.VMID, err)
	}
	return waitVMTask(api.bg, task, 10*time.Minute)
}

func (api *ProxmoxAPI) WaitForVirtualMachineAgent(vm *proxmox.VirtualMachine, timeout, stableFor time.Duration) error {
	if vm == nil {
		return fmt.Errorf("VM is nil")
	}
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	if stableFor < 0 {
		stableFor = 0
	}
	ctx, cancel := context.WithTimeout(api.bg, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var readySince time.Time
	for {
		_, err := vm.AgentOsInfo(ctx)
		if err == nil {
			if readySince.IsZero() {
				readySince = time.Now()
			}
			if time.Since(readySince) >= stableFor {
				return nil
			}
		} else {
			readySince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for VM %d guest agent: %w", vm.VMID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (api *ProxmoxAPI) ExecuteVirtualMachineCommand(vm *proxmox.VirtualMachine, command []string, input string, timeout time.Duration) (VMCommandResult, error) {
	if vm == nil {
		return VMCommandResult{}, fmt.Errorf("VM is nil")
	}
	if len(command) == 0 {
		return VMCommandResult{}, fmt.Errorf("command is empty")
	}
	if timeout <= 0 {
		timeout = defaultVMCommandTimeout
	}
	pid, err := vm.AgentExec(api.bg, command, input)
	if err != nil {
		return VMCommandResult{}, fmt.Errorf("execute command on VM %d: %w", vm.VMID, err)
	}
	status, err := vm.WaitForAgentExecExit(api.bg, pid, int(math.Ceil(timeout.Seconds())))
	if err != nil {
		return VMCommandResult{}, fmt.Errorf("wait for command on VM %d: %w", vm.VMID, err)
	}
	result := VMCommandResult{
		Stdout:    status.OutData,
		Stderr:    status.ErrData,
		ExitCode:  status.ExitCode,
		Truncated: status.ErrTruncated || status.OutTruncated != "",
	}
	if status.Signal {
		return result, fmt.Errorf("command on VM %d exited due to a signal", vm.VMID)
	}
	return result, nil
}

func waitVMTask(ctx context.Context, task *proxmox.Task, timeout time.Duration) error {
	if task == nil {
		return nil
	}
	if err := task.Wait(ctx, time.Second, timeout); err != nil {
		return err
	}
	if !task.IsSuccessful {
		exitStatus := strings.TrimSpace(task.ExitStatus)
		if exitStatus == "" {
			exitStatus = "unknown failure"
		}
		return errors.New(exitStatus)
	}
	return nil
}
