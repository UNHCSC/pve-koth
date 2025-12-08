package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type flexibleInt int

func (f *flexibleInt) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*f = 0
		return nil
	}

	var numeric int
	if err := json.Unmarshal(data, &numeric); err == nil {
		*f = flexibleInt(numeric)
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			*f = 0
			return nil
		}

		value, convErr := strconv.Atoi(text)
		if convErr != nil {
			return fmt.Errorf("invalid numeric value %q", text)
		}

		*f = flexibleInt(value)
		return nil
	}

	return fmt.Errorf("failed to parse numeric value: %s", string(data))
}

type flexibleStringList []string

func (l *flexibleStringList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*l = nil
		return nil
	}

	if len(data) > 0 && data[0] == '[' {
		var list []string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*l = flexibleStringList(list)
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return fmt.Errorf("failed to parse string list: %s", string(data))
	}

	single = strings.TrimSpace(single)
	if single == "" {
		*l = nil
		return nil
	}

	*l = flexibleStringList{single}
	return nil
}

type Team struct {
	ID           int64     `json:"id" gomysql:"id,primary,increment"`
	Name         string    `json:"name" gomysql:"name,unique"`
	Score        int       `json:"score" gomysql:"score"`
	ContainerIDs []int64   `json:"containerIDs" gomysql:"container_ids"`
	LastUpdated  time.Time `json:"lastUpdated" gomysql:"last_updated"`
	CreatedAt    time.Time `json:"createdAt" gomysql:"created_at"`
}

type Container struct {
	PVEID       int64     `json:"id" gomysql:"id,primary,unique"`
	IPAddress   string    `json:"ipAddress" gomysql:"ip_address,unique"`
	Status      string    `json:"status" gomysql:"status"`
	LastUpdated time.Time `json:"lastUpdated" gomysql:"last_updated"`
	CreatedAt   time.Time `json:"createdAt" gomysql:"created_at"`
}

type ScoreResult struct {
	ID             int64     `json:"id" gomysql:"id,primary,increment"`
	TeamID         int64     `json:"teamID" gomysql:"team_id"`
	ContainerName  string    `json:"containerName" gomysql:"container_name"`
	ContainerOrder int       `json:"containerOrder" gomysql:"container_order"`
	CheckID        string    `json:"checkID" gomysql:"check_id"`
	CheckName      string    `json:"checkName" gomysql:"check_name"`
	CheckOrder     int       `json:"checkOrder" gomysql:"check_order"`
	PassPoints     int       `json:"passPoints" gomysql:"pass_points"`
	FailPoints     int       `json:"failPoints" gomysql:"fail_points"`
	Passed         bool      `json:"passed" gomysql:"passed"`
	UpdatedAt      time.Time `json:"updatedAt" gomysql:"updated_at"`
}

type ContainerRestrictions struct {
	HostnamePrefix string `json:"hostnamePrefix" gomysql:"hostname_prefix"`
	RootPassword   string `json:"rootPassword" gomysql:"root_password"`
	Template       string `json:"template" gomysql:"template"`
	StoragePool    string `json:"storagePool" gomysql:"storage_pool"`
	GatewayIPv4    string `json:"gatewayIPv4" gomysql:"gateway_ipv4"`
	Nameserver     string `json:"nameserver" gomysql:"nameserver"`
	SearchDomain   string `json:"searchDomain" gomysql:"search_domain"`
	StorageGB      int    `json:"storageGB" gomysql:"storage_gb"`
	MemoryMB       int    `json:"memoryMB" gomysql:"memory_mb"`
	Cores          int    `json:"cores" gomysql:"cores"`
	IndividualCIDR int    `json:"individualCIDR" gomysql:"individual_cidr"`
}

type Competition struct {
	ID                       int64                 `json:"id" gomysql:"id,primary,increment"`
	SystemID                 string                `json:"competitionID" gomysql:"competition_id,unique"`
	Name                     string                `json:"name" gomysql:"name,unique"`
	Description              string                `json:"description" gomysql:"description"`
	Host                     string                `json:"host" gomysql:"host"`
	TeamIDs                  []int64               `json:"teamIDs" gomysql:"team_ids"`
	ContainerIDs             []int64               `json:"containerIDs" gomysql:"container_ids"`
	CreatedAt                time.Time             `json:"createdAt" gomysql:"created_at"`
	SSHPubKeyPath            string                `json:"sshPubKeyPath" gomysql:"ssh_pub_key_path"`
	SSHPrivKeyPath           string                `json:"sshPrivKeyPath" gomysql:"ssh_priv_key_path"`
	ContainerRestrictions    ContainerRestrictions `json:"containerRestrictions" gomysql:"container_restrictions"`
	IsPrivate                bool                  `json:"isPrivate" gomysql:"is_private"`
	PrivateLDAPAllowedGroups []string              `json:"privateLDAPAllowedGroups" gomysql:"private_ldap_allowed_groups"`
	NetworkCIDR              string                `json:"networkCIDR" gomysql:"network_cidr"`
	SetupPublicFolder        string                `json:"setupPublicFolder" gomysql:"setup_public_folder"`
	PackageStoragePath       string                `json:"packageStoragePath" gomysql:"package_storage_path"`
	ScoringActive            bool                  `json:"scoringActive" gomysql:"scoring_active"`
}

type CompetitionPackage struct {
	ID               int64     `json:"id" gomysql:"id,primary,increment"`
	CompetitionID    string    `json:"competitionID" gomysql:"competition_id,unique"`
	CompetitionName  string    `json:"competitionName" gomysql:"competition_name"`
	OriginalFilename string    `json:"originalFilename" gomysql:"original_filename"`
	StoragePath      string    `json:"storagePath" gomysql:"storage_path"`
	ConfigJSON       []byte    `json:"configJson" gomysql:"config_json"`
	AttachmentCount  int       `json:"attachmentCount" gomysql:"attachment_count"`
	CreatedAt        time.Time `json:"createdAt" gomysql:"created_at"`
}

type ScoringCheck struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PassPoints int    `json:"passPoints"`
	FailPoints int    `json:"failPoints"`
}

type TeamContainerConfig struct {
	Name           string         `json:"name"`
	LastOctetValue int            `json:"lastOctetValue"`
	SetupScript    []string       `json:"setupScript"`
	ScoringScript  []string       `json:"scoringScript"`
	ScoringSchema  []ScoringCheck `json:"scoringSchema"`
}

type CreateCompetitionRequest struct {
	CompetitionID          string `json:"competitionID"`
	CompetitionName        string `json:"competitionName"`
	CompetitionDescription string `json:"competitionDescription"`
	CompetitionHost        string `json:"competitionHost"`
	NumTeams               int    `json:"numTeams"`
	Privacy                struct {
		Public                  bool               `json:"public"`
		LDAPAllowedGroupsFilter flexibleStringList `json:"ldapAllowedGroupsFilter"`
	} `json:"privacy"`
	ContainerSpecs struct {
		TemplatePath   string      `json:"templatePath"`
		StoragePool    string      `json:"storagePool"`
		RootPassword   string      `json:"rootPassword"`
		StorageSizeGB  int         `json:"storageSizeGB"`
		MemoryMB       int         `json:"memoryMB"`
		Cores          int         `json:"cores"`
		GatewayIPv4    string      `json:"gatewayIPv4"`
		CIDRBlock      flexibleInt `json:"cidrBlock"`
		NameServerIPv4 string      `json:"nameServerIPv4"`
		SearchDomain   string      `json:"searchDomain"`
	} `json:"containerSpecs"`
	TeamContainerConfigs []TeamContainerConfig `json:"teamContainerConfigs"`
	SetupPublicFolder    string                `json:"setupPublicFolder"`
	WriteupFilePath      string                `json:"writeupFilePath"`
	AttachedFiles        []struct {
		SourceFilePath string `json:"sourceFilePath"`
		FileContent    []byte `json:"fileContent"`
	} `json:"attachedFiles"`
	PackagePath string `json:"-"`
}
