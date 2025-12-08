package tests

import (
	"fmt"
	"maps"
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

func initWebServer(t *testing.T, port string) (reqToken func() (token string), revToken func(token string)) {
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

		t.Logf("Fiber server authorized request for path: %s", c.Path())

		return c.Next()
	})

	app.Static("/", "./public")

	go app.Listen(fmt.Sprintf("0.0.0.0:%s", port))

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
	sshConnections     []*ssh.SSHConnection
	webPort            string
}

func (tenv *ProxmoxTestingEnvironment) Cleanup(t *testing.T) {
	var err error

	// Clean up containers
	for _, cleanup := range tenv.cleanups {
		if err = cleanup(); err != nil {
			t.Fatalf("failed to clean up container: %v", err)
		}
	}

	// Clean up SSH keys
	if tenv.cleanSSHHandler != nil {
		if err = tenv.cleanSSHHandler(); err != nil {
			t.Fatalf("failed to clean up SSH keys: %v", err)
		}
	}

	// Clean up SSH connections
	for _, conn := range tenv.sshConnections {
		if conn == nil {
			continue
		}

		if err = conn.Close(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "eof") {
			t.Fatalf("failed to close SSH connection: %v", err)
		}
	}
}

func (tenv *ProxmoxTestingEnvironment) ConnectSSH(idx int) (conn *ssh.SSHConnection, err error) {
	if idx < 0 || idx >= len(tenv.results) {
		err = fmt.Errorf("index out of range")
		return
	}

	if conn, err = ssh.ConnectOnceReadyWithRetry("root", tenv.ips[idx], 22, 5, ssh.WithPrivateKey([]byte(tenv.privKey))); err != nil {
		return
	}

	tenv.sshConnections[idx] = conn
	return
}

func (tenv *ProxmoxTestingEnvironment) ResetSSH(idx int) (err error) {
	if idx < 0 || idx >= len(tenv.results) {
		err = fmt.Errorf("index out of range")
		return
	}

	if tenv.sshConnections[idx] == nil {
		err = fmt.Errorf("SSH connection not established for container at index %d", idx)
		return
	}

	err = tenv.sshConnections[idx].Reset()
	return
}

func (tenv *ProxmoxTestingEnvironment) EnvsFor(idx int, overwrites ...map[string]any) (envs map[string]any, err error) {
	if idx < 0 || idx >= len(tenv.results) {
		err = fmt.Errorf("index out of range")
		return
	}

	envs = map[string]any{
		"KOTH_COMP_ID":       "test_competition",
		"KOTH_TEAM_ID":       fmt.Sprintf("%d", idx+1),
		"KOTH_HOSTNAME":      tenv.containerHostnames[idx],
		"KOTH_IP":            tenv.ips[idx],
		"KOTH_PUBLIC_FOLDER": fmt.Sprintf("http://%s", net.JoinHostPort(ssh.MustLocalIP(), tenv.webPort)),
		"KOTH_ACCESS_TOKEN":  tenv.reqToken(),
	}

	for _, overwrite := range overwrites {
		maps.Copy(envs, overwrite)
	}

	return
}

func (tenv *ProxmoxTestingEnvironment) ExecuteOn(idx int, command string) (status int, output string, err error) {
	if idx < 0 || idx >= len(tenv.results) {
		err = fmt.Errorf("index out of range")
		return
	}

	if tenv.sshConnections[idx] == nil {
		err = fmt.Errorf("SSH connection not established for container at index %d", idx)
		return
	}

	var bytesOut []byte
	status, bytesOut, err = tenv.sshConnections[idx].SendWithOutput(command)
	output = string(bytesOut)
	return
}

func proxmoxEnvironmentSetup(t *testing.T, needsSSHKeys, needsWebserver bool, containerHostnames []string, useBulkOperations bool) (env *ProxmoxTestingEnvironment, err error) {
	env = &ProxmoxTestingEnvironment{
		nodeRotator:        0,
		containerHostnames: containerHostnames,
		sshConnections:     make([]*ssh.SSHConnection, len(containerHostnames)),
	}

	env.api = initProxmox(t)

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
		if _, env.webPort, err = net.SplitHostPort(config.Config.WebServer.Address); err != nil {
			t.Fatalf("failed to parse web server address: %v", err)
		}

		env.reqToken, env.revToken = initWebServer(t, env.webPort)
	}

	// Allocate IPs
	if env.ips, err = getIPs(t, len(containerHostnames)); err != nil {
		return
	}

	// Prepare container configs
	for i, hostname := range containerHostnames {
		var conf *proxmoxAPI.ContainerCreateOptions = provisionConf(env.ips[i], hostname, env.pubKey)
		env.configs = append(env.configs, conf)
	}

	// Create containers
	if useBulkOperations {
		var bulkResults []*proxmoxAPI.ProxmoxAPIBulkCreateResult
		if bulkResults, err = env.api.BulkCreateContainersConcurrent(env.api.Nodes, env.configs, 1+len(containerHostnames)/4); err != nil {
			t.Fatalf("failed to bulk create containers: %v", err)
		}

		for _, bulkResult := range bulkResults {
			env.results = append(env.results, bulkResult.Result)
			env.cleanups = append(env.cleanups, bulkResult.Cleanup)
		}
	} else {
		for _, conf := range env.configs {
			var node *proxmox.Node = env.api.Nodes[env.nodeRotator%len(env.api.Nodes)]
			env.nodeRotator++

			var result *proxmoxAPI.ProxmoxAPICreateResult
			var cleanup func() (err error)

			result, cleanup = container(t, env.api, node, conf)

			env.results = append(env.results, result)
			env.cleanups = append(env.cleanups, cleanup)
		}
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

func readPublicFileContents(fileName string) (contents string, err error) {
	var data []byte
	if data, err = os.ReadFile(fmt.Sprintf("./public/%s", fileName)); err != nil {
		return
	}

	contents = string(data)
	return
}

func genHostnamesHelper(prefix string, count int) (hostnames []string) {
	for i := range count {
		hostnames = append(hostnames, fmt.Sprintf("%s%d", prefix, i+1))
	}

	return
}
