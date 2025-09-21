package config

import (
	"fmt"
	"os"

	"github.com/Netflix/go-env"
	"github.com/joho/godotenv"
)

type Configuration struct {
	Database struct {
		File string `env:"DB_FILE,default=pve-koth.db"`
	}

	LDAP struct {
		Address    string `env:"LDAP_ADDRESS,required=true"`
		DomainSLD  string `env:"LDAP_DOMAIN_SLD,required=true"`
		DomainTLD  string `env:"LDAP_DOMAIN_TLD,required=true"`
		AccountsCN string `env:"LDAP_ACCOUNTS_CN,default=accounts"`
		UsersCN    string `env:"LDAP_USERS_CN,default=users"`
		GroupsCN   string `env:"LDAP_GROUPS_CN,default=groups"`

		// Array values are separated with "|" in the .env file (e.g. LDAP_ADMIN_GROUPS=admins|laasAdmins)
		AdminGroups []string `env:"LDAP_ADMIN_GROUPS,required=true"`
		UserGroups  []string `env:"LDAP_USER_GROUPS,required=true"`
	}

	WebServer struct {
		Address string `env:"WEB_ADDRESS,default=:8080"`
		TlsDir  string `env:"WEB_TLS_DIR"`
	}

	Proxmox struct {
		Host    string `env:"PROXMOX_HOST,required=true"`
		Port    string `env:"PROXMOX_PORT,required=true"`
		TokenID string `env:"PROXMOX_API_TOKEN_ID,required=true"`
		Secret  string `env:"PROXMOX_API_TOKEN_SECRET,required=true"`
	}
}

var Config Configuration

// Try to initialize the environment variables from a .env in the directory the program is run from.
// If the .env file is not present, we will create a sample .env file based on the Configuration struct.
// You can then use config.Config globally
func InitEnv(path string) error {
	if _, err := os.Stat(path); err != nil {
		if e := GenerateSampleEnvFile(path); e != nil {
			return e
		}

		return fmt.Errorf("no .env file found, created a sample .env file. Please fill in the required values and try again")
	}

	if err := godotenv.Load(path); err != nil {
		return err
	}

	_, err := env.UnmarshalFromEnviron(&Config)
	if err != nil {
		return err
	}

	return nil
}
