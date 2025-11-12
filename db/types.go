package db

import "time"

type Team struct {
	ID           int64     `json:"id" gomysql:"id,primary,increment"`
	Name         string    `json:"name" gomysql:"name,unique"`
	Score        int       `json:"score" gomysql:"score"`
	ContainerIDs []int64   `json:"containerIDs" gomysql:"container_ids"`
	LastUpdated  time.Time `json:"lastUpdated" gomysql:"last_updated"`
	CreatedAt    time.Time `json:"createdAt" gomysql:"created_at"`
}

type Container struct {
	ID          int64     `json:"id" gomysql:"id,primary,increment"`
	IPAddress   string    `json:"ipAddress" gomysql:"ip_address,unique"`
	Status      string    `json:"status" gomysql:"status"`
	LastUpdated time.Time `json:"lastUpdated" gomysql:"last_updated"`
	CreatedAt   time.Time `json:"createdAt" gomysql:"created_at"`
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
}

type CreateCompetitionRequest struct {
	CompetitionID          string `json:"competitionID"`
	CompetitionName        string `json:"competitionName"`
	CompetitionDescription string `json:"competitionDescription"`
	CompetitionHost        string `json:"competitionHost"`
	NumTeams               int    `json:"numTeams"`
	Privacy                struct {
		Public                  bool     `json:"public"`
		LDAPAllowedGroupsFilter []string `json:"ldapAllowedGroupsFilter"`
	} `json:"privacy"`
	ContainerSpecs struct {
		TemplatePath   string `json:"templatePath"`
		StoragePool    string `json:"storagePool"`
		RootPassword   string `json:"rootPassword"`
		StorageSizeGB  int    `json:"storageSizeGB"`
		MemoryMB       int    `json:"memoryMB"`
		Cores          int    `json:"cores"`
		GatewayIPv4    string `json:"gatewayIPv4"`
		CIDRBlock      int    `json:"cidrBlock"`
		NameServerIPv4 string `json:"nameServerIPv4"`
		SearchDomain   string `json:"searchDomain"`
	} `json:"containerSpecs"`
	TeamContainerConfigs []struct {
		Name           string   `json:"name"`
		LastOctetValue int      `json:"lastOctetValue"`
		SetupScript    []string `json:"setupScript"`
		ScoringScript  []string `json:"scoringScript"`
		ScoringSchema  []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			PassPoints int    `json:"passPoints"`
			FailPoints int    `json:"failPoints"`
		} `json:"scoringSchema"`
	} `json:"teamContainerConfigs"`
	SetupPublicFolder string `json:"setupPublicFolder"`
	WriteupFilePath   string `json:"writeupFilePath"`
	AttachedFiles     []struct {
		SourceFilePath string `json:"sourceFilePath"`
		FileContent    []byte `json:"fileContent"`
	} `json:"attachedFiles"`
}
