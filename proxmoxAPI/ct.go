package proxmoxAPI

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/luthermonson/go-proxmox"
)

func (api *ProxmoxAPI) NodeForContainer(ctID int) (node *proxmox.Node, err error) {
	for _, node = range api.Nodes {
		if _, err = node.Container(api.bg, ctID); err == nil {
			return
		}
	}

	err = proxmox.ErrNotFound
	return
}

func (api *ProxmoxAPI) Container(ctID int) (ct *proxmox.Container, err error) {
	var node *proxmox.Node
	if node, err = api.NodeForContainer(ctID); err != nil {
		return
	}

	ct, err = node.Container(api.bg, ctID)
	return
}

func (api *ProxmoxAPI) StartContainer(ct *proxmox.Container) (err error) {
	var task *proxmox.Task
	if task, err = ct.Start(api.bg); err == nil {
		err = task.Wait(api.bg, time.Second, time.Minute*3)
	}

	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already running") {
		err = nil
	}

	return
}

func (api *ProxmoxAPI) StopContainer(ct *proxmox.Container) (err error) {
	var task *proxmox.Task
	if task, err = ct.Stop(api.bg); err == nil {
		err = task.Wait(api.bg, time.Second, time.Minute*3)
	}

	if err != nil && strings.Contains(strings.ToLower(err.Error()), "not running") {
		err = nil
	}

	return
}

func (api *ProxmoxAPI) DeleteContainer(ct *proxmox.Container) (err error) {
	var task *proxmox.Task
	if task, err = ct.Delete(api.bg); err == nil {
		err = task.Wait(api.bg, time.Second, time.Minute*3)
	}

	return
}

func (api *ProxmoxAPI) CreateTemplate(ct *proxmox.Container) (err error) {
	err = ct.Template(api.bg)
	return
}

func (api *ProxmoxAPI) CloneTemplate(ct *proxmox.Container, hostname string) (newCT *proxmox.Container, err error) {
	var (
		newID int
		task  *proxmox.Task
	)

	if newID, task, err = ct.Clone(api.bg, &proxmox.ContainerCloneOptions{
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

func (api *ProxmoxAPI) ChangeContainerNetworking(ct *proxmox.Container, gatewayIPv4, IPv4Address string, CIDRBlock int) (err error) {
	var task *proxmox.Task

	if task, err = ct.Config(api.bg, proxmox.ContainerOption{
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

func (api *ProxmoxAPI) GetContainers(ids []int) (containers []*proxmox.Container, err error) {
	containers = make([]*proxmox.Container, 0, len(ids))

	for _, node := range api.Nodes {
		var nodeContainers []*proxmox.Container
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

func (api *ProxmoxAPI) bulkJob(tasks []*proxmox.Task, bucketSize int) (err error) {
	var buckets [][]*proxmox.Task

	for i := 0; i < len(tasks); i += bucketSize {
		buckets = append(buckets, tasks[i:min(i+bucketSize, len(tasks))])
	}

	for _, bucket := range buckets {
		var wg sync.WaitGroup

		for _, task := range bucket {
			if task == nil {
				continue
			}

			wg.Add(1)

			go func(t *proxmox.Task) {
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
	var tasks []*proxmox.Task

	var containers []*proxmox.Container
	if containers, err = api.GetContainers(ids); err != nil {
		return
	}

	for _, ct := range containers {
		var task *proxmox.Task
		if task, err = ct.Start(api.bg); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already running") {
			err = fmt.Errorf("failed to start container %d: %w", ct.VMID, err)
			return
		}

		tasks = append(tasks, task)
	}

	err = api.bulkJob(tasks, 5)
	return
}

func (api *ProxmoxAPI) BulkStop(ids []int) (err error) {
	var tasks []*proxmox.Task

	var containers []*proxmox.Container
	if containers, err = api.GetContainers(ids); err != nil {
		return
	}

	for _, ct := range containers {
		var task *proxmox.Task
		if task, err = ct.Stop(api.bg); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not running") {
			err = fmt.Errorf("failed to stop container %d: %w", ct.VMID, err)
			return
		}

		tasks = append(tasks, task)
	}

	err = api.bulkJob(tasks, 5)
	return
}

func (api *ProxmoxAPI) BulkDelete(ids []int) (err error) {
	var tasks []*proxmox.Task

	var containers []*proxmox.Container
	if containers, err = api.GetContainers(ids); err != nil {
		return
	}

	for _, ct := range containers {
		var task *proxmox.Task
		if task, err = ct.Delete(api.bg); err != nil {
			err = fmt.Errorf("failed to delete container %d: %w", ct.VMID, err)
			return
		}

		tasks = append(tasks, task)
	}

	err = api.bulkJob(tasks, 5)
	return
}

func (api *ProxmoxAPI) CTActionWithRetries(action func(ct *proxmox.Container) error, ct *proxmox.Container, numRetries int) (err error) {
	for i := range numRetries + 1 {
		if err = action(ct); err == nil {
			if i > 0 {
				// fmt.Printf("Action on container %d succeeded after %d retries.\n", ct.VMID, i-1)
			}

			return
		}

		// fmt.Printf("Action on container %d failed: %v. Retrying (%d/%d)...\n", ct.VMID, err, i+1, numRetries)
		time.Sleep(time.Second * (time.Duration(i) + 1))
	}

	return
}

func (api *ProxmoxAPI) BulkCTActionWithRetries(action func(ids []int) (err error), ids []int, numRetries int) (err error) {
	for i := range numRetries + 1 {
		if err = action(ids); err == nil {
			if i > 0 {
				// fmt.Printf("Action on container %d succeeded after %d retries.\n", ct.VMID, i-1)
			}

			return
		}

		// fmt.Printf("Action on container %d failed: %v. Retrying (%d/%d)...\n", ct.VMID, err, i+1, numRetries)
		time.Sleep(time.Second * (time.Duration(i) + 1))
	}

	return
}

func (api *ProxmoxAPI) ListContainers(node *proxmox.Node) (containers []*proxmox.Container, err error) {
	containers, err = node.Containers(api.bg)
	return
}

func (api *ProxmoxAPI) RawExecute(ct *proxmox.Container, username, password, command string) (stdout string, stderr string, err error) {
	var term *proxmox.Term

	if term, err = ct.TermProxy(api.bg); err != nil {
		err = fmt.Errorf("failed to create terminal proxy: %w", err)
		return
	}

	if term.User = determineTerminalUser(term.User, username, api.tokenUser); term.User == "" {
		err = fmt.Errorf("terminal websocket response did not include a user")
		return
	}

	var (
		send, recv chan []byte
		errs       chan error
		close      func() error
	)

	if send, recv, errs, close, err = ct.TermWebSocket(term); err != nil {
		err = fmt.Errorf("failed to create terminal websocket: %w", err)
		return
	}

	defer close()

	authCmd := fmt.Sprintf("echo '%s' | sudo -S -u root -i bash -c \"%s\"\n", password, command)

	send <- []byte(authCmd)

	var outputBuilder strings.Builder
	var errorBuilder strings.Builder

	done := make(chan struct{})

	go func() {
		for {
			select {
			case msg := <-recv:
				outputBuilder.Write(msg)
				fmt.Print(string(msg))
			case err := <-errs:
				if err != nil {
					errorBuilder.WriteString(err.Error())
				}
			case <-done:
				return
			}
		}
	}()

	// Wait for a short period to allow command execution
	time.Sleep(3 * time.Second)
	close()

	stdout = outputBuilder.String()
	stderr = errorBuilder.String()

	return
}

func determineTerminalUser(existing, desired, tokenUser string) string {
	if existing != "" {
		return existing
	}

	user := tokenUser
	if user == "" {
		user = desired
	}

	if user == "" {
		return ""
	}

	if sep := strings.Index(user, "!"); sep > 0 {
		user = user[:sep]
	}

	if !strings.Contains(user, "@") {
		user = fmt.Sprintf("%s@pam", user)
	}

	return user
}

func (api *ProxmoxAPI) RawExecuteWithRetries(ct *proxmox.Container, username, password, command string, numRetries int) (stdout string, stderr string, err error) {
	for i := range numRetries + 1 {
		if stdout, stderr, err = api.RawExecute(ct, username, password, command); err == nil {
			if i > 0 {
				// fmt.Printf("Raw execute on container %d succeeded after %d retries.\n", ct.VMID, i-1)
			}

			return
		}

		// fmt.Printf("Raw execute on container %d failed: %v. Retrying (%d/%d)...\n", ct.VMID, err, i+1, numRetries)
		time.Sleep(time.Second * (time.Duration(i) + 1))
	}

	return
}
