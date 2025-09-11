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
	HostnamePrefix, RootPassword, Template, StoragePool, GatewayIPv4, Nameserver, SearchDomain string
	StorageGB, MemoryMB, Cores, IndividualCIDR                                                 int
}

type Competition struct {
	ID                    int64                 `json:"id" gomysql:"id,primary,increment"`
	Name                  string                `json:"name" gomysql:"name,unique"`
	TeamIDs               []int64               `json:"teamIDs" gomysql:"team_ids"`
	ContainerIDs          []int64               `json:"containerIDs" gomysql:"container_ids"`
	CreatedAt             time.Time             `json:"createdAt" gomysql:"created_at"`
	SSHPubKeyPath         string                `json:"sshKeyPath" gomysql:"ssh_key_path"`
	SSHPrivKeyPath        string                `json:"sshPrivKeyPath" gomysql:"ssh_priv_key_path"`
	ContainerRestrictions ContainerRestrictions `json:"containerRestrictions" gomysql:"container_restrictions"`
}
