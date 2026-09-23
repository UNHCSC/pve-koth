package koth

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/UNHCSC/pve-koth/db"
	"github.com/z46-dev/go-logger"
)

const containerStatusRefreshInterval = 30 * time.Second

var (
	containerLog         *logger.Logger = logger.NewLogger().SetPrefix("[CTRS]", logger.BoldPurple).IncludeTimestamp()
	containerMonitorOnce sync.Once
)

// ContainerRuntime describes the current state of a container reported by Proxmox.
type ContainerRuntime struct {
	ID     int64
	Name   string
	Status string
	Node   string
}

// StartContainerStatusMonitor begins a ticker that keeps container power states fresh in the DB.
func StartContainerStatusMonitor() {
	containerMonitorOnce.Do(func() {
		go containerStatusLoop()
	})
}

// RefreshContainerStatuses updates the stored state for the provided IDs (or all containers when ids is empty).
func RefreshContainerStatuses(ids []int64) error {
	return updateContainerStatuses(ids)
}

func containerStatusLoop() {
	containerLog.Basicf("container monitor started (interval %s)\n", containerStatusRefreshInterval)
	if err := updateContainerStatuses(nil); err != nil {
		containerLog.Errorf("initial container refresh failed: %v\n", err)
	}

	ticker := time.NewTicker(containerStatusRefreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := updateContainerStatuses(nil); err != nil {
			containerLog.Errorf("container refresh failed: %v\n", err)
		}
	}
}

func updateContainerStatuses(targetIDs []int64) (err error) {
	var ids []int64
	if ids, err = resolveContainerIDs(targetIDs); err != nil {
		return fmt.Errorf("resolve containers: %w", err)
	}

	if len(ids) == 0 {
		return nil
	}

	var runtime map[int64]ContainerRuntime
	if runtime, err = ContainerRuntimeSnapshot(ids); err != nil {
		return fmt.Errorf("runtime snapshot: %w", err)
	}

	var records map[int64]*db.Container
	if records, err = loadContainerRecords(ids); err != nil {
		return fmt.Errorf("load container records: %w", err)
	}

	var now = time.Now()
	for _, id := range ids {
		record := records[id]
		if record == nil {
			continue
		}

		state := "unknown"
		if rt, ok := runtime[id]; ok {
			if strings.TrimSpace(rt.Status) != "" {
				state = strings.ToLower(strings.TrimSpace(rt.Status))
			}
		}

		record.Status = state
		record.LastUpdated = now
		if updateErr := db.Containers.Update(record); updateErr != nil {
			containerLog.Errorf("failed to update container %d status: %v\n", record.PVEID, updateErr)
		}
	}

	return nil
}

// ContainerRuntimeSnapshot fetches runtime data for the provided IDs.
func ContainerRuntimeSnapshot(ids []int64) (map[int64]ContainerRuntime, error) {
	normalized := normalizeContainerIDs(ids)
	result := make(map[int64]ContainerRuntime, len(normalized))
	if len(normalized) == 0 {
		return result, nil
	}

	if api == nil {
		return nil, fmt.Errorf("proxmox API is not initialized")
	}

	records, err := loadContainerRecords(normalized)
	if err != nil {
		return nil, err
	}
	for _, id := range normalized {
		record := records[id]
		if record != nil && record.GuestKind == db.GuestKindQEMU {
			vm, vmErr := api.VirtualMachine(int(id))
			if vmErr == nil {
				name := strings.TrimSpace(vm.Name)
				if name == "" && vm.VirtualMachineConfig != nil {
					name = strings.TrimSpace(vm.VirtualMachineConfig.Name)
				}
				result[id] = ContainerRuntime{ID: id, Name: name, Status: strings.ToLower(strings.TrimSpace(vm.Status)), Node: strings.TrimSpace(vm.Node)}
			}
			continue
		}
		ct, ctErr := api.Container(int(id))
		if ctErr != nil {
			continue
		}
		name := strings.TrimSpace(ct.Name)
		if name == "" && ct.ContainerConfig != nil {
			name = strings.TrimSpace(ct.ContainerConfig.Hostname)
		}
		result[id] = ContainerRuntime{ID: id, Name: name, Status: strings.ToLower(strings.TrimSpace(ct.Status)), Node: strings.TrimSpace(ct.Node)}
	}

	for _, id := range normalized {
		if _, exists := result[id]; !exists {
			result[id] = ContainerRuntime{ID: id, Status: "unknown"}
		}
	}

	return result, nil
}

// BulkStartContainers powers on the provided CTIDs using the Proxmox bulk APIs.
func BulkStartContainers(ids []int64) error {
	return bulkGuestAction(ids, true)
}

// BulkStopContainers powers off the provided CTIDs using the Proxmox bulk APIs.
func BulkStopContainers(ids []int64) error {
	return bulkGuestAction(ids, false)
}

func bulkGuestAction(ids []int64, start bool) error {
	normalized := normalizeContainerIDs(ids)
	if len(normalized) == 0 {
		return fmt.Errorf("no container IDs supplied")
	}
	for _, id := range normalized {
		record, err := db.Containers.Select(id)
		if err != nil || record == nil {
			return fmt.Errorf("load guest %d: %w", id, err)
		}
		if record.GuestKind == db.GuestKindQEMU {
			vm, vmErr := api.VirtualMachine(int(id))
			if vmErr != nil {
				return vmErr
			}
			if start {
				err = api.StartVirtualMachine(vm)
				if err == nil {
					err = api.WaitForVirtualMachineAgent(vm, 5*time.Minute, 30*time.Second)
				}
			} else {
				err = api.StopVirtualMachine(vm)
			}
		} else {
			ct, ctErr := api.Container(int(id))
			if ctErr != nil {
				return ctErr
			}
			if start {
				err = api.StartContainer(ct)
			} else {
				err = api.StopContainer(ct)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func bulkContainerAction(ids []int64, action func(ids []int) error) error {
	normalized := normalizeContainerIDs(ids)
	if len(normalized) == 0 {
		return fmt.Errorf("no container IDs supplied")
	}

	if api == nil {
		return fmt.Errorf("proxmox API is not initialized")
	}

	intIDs, err := convertContainerIDs(normalized)
	if err != nil {
		return err
	}

	return api.BulkCTActionWithRetries(action, intIDs, 2)
}

func resolveContainerIDs(targetIDs []int64) (ids []int64, err error) {
	if len(targetIDs) > 0 {
		return normalizeContainerIDs(targetIDs), nil
	}

	var comps []*db.Competition
	if comps, err = db.Competitions.SelectAll(); err != nil {
		return nil, err
	}

	for _, comp := range comps {
		if comp == nil {
			continue
		}
		ids = append(ids, comp.ContainerIDs...)
	}

	return normalizeContainerIDs(ids), nil
}

func loadContainerRecords(ids []int64) (map[int64]*db.Container, error) {
	records := make(map[int64]*db.Container, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}

		record, err := db.Containers.Select(id)
		if err != nil {
			return nil, err
		}
		if record != nil {
			records[id] = record
		}
	}

	return records, nil
}

func normalizeContainerIDs(ids []int64) []int64 {
	var result []int64
	if len(ids) == 0 {
		return result
	}

	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}

	return result
}

func convertContainerIDs(ids []int64) ([]int, error) {
	intIDs := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if id > math.MaxInt {
			return nil, fmt.Errorf("container ID %d exceeds max int", id)
		}
		intIDs = append(intIDs, int(id))
	}
	return intIDs, nil
}
