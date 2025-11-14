package tests

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/sshcomm"
	"github.com/gofiber/fiber/v2"
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
		TemplatePath:     "isos-ct_templates:vztmpl/ubuntu-25.04-standard_25.04-1.1_amd64.tar.zst",
		StoragePool:      "laas",
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

	var result *proxmoxAPI.ProxmoxAPICreateResult

	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	t.Logf("created container with ID %d\n", result.CTID)

	if err = api.StartContainer(result.Container); err != nil {
		t.Errorf("failed to start container: %v\n", err)
	} else {
		t.Logf("started container with ID %d\n", result.CTID)
	}

	if err = api.StopContainer(result.Container); err != nil {
		t.Errorf("failed to stop container: %v\n", err)
	} else {
		t.Logf("stopped container with ID %d\n", result.CTID)
	}

	if err = api.DeleteContainer(result.Container); err != nil {
		t.Errorf("failed to delete container: %v\n", err)
	} else {
		t.Logf("deleted container with ID %d\n", result.CTID)
	}
}

func helperSpinUpFileServer() {
	var app = fiber.New(fiber.Config{})

	var middleware = func(c *fiber.Ctx) error {
		fmt.Printf("Received request for %s\n", c.Path())

		if c.Cookies("Authorization", "") != "test_token" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		return c.Next()
	}

	app.Use(middleware)
	app.Static("/", "./public")

	app.Listen(":8080")
}

// This test should be run without a timeout as this can take a while
// go test -timeout=9999s -v -run ^TestCreateAndSetUpContainer$ github.com/UNHCSC/pve-koth/tests
func TestCreateAndSetUpContainer(t *testing.T) {
	// Skip if not run with long timeout
	if testing.Short() {
		t.Skip("skipping test in short mode.")
		return
	}

	const CT_IP = "10.255.255.89"
	var (
		err                            error
		exitCode                       int
		pubKey, privKey, commandOutput string
		api                            *proxmoxAPI.ProxmoxAPI
		result                         *proxmoxAPI.ProxmoxAPICreateResult
		conn                           *sshcomm.SSHConnection
	)

	defer func() {
		t.Log("---- Beginning Cleanup Phase ----")
		t.Log("Cleaning up SSH keys...")
		if err = os.RemoveAll("tmpssh"); err != nil {
			t.Logf("failed to remove temporary SSH key directory: %v\n", err)
		}

		if api != nil && result.Container != nil {
			t.Log("Cleaning up container...")
			if err = api.StopContainer(result.Container); err != nil {
				t.Logf("failed to stop container: %v\n", err)
			}

			if err = api.DeleteContainer(result.Container); err != nil {
				t.Logf("failed to delete container: %v\n", err)
			}
		}

		if conn != nil {
			t.Log("Closing SSH connection...")
			conn.Close()
		}
	}()

	t.Log("---- Beginning Setup Phase ----")
	t.Log("Initializing env...")
	if err = config.InitEnv("../.env"); err != nil {
		t.Fatalf("failed to initialize environment: %v\n", err)
		return
	}

	t.Log("Creating SSH key pair...")
	if pubKey, privKey, err = sshcomm.CreateSSHKeyPair("tmpssh"); err != nil {
		t.Fatalf("failed to create SSH key pair: %v\n", err)
		return
	}

	t.Log("Initializing Proxmox API...")
	if api, err = proxmoxAPI.InitProxmox(); err != nil {
		t.Fatalf("failed to initialize Proxmox API: %v\n", err)
		return
	}

	t.Log("Creating container...")
	if result, err = api.CreateContainer(api.NextNode(), &proxmoxAPI.ContainerCreateOptions{
		TemplatePath:     "isos-ct_templates:vztmpl/ubuntu-25.04-standard_25.04-1.1_amd64.tar.zst",
		StoragePool:      "laas",
		Hostname:         "koth-test-ct",
		RootPassword:     "password",
		RootSSHPublicKey: pubKey,
		StorageSizeGB:    8,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      "10.0.0.1",
		IPv4Address:      CT_IP,
		CIDRBlock:        8,
		NameServer:       "10.0.0.2",
		SearchDomain:     "cyber.lab",
	}); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	t.Log("Starting container...")
	if err = api.StartContainer(result.Container); err != nil {
		t.Fatalf("failed to start container: %v\n", err)
		return
	}

	t.Log("Waiting for container to come online...")
	if err = sshcomm.WaitOnline(CT_IP); err != nil {
		t.Fatalf("container did not come online in time: %v\n", err)
		return
	}

	t.Log("Connecting via SSH...")
	if conn, err = sshcomm.Connect("root", CT_IP, 22, sshcomm.WithPrivateKey([]byte(privKey))); err != nil {
		t.Fatalf("failed to connect via SSH: %v\n", err)
		return
	}

	var envs = map[string]any{
		"KOTH_COMP_ID":               "testcomp",
		"KOTH_ACCESS_TOKEN":          "test_token",
		"KOTH_PUBLIC_FOLDER":         fmt.Sprintf("http://%s:%d/", sshcomm.MustLocalIP(), 8080),
		"KOTH_TEAM_ID":               "team1",
		"KOTH_HOSTNAME":              "koth-test-ct",
		"KOTH_IP":                    CT_IP,
		"KOTH_CONTAINER_IPS_grafana": CT_IP,
		"KOTH_CONTAINER_IPS":         CT_IP,
	}

	go helperSpinUpFileServer()

	t.Log("Sending global setup script...")
	if exitCode, commandOutput, err = conn.SendWithOutput(sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/setup_global.sh", sshcomm.MustLocalIP(), 8080), "test_token", envs)); err != nil {
		t.Fatalf("failed to send global setup script: %v\n", err)
		return
	} else {
		t.Logf("Global setup script exited with code %d\nOutput:\n%s\n", exitCode, commandOutput)
	}

	if err = conn.Reset(); err != nil {
		t.Fatalf("failed to reset SSH connection: %v\n", err)
		return
	}

	t.Log("Sending grafana setup script...")
	if exitCode, commandOutput, err = conn.SendWithOutput(sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/setup_grafana.sh", sshcomm.MustLocalIP(), 8080), "test_token", envs)); err != nil {
		t.Fatalf("failed to send grafana setup script: %v\n", err)
		return
	} else {
		t.Logf("Grafana setup script exited with code %d\nOutput:\n%s\n", exitCode, commandOutput)
	}

	if err = conn.Reset(); err != nil {
		t.Fatalf("failed to reset SSH connection: %v\n", err)
		return
	}

	t.Log("---- Container is now set up; Beginning to Scoring Phase ----")

	t.Log("Sending global scoring script...")
	if exitCode, commandOutput, err = conn.SendWithOutput(sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/score_global.sh", sshcomm.MustLocalIP(), 8080), "test_token", envs)); err != nil {
		t.Fatalf("failed to send global scoring script: %v\n", err)
		return
	} else {
		t.Logf("Global scoring script exited with code %d\nOutput:\n%s\n", exitCode, commandOutput)
	}

	if err = conn.Reset(); err != nil {
		t.Fatalf("failed to reset SSH connection: %v\n", err)
		return
	}

	t.Log("Sending grafana scoring script...")
	if exitCode, commandOutput, err = conn.SendWithOutput(sshcomm.LoadAndRunScript(fmt.Sprintf("http://%s:%d/score_grafana.sh", sshcomm.MustLocalIP(), 8080), "test_token", envs)); err != nil {
		t.Fatalf("failed to send grafana scoring script: %v\n", err)
		return
	} else {
		t.Logf("Grafana scoring script exited with code %d\nOutput:\n%s\n", exitCode, commandOutput)
	}
}

// This test should be run without a timeout as this can take a while
// go test -timeout=9999s -v -run ^TestCTTemplateClone$ github.com/UNHCSC/pve-koth/tests
func TestCTTemplateClone(t *testing.T) {
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
		TemplatePath:     "isos-ct_templates:vztmpl/ubuntu-25.04-standard_25.04-1.1_amd64.tar.zst",
		StoragePool:      "laas",
		Hostname:         "koth-test-ct",
		RootPassword:     "password",
		RootSSHPublicKey: "",
		StorageSizeGB:    8,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      "10.0.0.1",
		IPv4Address:      "10.224.0.2",
		CIDRBlock:        8,
		NameServer:       "10.0.0.2",
		SearchDomain:     "cyber.lab",
	}

	var result *proxmoxAPI.ProxmoxAPICreateResult

	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	t.Logf("created container with ID %d\n", result.CTID)

	if err = api.StartContainer(result.Container); err != nil {
		t.Errorf("failed to start container: %v\n", err)
	} else {
		t.Logf("started container with ID %d\n", result.CTID)
	}

	if err = api.StopContainer(result.Container); err != nil {
		t.Errorf("failed to stop container: %v\n", err)
	} else {
		t.Logf("stopped container with ID %d\n", result.CTID)
	}

	if err = api.CreateTemplate(result.Container); err != nil {
		t.Errorf("failed to create template from container: %v\n", err)
	} else {
		t.Logf("created template from container with ID %d\n", result.CTID)
	}

	var newCT *proxmox.Container
	if newCT, err = api.CloneTemplate(result.Container, "koth-test-clone"); err != nil {
		t.Errorf("failed to clone template: %v\n", err)
	} else {
		t.Logf("cloned template from container with ID %d to new container with ID %d\n", result.CTID, newCT.VMID)
	}

	if err = api.ChangeContainerNetworking(newCT, "10.0.0.1", "10.224.0.3", 8); err != nil {
		t.Errorf("failed to change networking of cloned container: %v\n", err)
	} else {
		t.Logf("changed networking of cloned container with ID %d\n", newCT.VMID)
	}

	if err = api.StartContainer(newCT); err != nil {
		t.Errorf("failed to start cloned container: %v\n", err)
	} else {
		t.Logf("started cloned container with ID %d\n", newCT.VMID)
	}

	if err = sshcomm.WaitOnline("10.224.0.3"); err != nil {
		t.Errorf("cloned container did not come online in time: %v\n", err)
	} else {
		t.Logf("cloned container with ID %d is online\n", newCT.VMID)
	}

	if err = api.StopContainer(newCT); err != nil {
		t.Errorf("failed to stop cloned container: %v\n", err)
	} else {
		t.Logf("stopped cloned container with ID %d\n", newCT.VMID)
	}

	if err = api.DeleteContainer(newCT); err != nil {
		t.Errorf("failed to delete cloned container: %v\n", err)
	} else {
		t.Logf("deleted cloned container with ID %d\n", newCT.VMID)
	}

	if err = api.DeleteContainer(result.Container); err != nil {
		t.Errorf("failed to delete original container: %v\n", err)
	} else {
		t.Logf("deleted original container with ID %d\n", result.CTID)
	}
}

// This test should be run without a timeout as this can take a while
// go test -timeout=9999s -v -run ^TestMassCTLifecycle$ github.com/UNHCSC/pve-koth/tests
func TestMassCTLifecycle(t *testing.T) {
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

	var pubKey, privKey string
	if pubKey, privKey, err = sshcomm.CreateSSHKeyPair("tmpssh"); err != nil {
		t.Fatalf("failed to create SSH key pair: %v\n", err)
		return
	}
	defer os.RemoveAll("tmpssh")

	var confs = make([]*proxmoxAPI.ContainerCreateOptions, 32)
	for i := range len(confs) {
		confs[i] = &proxmoxAPI.ContainerCreateOptions{
			TemplatePath:     "isos-ct_templates:vztmpl/ubuntu-25.04-standard_25.04-1.1_amd64.tar.zst",
			StoragePool:      "laas",
			Hostname:         fmt.Sprintf("koth-mass-test-ct-%d", i),
			RootPassword:     "password",
			RootSSHPublicKey: pubKey,
			StorageSizeGB:    16,
			MemoryMB:         8192,
			Cores:            4,
			GatewayIPv4:      "10.0.0.1",
			IPv4Address:      fmt.Sprintf("10.224.254.%d", 1+i),
			CIDRBlock:        8,
			NameServer:       "10.0.0.2",
			SearchDomain:     "cyber.lab",
		}
	}

	var results []*proxmoxAPI.ProxmoxAPIBulkCreateResult
	if results, err = api.BulkCreateContainersConcurrent(api.Nodes, confs, len(confs)/8); err != nil {
		t.Fatalf("failed to bulk create containers: %v\n", err)
		return
	} else if results == nil {
		t.Fatalf("bulk create containers returned nil results")
		return
	}

	var wg sync.WaitGroup
	for i, result := range results {
		if result.Error != nil {
			t.Errorf("failed to create container: %v\n", result.Error)
			continue
		}

		wg.Add(1)
		go func(res *proxmoxAPI.ProxmoxAPICreateResult, cfg *proxmoxAPI.ContainerCreateOptions) {
			defer wg.Done()

			if err = api.CTActionWithRetries(api.StartContainer, res.Container, 10); err != nil {
				t.Errorf("failed to start container %d: %v\n", res.CTID, err)
			}

			defer func() {
				if err = api.CTActionWithRetries(api.StopContainer, res.Container, 10); err != nil {
					t.Errorf("failed to stop container %d: %v\n", res.CTID, err)
				}

				if err = api.CTActionWithRetries(api.DeleteContainer, res.Container, 10); err != nil {
					t.Errorf("failed to delete container %d: %v\n", res.CTID, err)
				}
			}()

			// Update package lists
			var conn *sshcomm.SSHConnection
			if conn, err = sshcomm.ConnectOnceReadyWithRetry("root", cfg.IPv4Address, 22, sshcomm.WithPrivateKey([]byte(privKey)), 10); err != nil {
				t.Errorf("failed to connect to container %d via SSH w/ Retries: %v\n", res.CTID, err)
			} else {
				defer conn.Close()
				var (
					exitCode int
					output   string
				)

				// if exitCode, _, err = conn.SendWithOutput("apt-get update"); err != nil {
				// 	t.Errorf("failed to send 'apt-get update' to container %d: %v\n", res.CTID, err)
				// } else if exitCode != 0 {
				// 	t.Logf("'apt-get update' on container %d exited with code %d\n", res.CTID, exitCode)
				// }

				// if err = conn.Reset(); err != nil {
				// 	t.Errorf("failed to reset SSH connection to container %d: %v\n", res.CTID, err)
				// }

				// if exitCode, _, err = conn.SendWithOutput("apt-get upgrade -y"); err != nil {
				// 	t.Errorf("failed to send 'apt-get upgrade -y' to container %d: %v\n", res.CTID, err)
				// } else if exitCode != 0 {
				// 	t.Logf("'apt-get upgrade -y' on container %d exited with code %d\n", res.CTID, exitCode)
				// }

				// if err = conn.Reset(); err != nil {
				// 	t.Errorf("failed to reset SSH connection to container %d: %v\n", res.CTID, err)
				// }

				// if exitCode, _, err = conn.SendWithOutput("apt-get install -y neofetch"); err != nil {
				// 	t.Errorf("failed to send 'apt-get install -y neofetch' to container %d: %v\n", res.CTID, err)
				// } else if exitCode != 0 {
				// 	t.Logf("'apt-get install -y neofetch' on container %d exited with code %d\n", res.CTID, exitCode)
				// }

				// if err = conn.Reset(); err != nil {
				// 	t.Errorf("failed to reset SSH connection to container %d: %v\n", res.CTID, err)
				// }

				// if exitCode, _, err = conn.SendWithOutput("neofetch"); err != nil {
				// 	t.Errorf("failed to send 'neofetch' to container %d: %v\n", res.CTID, err)
				// } else if exitCode != 0 {
				// 	t.Logf("'neofetch' on container %d exited with code %d\n", res.CTID, exitCode)
				// }

				if exitCode, output, err = conn.SendWithOutput("whoami"); err != nil {
					t.Errorf("failed to send 'uname -a' to container %d: %v\n", res.CTID, err)
				} else if exitCode != 0 {
					t.Logf("'whoami' on container %d exited with code %d\n", res.CTID, exitCode)
				} else if strings.TrimSpace(output) != "root" {
					t.Errorf("unexpected output from 'whoami' on container %d: %s\n", res.CTID, output)
				}
			}
		}(result.Result, confs[i])
	}

	wg.Wait()
}

func TestCleanup(t *testing.T) {
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

	defer os.RemoveAll("tmpssh")

	var containers []*proxmox.Container
	for _, node := range api.Nodes {
		var nodeContainers []*proxmox.Container
		if nodeContainers, err = api.ListContainers(node); err != nil {
			t.Fatalf("failed to list containers on node %s: %v\n", node.Name, err)
			return
		}

		containers = append(containers, nodeContainers...)
	}

	var wg sync.WaitGroup
	for _, ct := range containers {
		if strings.HasPrefix(ct.Name, "koth-") {
			t.Logf("Cleaning up container %d (%s)...", ct.VMID, ct.Name)
			wg.Add(1)

			go func(container *proxmox.Container) {
				defer wg.Done()

				if err = api.StopContainer(container); err != nil {
					t.Errorf("failed to stop container %d: %v\n", container.VMID, err)
				}

				if err = api.DeleteContainer(container); err != nil {
					t.Errorf("failed to delete container %d: %v\n", container.VMID, err)
				}
			}(ct)
		}
	}

	wg.Wait()
}
