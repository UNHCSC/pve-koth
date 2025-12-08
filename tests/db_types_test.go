package tests

import (
	"encoding/json"
	"testing"

	"github.com/UNHCSC/pve-koth/db"
)

func TestCreateCompetitionRequestLegacyFields(t *testing.T) {
	setup(t)
	defer cleanup(t)

	const payload = `{
		"competitionID": "legacy",
		"competitionName": "Legacy",
		"competitionDescription": "",
		"competitionHost": "",
		"numTeams": 1,
		"privacy": {
			"public": false,
			"LDAPAllowedGroupsFilter": "cn=admins,ou=groups,dc=example,dc=com"
		},
		"containerSpecs": {
			"templatePath": "tmpl",
			"storagePool": "pool",
			"rootPassword": "pass",
			"storageSizeGB": 8,
			"memoryMB": 512,
			"cores": 1,
			"gatewayIPv4": "10.0.0.1",
			"cidrBlock": "8",
			"nameServerIPv4": "10.0.0.2",
			"searchDomain": "example.lab"
		},
		"teamContainerConfigs": [],
		"setupPublicFolder": "public",
		"writeupFilePath": "writeup.pdf"
	}`

	var req db.CreateCompetitionRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("failed to parse legacy payload: %v", err)
	}

	if int(req.ContainerSpecs.CIDRBlock) != 8 {
		t.Fatalf("expected cidrBlock to parse as 8, got %d", req.ContainerSpecs.CIDRBlock)
	}

	groups := []string(req.Privacy.LDAPAllowedGroupsFilter)
	if len(groups) != 1 || groups[0] != "cn=admins,ou=groups,dc=example,dc=com" {
		t.Fatalf("expected single LDAP group, got %v", groups)
	}
}

func TestCreateCompetitionRequestModernFields(t *testing.T) {
	setup(t)
	defer cleanup(t)

	const payload = `{
		"competitionID": "modern",
		"competitionName": "Modern",
		"competitionDescription": "",
		"competitionHost": "",
		"numTeams": 1,
		"privacy": {
			"public": false,
			"ldapAllowedGroupsFilter": [
				"cn=admins,ou=groups,dc=example,dc=com",
				"cn=staff,ou=groups,dc=example,dc=com"
			]
		},
		"containerSpecs": {
			"templatePath": "tmpl",
			"storagePool": "pool",
			"rootPassword": "pass",
			"storageSizeGB": 8,
			"memoryMB": 512,
			"cores": 1,
			"gatewayIPv4": "10.0.0.1",
			"cidrBlock": 24,
			"nameServerIPv4": "10.0.0.2",
			"searchDomain": "example.lab"
		},
		"teamContainerConfigs": [],
		"setupPublicFolder": "public",
		"writeupFilePath": "writeup.pdf"
	}`

	var req db.CreateCompetitionRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("failed to parse modern payload: %v", err)
	}

	if int(req.ContainerSpecs.CIDRBlock) != 24 {
		t.Fatalf("expected cidrBlock to parse as 24, got %d", req.ContainerSpecs.CIDRBlock)
	}

	groups := []string(req.Privacy.LDAPAllowedGroupsFilter)
	if len(groups) != 2 {
		t.Fatalf("expected 2 LDAP groups, got %d", len(groups))
	}
}
