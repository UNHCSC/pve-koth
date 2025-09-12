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
	Name                     string                `json:"name" gomysql:"name,unique"`
	TeamIDs                  []int64               `json:"teamIDs" gomysql:"team_ids"`
	ContainerIDs             []int64               `json:"containerIDs" gomysql:"container_ids"`
	CreatedAt                time.Time             `json:"createdAt" gomysql:"created_at"`
	SSHPubKeyPath            string                `json:"sshPubKeyPath" gomysql:"ssh_pub_key_path"`
	SSHPrivKeyPath           string                `json:"sshPrivKeyPath" gomysql:"ssh_priv_key_path"`
	ContainerRestrictions    ContainerRestrictions `json:"containerRestrictions" gomysql:"container_restrictions"`
	IsPrivate                bool                  `json:"isPrivate" gomysql:"is_private"`
	PrivateLDAPAllowedGroups []string              `json:"privateLDAPAllowedGroups" gomysql:"private_ldap_allowed_groups"`
}
