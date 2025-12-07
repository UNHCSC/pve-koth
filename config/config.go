package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	} `toml:"proxmox"` // Proxmox VE integration configuration
}

var Config Configuration

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
