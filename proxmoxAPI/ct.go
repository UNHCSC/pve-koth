package proxmoxAPI

import (
	"fmt"
	"slices"
	"sync"
	"time"

	goProxmox "github.com/luthermonson/go-proxmox"
)

func (api *ProxmoxAPI) NodeForContainer(ctID int) (node *goProxmox.Node, err error) {
	for _, node = range api.Nodes {
		if _, err = node.Container(api.bg, ctID); err == nil {
			return
		}
	}

	err = goProxmox.ErrNotFound
	return
}

func (api *ProxmoxAPI) Container(ctID int) (ct *goProxmox.Container, err error) {
	var node *goProxmox.Node
	if node, err = api.NodeForContainer(ctID); err != nil {
		return
	}

	ct, err = node.Container(api.bg, ctID)
	return
}

func (api *ProxmoxAPI) StartContainer(ct *goProxmox.Container) (err error) {
	var task *goProxmox.Task
	if task, err = ct.Start(api.bg); err != nil {
		err = fmt.Errorf("failed to start container: %w", err)
	} else if err = task.Wait(api.bg, time.Second, time.Minute*3); err != nil {
		err = fmt.Errorf("failed to wait for container start task: %w", err)
	}

	return
}

func (api *ProxmoxAPI) StopContainer(ct *goProxmox.Container) (err error) {
	var task *goProxmox.Task
	if task, err = ct.Stop(api.bg); err != nil {
		err = fmt.Errorf("failed to stop container: %w", err)
	} else if err = task.Wait(api.bg, time.Second, time.Minute*3); err != nil {
		err = fmt.Errorf("failed to wait for container stop task: %w", err)
	}

	return
}

func (api *ProxmoxAPI) DeleteContainer(ct *goProxmox.Container) (err error) {
	var task *goProxmox.Task
	if task, err = ct.Delete(api.bg); err != nil {
		err = fmt.Errorf("failed to delete container: %w", err)
	} else if err = task.Wait(api.bg, time.Second, time.Minute*3); err != nil {
		err = fmt.Errorf("failed to wait for container delete task: %w", err)
	}

	return
}

func (api *ProxmoxAPI) CreateTemplate(ct *goProxmox.Container) (err error) {
	err = ct.Template(api.bg)
	return
}

func (api *ProxmoxAPI) CloneTemplate(ct *goProxmox.Container, hostname string) (newCT *goProxmox.Container, err error) {
	var (
		newID int
		task  *goProxmox.Task
	)

	if newID, task, err = ct.Clone(api.bg, &goProxmox.ContainerCloneOptions{
		Full:     1,
		Hostname: hostname,
	}); err != nil {
		err = fmt.Errorf("failed to clone container: %w", err)
		return
	} else if err = task.Wait(api.bg, time.Second, time.Minute*10); err != nil {
		err = fmt.Errorf("failed to wait for container clone task: %w", err)
		return
	}

	if newCT, err = api.Container(newID); err != nil {
		err = fmt.Errorf("failed to get new container after clone: %w", err)
		return
	}

	return
}

func (api *ProxmoxAPI) ChangeContainerNetworking(ct *goProxmox.Container, gatewayIPv4, IPv4Address string, CIDRBlock int) (err error) {
	var task *goProxmox.Task

	if task, err = ct.Config(api.bg, goProxmox.ContainerOption{
		Name:  "net0",
		Value: fmt.Sprintf("name=eth0,bridge=vmbr0,firewall=1,gw=%s,ip=%s/%d", gatewayIPv4, IPv4Address, CIDRBlock),
	}); err != nil {
		err = fmt.Errorf("failed to change container networking: %w", err)
	} else if task != nil {
		if err = task.Wait(api.bg, time.Second, time.Minute*3); err != nil {
			err = fmt.Errorf("failed to wait for container config task: %w", err)
		}
	}

	return
}

func (api *ProxmoxAPI) GetContainers(ids []int) (containers []*goProxmox.Container, err error) {
	containers = make([]*goProxmox.Container, 0, len(ids))

	for _, node := range api.Nodes {
		var nodeContainers []*goProxmox.Container
		if nodeContainers, err = node.Containers(api.bg); err != nil {
			err = fmt.Errorf("failed to get containers for node %s: %w", node.Name, err)
			return
		}

		for _, ct := range nodeContainers {
			if slices.Contains(ids, int(ct.VMID)) {
				containers = append(containers, ct)
			}
		}
	}

	return
}

func (api *ProxmoxAPI) bulkJob(tasks []*goProxmox.Task, bucketSize int) (err error) {
	var buckets [][]*goProxmox.Task

	for i := 0; i < len(tasks); i += bucketSize {
		buckets = append(buckets, tasks[i:min(i+bucketSize, len(tasks))])
	}

	for _, bucket := range buckets {
		var wg sync.WaitGroup

		for _, task := range bucket {
			wg.Add(1)

			go func(t *goProxmox.Task) {
				defer wg.Done()

				if e := t.Wait(api.bg, time.Second, time.Minute*5); e != nil {
					err = fmt.Errorf("failed to wait for task %s: %w", t.UPID, e)
				}
			}(task)
		}

		wg.Wait()
	}

	return
}

func (api *ProxmoxAPI) BulkStart(ids []int) (err error) {
	var tasks []*goProxmox.Task

	var containers []*goProxmox.Container
	if containers, err = api.GetContainers(ids); err != nil {
		return
	}

	for _, ct := range containers {
		var task *goProxmox.Task
		if task, err = ct.Start(api.bg); err != nil {
			err = fmt.Errorf("failed to start container %d: %w", ct.VMID, err)
			return
		}

		tasks = append(tasks, task)
	}

	err = api.bulkJob(tasks, 5)
	return
}

func (api *ProxmoxAPI) BulkStop(ids []int) (err error) {
	var tasks []*goProxmox.Task

	var containers []*goProxmox.Container
	if containers, err = api.GetContainers(ids); err != nil {
		return
	}

	for _, ct := range containers {
		var task *goProxmox.Task
		if task, err = ct.Stop(api.bg); err != nil {
			err = fmt.Errorf("failed to stop container %d: %w", ct.VMID, err)
			return
		}

		tasks = append(tasks, task)
	}

	err = api.bulkJob(tasks, 5)
	return
}

func (api *ProxmoxAPI) BulkDelete(ids []int) (err error) {
	var tasks []*goProxmox.Task

	var containers []*goProxmox.Container
	if containers, err = api.GetContainers(ids); err != nil {
		return
	}

	for _, ct := range containers {
		var task *goProxmox.Task
		if task, err = ct.Delete(api.bg); err != nil {
			err = fmt.Errorf("failed to delete container %d: %w", ct.VMID, err)
			return
		}

		tasks = append(tasks, task)
	}

	err = api.bulkJob(tasks, 5)
	return
}
