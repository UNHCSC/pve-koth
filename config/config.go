package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
)

type Configuration struct {
	WebServer struct {
		Address                     string   `toml:"address" default:":8080" validate:"required"`                     // Listen address for the web application server e.g. ":8080" or "0.0.0.0:8080"
		TLSDir                      string   `toml:"tls_dir" default:""`                                              // Directory containing a crt and a key file for TLS. Leave empty to use HTTP instead of HTTPS.
		ReloadTemplatesOnEachRender bool     `toml:"reload_templates_on_each_render" default:"false"`                 // For development purposes. If true, templates are reloaded from disk on each render.
		RedirectServerAddresses     []string `toml:"redirect_server_addresses" default:"[]" validate:"dive,required"` // List of addresses ("host:port" or ":port") to which HTTP requests should be redirected to HTTPS. If your web app is on ":443", you might want to redirect ":80" here.
		PublicURL                   string   `toml:"public_url" default:""`                                           // Optional externally reachable base URL used inside containers (e.g. "https://koth.cyber.lab")
	} `toml:"web_server"` // Web server configuration

	Database struct {
		File string `toml:"file" default:"koth.db" validate:"required"` // Path to the MySQL database file
	} `toml:"database"` // Database configuration

	LDAP struct {
		Address     string   `toml:"address" default:"" validate:"required"`                   // LDAP server address (e.g. "ldaps://domain.cyber.lab:636")
		DomainSLD   string   `toml:"domain_sld" default:"" validate:"required"`                // LDAP domain second-level domain (e.g. "cyber" for "domain.cyber.lab")
		DomainTLD   string   `toml:"domain_tld" default:"" validate:"required"`                // LDAP domain top-level domain (e.g. "lab" for "domain.cyber.lab")
		AccountsCN  string   `toml:"accounts_cn" default:"accounts" validate:"required"`       // LDAP container name for accounts (usually "accounts")
		UsersCN     string   `toml:"users_cn" default:"users" validate:"required"`             // LDAP container name for users (usually "users")
		GroupsCN    string   `toml:"groups_cn" default:"groups" validate:"required"`           // LDAP container name for groups (usually "groups")
		AdminGroups []string `toml:"admin_groups" default:"[\"admins\"]" validate:"required"`  // LDAP groups whose members should have admin access to the web app
		UserGroups  []string `toml:"user_groups" default:"[\"ipausers\"]" validate:"required"` // LDAP groups whose members should have user access to the web app
	} `toml:"ldap"` // LDAP configuration

	Proxmox struct {
		Hostname string `toml:"hostname" default:"proxmox.local" validate:"required"` // Proxmox VE server hostname or IP address (e.g. "proxmox.cyber.lab")
		Port     string `toml:"port" default:"8006" validate:"required"`              // Proxmox VE API port (usually "8006")
		TokenID  string `toml:"token_id" default:"" validate:"required"`              // Proxmox VE API token ID (e.g. "laas-api-token-id")
		Secret   string `toml:"secret" default:"" validate:"required"`                // Proxmox VE API token secret
		Username string `toml:"username" default:""`                                  // Proxmox VE username (with realm) for ticket-based console auth, e.g. "root@pam"
		Password string `toml:"password" default:""`                                  // Proxmox VE password for ticket-based console auth
		Testing  struct {
			Enabled        bool   `toml:"enabled" default:"false"`                                                                          // Enable Proxmox VE integration testing mode
			SubnetCIDR     string `toml:"subnet_cidr" default:"10.255.0.0/16"`                                                              // Subnet CIDR to use for testing VMs
			Storage        string `toml:"storage" default:"team"`                                                                           // Proxmox VE storage to use for testing VMs
			UbuntuTemplate string `toml:"ubuntu_template" default:"isos-ct_templates:vztmpl/ubuntu-25.04-standard_25.04-1.1_amd64.tar.zst"` // Proxmox VE container template to use for testing VMs
			Gateway        string `toml:"gateway" default:"10.0.0.1"`                                                                       // Gateway IP for testing VMs
			DNS            string `toml:"dns" default:"10.0.0.2"`                                                                           // DNS server IP for testing VMs
			SearchDomain   string `toml:"search_domain" default:"cyber.lab"`                                                                // Search domain for testing VMs
		} `toml:"testing"` // Proxmox VE integration testing configuration
	} `toml:"proxmox"` // Proxmox VE integration configuration

	Storage struct {
		BasePath string `toml:"base_path" default:"./koth_live_data" validate:"required"` // Root directory where uploaded competition packages are stored
	} `toml:"storage"`

	Network               NetworkConfig               `toml:"network"`
	ContainerRestrictions ContainerRestrictionsConfig `toml:"container_restrictions"`
}

var Config Configuration

type NetworkConfig struct {
	PoolCIDR                string `toml:"pool_cidr" default:"10.0.0.0/8" validate:"required"`
	CompetitionSubnetPrefix int    `toml:"competition_subnet_prefix" default:"16" validate:"min=8,max=30"`
	TeamSubnetPrefix        int    `toml:"team_subnet_prefix" default:"24" validate:"min=8,max=30"`
	ContainerCIDR           int    `toml:"container_cidr" default:"8" validate:"min=1,max=30"`
	ContainerGateway        string `toml:"container_gateway" default:"10.0.0.1" validate:"required,ipv4"`
	ContainerNameserver     string `toml:"container_nameserver" default:"10.0.0.2" validate:"required,ipv4"`
	ContainerSearchDomain   string `toml:"container_search_domain" default:"cyber.lab" validate:"required"`

	parsedPool *net.IPNet `toml:"-"`
}

type ContainerRestrictionsConfig struct {
	AllowedLXCTemplates []string `toml:"allowed_lxc_templates" default:"[]"`
	AllowedStoragePools []string `toml:"allowed_storage_pools" default:"[]"`
	MaxCPUCores         int      `toml:"max_cpu_cores" default:"4" validate:"min=1"`
	MaxMemoryMB         int      `toml:"max_memory_mb" default:"8192" validate:"min=1"`
	MaxDiskMB           int      `toml:"max_disk_mb" default:"32768" validate:"min=1"`
}

func (n *NetworkConfig) initialize() error {
	if n == nil {
		return fmt.Errorf("network configuration missing")
	}

	var (
		err error
		ip  net.IP
	)

	if ip, n.parsedPool, err = net.ParseCIDR(n.PoolCIDR); err != nil {
		return fmt.Errorf("invalid pool_cidr %q: %w", n.PoolCIDR, err)
	}

	if ip.To4() == nil {
		return fmt.Errorf("pool_cidr must be an IPv4 network")
	}

	maskOnes, maskBits := n.parsedPool.Mask.Size()
	if maskBits != 32 {
		return fmt.Errorf("pool_cidr must be an IPv4 network")
	}

	if maskOnes > 16 {
		return fmt.Errorf("pool_cidr %s is smaller than /16 and cannot supply competition networks", n.PoolCIDR)
	}

	if n.CompetitionSubnetPrefix != 16 {
		return fmt.Errorf("competition_subnet_prefix must currently be 16")
	}

	if n.CompetitionSubnetPrefix < maskOnes {
		return fmt.Errorf("competition_subnet_prefix (/ %d) must be equal to or larger than pool prefix (/ %d)", n.CompetitionSubnetPrefix, maskOnes)
	}

	if n.TeamSubnetPrefix < n.CompetitionSubnetPrefix {
		return fmt.Errorf("team_subnet_prefix (/ %d) must be larger than competition subnet (/ %d)", n.TeamSubnetPrefix, n.CompetitionSubnetPrefix)
	}

	if n.ContainerCIDR > n.CompetitionSubnetPrefix {
		return fmt.Errorf("container_cidr (/ %d) must be less specific than competition subnet (/ %d)", n.ContainerCIDR, n.CompetitionSubnetPrefix)
	}

	// Canonicalize stored IP reference
	n.parsedPool.IP = ip.To4()
	return nil
}

func (n *NetworkConfig) ParsedPool() *net.IPNet {
	if n == nil || n.parsedPool == nil {
		return nil
	}

	var clone = *n.parsedPool
	clone.IP = append(net.IP(nil), n.parsedPool.IP...)
	return &clone
}

func loadConfig(path string) (err error) {
	// Apply struct defaults BEFORE loading TOML (so TOML overrides)
	if err = defaults.Set(&Config); err != nil {
		err = fmt.Errorf("set defaults: %w", err)
		return
	}

	// Decode TOML file into struct
	if _, err = toml.DecodeFile(path, &Config); err != nil {
		err = fmt.Errorf("decode toml: %w", err)
		return
	}

	// Validate required fields
	if err = validator.New(validator.WithRequiredStructEnabled()).Struct(Config); err != nil {
		err = fmt.Errorf("validate config: %w", err)
		return
	}

	if err = Config.Network.initialize(); err != nil {
		err = fmt.Errorf("network config: %w", err)
	}

	return
}

// generateDefaultConfig writes a config.toml with all default values filled in.
// It will overwrite any existing file at path.
func generateDefaultConfig(path string) (err error) {
	var cfg Configuration

	// 1. Apply struct defaults
	if err = defaults.Set(&cfg); err != nil {
		err = fmt.Errorf("set defaults: %w", err)
		return
	}

	// NOTE: Do NOT validate here.
	// The default config is allowed to be "invalid" from a required-fields POV;
	// it's just a template for the user to fill in.
	// Validation happens in LoadConfig() when we actually load the file.

	// 2. Create / truncate the file
	var file *os.File
	if file, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); err != nil {
		err = fmt.Errorf("create config file: %w", err)
		return
	}

	defer file.Close()

	// 3. Encode as TOML
	var encoder *toml.Encoder = toml.NewEncoder(file)
	encoder.Indent = "    "
	if err = encoder.Encode(cfg); err != nil {
		err = fmt.Errorf("encode toml: %w", err)
	}

	return
}

func Init(path string) (err error) {
	if !filepath.IsAbs(path) {
		if path, err = filepath.Abs(path); err != nil {
			return err
		}
	}

	if _, err = os.Stat(path); err != nil {
		if err = generateDefaultConfig(path); err != nil {
			return
		}

		err = fmt.Errorf("no config file found, created a default config at %s. Please fill in the required values and try again", path)
		return
	}

	if err = loadConfig(path); err != nil {
		return err
	}

	return nil
}

func StorageBasePath() string {
	var base = strings.TrimSpace(Config.Storage.BasePath)
	if base == "" {
		base = "./koth_live_data"
	}
	return base
}
