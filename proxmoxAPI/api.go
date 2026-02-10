package proxmoxAPI

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/luthermonson/go-proxmox"
)

type ProxmoxAPI struct {
	createLock         sync.Mutex
	client             *proxmox.Client
	bg                 context.Context
	Nodes              []*proxmox.Node
	Cluster            *proxmox.Cluster
	nodeRotator        int
	tokenUser          string
	apiHost            string
	apiPort            string
	username           string
	password           string
	InsecureSkipVerify bool
	httpClient         *http.Client
	ConnectTimeout     time.Duration
	CommandTimeout     time.Duration
}

type ProxmoxAPICreateResult struct {
	Container *proxmox.Container
	CTID      int
}

type ProxmoxAPIBulkCreateResult struct {
	Result *ProxmoxAPICreateResult
	Error  error
}

func InitProxmox() (api *ProxmoxAPI, err error) {
	var (
		tlsConfig  = &tls.Config{InsecureSkipVerify: true}
		transport  = &http.Transport{TLSClientConfig: tlsConfig}
		httpClient = &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
	)

	api = &ProxmoxAPI{
		client: proxmox.NewClient(
			fmt.Sprintf("https://%s:%s/api2/json", config.Config.Proxmox.Hostname, config.Config.Proxmox.Port),
			proxmox.WithHTTPClient(&http.Client{
				Transport: transport,
			}),
			proxmox.WithAPIToken(config.Config.Proxmox.TokenID, config.Config.Proxmox.Secret),
		),
		bg:                 context.Background(),
		Nodes:              make([]*proxmox.Node, 0),
		apiHost:            config.Config.Proxmox.Hostname,
		apiPort:            config.Config.Proxmox.Port,
		username:           config.Config.Proxmox.Username,
		password:           config.Config.Proxmox.Password,
		InsecureSkipVerify: true,
		httpClient:         httpClient,
		ConnectTimeout:     30 * time.Second,
		CommandTimeout:     5 * time.Minute,
	}
	if token := config.Config.Proxmox.TokenID; token != "" {
		if parts := strings.SplitN(token, "!", 2); len(parts) > 0 {
			api.tokenUser = parts[0]
		}
	}

	if api.Cluster, err = api.client.Cluster(api.bg); err != nil {
		err = fmt.Errorf("failed to get cluster info: %w", err)
		return
	}

	var nodeStatuses []*proxmox.NodeStatus
	if nodeStatuses, err = api.client.Nodes(api.bg); err != nil {
		err = fmt.Errorf("failed to get node statuses: %w", err)
		return
	}

	for _, ns := range nodeStatuses {
		if ns.Status == "online" {
			var node *proxmox.Node
			if node, err = api.client.Node(api.bg, ns.Node); err != nil {
				err = fmt.Errorf("failed to get node info for node %s: %w", ns.Node, err)
				return
			}

			api.Nodes = append(api.Nodes, node)
		}
	}

	return
}

func (api *ProxmoxAPI) NextNode() *proxmox.Node {
	if len(api.Nodes) == 0 {
		return nil
	}

	api.nodeRotator = (api.nodeRotator + 1) % len(api.Nodes)
	return api.Nodes[api.nodeRotator]
}

func (api *ProxmoxAPI) NodeByName(name string) *proxmox.Node {
	if name == "" {
		return nil
	}

	for _, node := range api.Nodes {
		if node != nil && node.Name == name {
			return node
		}
	}

	node, err := api.client.Node(api.bg, name)
	if err != nil {
		return nil
	}
	api.Nodes = append(api.Nodes, node)
	return node
}
