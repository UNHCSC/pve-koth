package proxmoxAPI

import (
	"fmt"
	"time"

	goProxmox "github.com/luthermonson/go-proxmox"
)

func (api *ProxmoxAPI) CreateContainer(node *goProxmox.Node, conf *ContainerCreateOptions) (ct *goProxmox.Container, ctID int, err error) {

	if ctID, err = api.Cluster.NextID(api.bg); err != nil {
		err = fmt.Errorf("failed to get next container ID: %w", err)
		return
	}

	var task *goProxmox.Task
	if task, err = node.NewContainer(api.bg, ctID, conf.GoProxmoxOptions()...); err != nil {
		err = fmt.Errorf("failed to create container: %w", err)
		return
	}

	if err = task.Wait(api.bg, time.Second, time.Minute*5); err != nil {
		err = fmt.Errorf("failed to wait for container creation task: %w", err)
		return
	}

	if ct, err = node.Container(api.bg, ctID); err != nil {
		err = fmt.Errorf("failed to get created container info: %w", err)
		return
	}

	return
}
