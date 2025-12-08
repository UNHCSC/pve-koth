package tests

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/proxmoxAPI"
	"github.com/UNHCSC/pve-koth/ssh"
	"github.com/gofiber/fiber/v2"
	"github.com/luthermonson/go-proxmox"
)

func setup(t *testing.T) {
	var err error

	if err = config.Init("../config.toml"); err != nil {
		t.Fatalf("Failed to load config.toml: %v", err)
	}

	// Randomize the database file name to avoid conflicts
	config.Config.Database.File = fmt.Sprintf("test_db_%d.db", time.Now().UnixNano())

	if err = db.Init(); err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
}

func initProxmox(t *testing.T) (api *proxmoxAPI.ProxmoxAPI) {
	var err error

	if api, err = proxmoxAPI.InitProxmox(); err != nil {
		t.Fatalf("failed to initialize Proxmox API: %v\n", err)
	}

	if len(api.Nodes) == 0 {
		t.Fatalf("no Proxmox nodes found")
	}

	return
}

func initWebServer() (reqToken func() (token string), revToken func(token string)) {
	var tokens map[string]bool = map[string]bool{}

	reqToken = func() (token string) {
		token = fmt.Sprintf("test_token_%d", time.Now().UnixNano())
		tokens[token] = true
		return
	}

	revToken = func(token string) {
		delete(tokens, token)
	}

	var app *fiber.App = fiber.New(fiber.Config{})
	app.Use(func(c *fiber.Ctx) error {
		var token string = c.Cookies("Authorization", "")
		if _, exists := tokens[token]; !exists {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		return c.Next()
	})

	app.Static("/", "./public")

	go app.Listen(":8080")

	return
}

func container(t *testing.T, api *proxmoxAPI.ProxmoxAPI, node *proxmox.Node, conf *proxmoxAPI.ContainerCreateOptions) (result *proxmoxAPI.ProxmoxAPICreateResult, cleanup func() (err error)) {
	var err error

	// Creation
	t.Logf("Creating container on node %s", node.Name)
	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	t.Logf("Created container with ID %d", result.CTID)

	cleanup = func() (err error) {
		// Power off (if running)
		t.Logf("Stopping container with ID %d", result.CTID)
		if err = api.StopContainer(result.Container); err != nil && !strings.Contains(err.Error(), "not running") {
			t.Fatalf("failed to stop container: %v", err)
		}

		t.Logf("Stopped container with ID %d", result.CTID)

		// Destruction
		t.Logf("Destroying container with ID %d", result.CTID)
		if err = api.DeleteContainer(result.Container); err != nil {
			t.Fatalf("failed to destroy container: %v", err)
		}

		t.Logf("Destroyed container with ID %d", result.CTID)
		return
	}

	// Power on
	t.Logf("Starting container with ID %d", result.CTID)
	if err = api.StartContainer(result.Container); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	t.Logf("Started container with ID %d", result.CTID)

	return
}

func getIPs(t *testing.T, num int) (addresses []string, err error) {
	var (
		firstIP, lastIP net.IP
		ips             []net.IP
	)

	if firstIP, lastIP, _, err = ssh.ParseSubnet(config.Config.Proxmox.Testing.SubnetCIDR); err != nil {
		t.Fatalf("failed to get IP range from subnet %s: %v\n", config.Config.Proxmox.Testing.SubnetCIDR, err)
		return
	}

	ips = ssh.GetSubnetRange(firstIP, lastIP)

	var openIPs []net.IP
	if openIPs, err = ssh.FindOpenIPs(ips, num); err != nil {
		t.Fatalf("failed to find open IPs in subnet %s: %v\n", config.Config.Proxmox.Testing.SubnetCIDR, err)
		return
	}

	for i := range openIPs {
		addresses = append(addresses, openIPs[i].String())
		if len(addresses) >= num {
			break
		}
	}

	return
}

func provisionConf(ip, hostname, sshPubKey string) (conf *proxmoxAPI.ContainerCreateOptions) {
	conf = &proxmoxAPI.ContainerCreateOptions{
		TemplatePath:     config.Config.Proxmox.Testing.UbuntuTemplate,
		StoragePool:      config.Config.Proxmox.Testing.Storage,
		Hostname:         hostname,
		RootPassword:     "password",
		RootSSHPublicKey: sshPubKey,
		StorageSizeGB:    4,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      config.Config.Proxmox.Testing.Gateway,
		IPv4Address:      ip,
		CIDRBlock:        8,
		NameServer:       config.Config.Proxmox.Testing.DNS,
		SearchDomain:     config.Config.Proxmox.Testing.SearchDomain,
	}

	return
}

type ProxmoxTestingEnvironment struct {
	api                *proxmoxAPI.ProxmoxAPI
	nodeRotator        int
	configs            []*proxmoxAPI.ContainerCreateOptions
	containerHostnames []string
	ips                []string
	results            []*proxmoxAPI.ProxmoxAPICreateResult
	cleanups           []func() (err error)
	reqToken           func() (token string)
	revToken           func(token string)
	pubKey, privKey    string
	cleanSSHHandler    func() (err error)
}

func proxmoxEnvironmentSetup(t *testing.T, needsSSHKeys, needsWebserver bool, containerHostnames []string) (env *ProxmoxTestingEnvironment, err error) {
	env = &ProxmoxTestingEnvironment{
		nodeRotator:        0,
		containerHostnames: containerHostnames,
	}

	if env.api, err = initProxmox(t), nil; err != nil {
		return
	}

	if needsSSHKeys {
		var sshPath string = fmt.Sprintf("%s/tmpssh%d", os.TempDir(), time.Now().UnixNano())

		if env.pubKey, env.privKey, err = ssh.CreateSSHKeyPair(sshPath); err != nil {
			t.Fatalf("failed to generate SSH key pair: %v", err)
		}

		env.cleanSSHHandler = func() (err error) {
			if err = os.RemoveAll(sshPath); err != nil {
				t.Fatalf("failed to remove temporary SSH key pair: %v", err)
			}

			return
		}
	}

	if needsWebserver {
		env.reqToken, env.revToken = initWebServer()
	}

	return
}

func cleanup(t *testing.T) {
	var err error

	if err = db.Close(); err != nil {
		t.Fatalf("Failed to close DB: %v", err)
	}

	if err = os.Remove(db.DatabaseFilePath()); err != nil {
		t.Fatalf("Failed to remove test database file: %v", err)
	}
}
