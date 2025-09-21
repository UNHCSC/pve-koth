package tests

import (
	"testing"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/luthermonson/go-proxmox"
)

func TestCTLifecycle(t *testing.T) {
	var err error

	if err = config.InitEnv("../.env"); err != nil {
		t.Fatalf("failed to initialize environment: %v\n", err)
		return
	}

	var api *proxmoxAPI.ProxmoxAPI
	if api, err = proxmoxAPI.InitProxmox(); err != nil {
		t.Fatalf("failed to initialize Proxmox API: %v\n", err)
		return
	}

	if len(api.Nodes) == 0 {
		t.Fatalf("no online Proxmox nodes found")
		return
	}

	var node = api.Nodes[0]

	var conf = &proxmoxAPI.ContainerCreateOptions{
		TemplatePath:     "local:vztmpl/ubuntu-25.04-standard_25.04-1.1_amd64.tar.zst",
		StoragePool:      "Storage",
		Hostname:         "koth-test-ct",
		RootPassword:     "password",
		RootSSHPublicKey: "",
		StorageSizeGB:    8,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      "10.0.0.1",
		IPv4Address:      "10.255.255.88",
		CIDRBlock:        8,
		NameServer:       "10.0.0.2",
		SearchDomain:     "cyber.lab",
	}

	var ct *proxmox.Container
	var ctID int

	if ct, ctID, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	t.Logf("created container with ID %d\n", ctID)

	if err = api.StartContainer(ct); err != nil {
		t.Errorf("failed to start container: %v\n", err)
	} else {
		t.Logf("started container with ID %d\n", ctID)
	}

	if err = api.StopContainer(ct); err != nil {
		t.Errorf("failed to stop container: %v\n", err)
	} else {
		t.Logf("stopped container with ID %d\n", ctID)
	}

	if err = api.DeleteContainer(ct); err != nil {
		t.Errorf("failed to delete container: %v\n", err)
	} else {
		t.Logf("deleted container with ID %d\n", ctID)
	}
}
