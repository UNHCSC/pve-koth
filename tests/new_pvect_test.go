package tests

import (
	"testing"

	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/luthermonson/go-proxmox"
)

func TestNewLifecycle(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err     error
		api     *proxmoxAPI.ProxmoxAPI = initProxmox(t)
		node    *proxmox.Node          = api.Nodes[0]
		cleanup func() (err error)
		ips     []string
		conf    *proxmoxAPI.ContainerCreateOptions
	)

	if ips, err = getIPs(t, 1); err != nil {
		t.Fatalf("failed to get open IP: %v", err)
	}

	conf = provisionConf(ips[0], "koth-lifecycle-test-ct", "")

	_, cleanup = container(t, api, node, conf)
	cleanup()
}

func TestSimpleCTScriptRunner(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err      error
		api      *proxmoxAPI.ProxmoxAPI = initProxmox(t)
		node     *proxmox.Node          = api.Nodes[0]
		result   *proxmoxAPI.ProxmoxAPICreateResult
		cleanup  func() (err error)
		ips      []string
		conf     *proxmoxAPI.ContainerCreateOptions
		reqToken func() (token string)
		revToken func(token string)
	)

	if ips, err = getIPs(t, 1); err != nil {
		t.Fatalf("failed to get open IP: %v", err)
	}

	reqToken, revToken = initWebServer()

	// Creation
	t.Logf("Creating container on node %s", node.Name)
	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	t.Logf("Created container with ID %d", result.CTID)

	// Power on
	t.Logf("Starting container with ID %d", result.CTID)
	if err = api.StartContainer(result.Container); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	t.Logf("Started container with ID %d", result.CTID)

}
