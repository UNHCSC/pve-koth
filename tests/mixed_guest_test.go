package tests

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/UNHCSC/pve-koth/app"
	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
	"github.com/UNHCSC/pve-koth/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMixedLXCAndWindowsCompetition(t *testing.T) {
	setup(t)
	defer cleanup(t)
	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing environment is not enabled; skipping test")
	}

	source := filepath.Join("..", "examples", "mixed_vm_competition")
	packageRoot := t.TempDir()
	require.NoError(t, os.CopyFS(packageRoot, os.DirFS(source)))
	raw, err := os.ReadFile(filepath.Join(packageRoot, "config.json"))
	require.NoError(t, err)

	var request db.CreateCompetitionRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	suffix := time.Now().Unix()
	request.CompetitionID = fmt.Sprintf("mixedGuests%d", suffix)
	request.CompetitionName = fmt.Sprintf("Mixed Guest Test %d", suffix)
	request.PackagePath = packageRoot
	request.NumTeams = 1
	request.EnableAdvancedLogging = false

	listener, err := net.Listen("tcp", "0.0.0.0:8080")
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	baseURL := "http://" + net.JoinHostPort(ssh.MustLocalIP(), port)
	config.Config.WebServer.PublicURL = baseURL
	webApp := app.CreateApp()
	go func() {
		_ = webApp.Listener(listener)
	}()
	t.Cleanup(func() { _ = webApp.Shutdown() })

	require.NoError(t, koth.Init())
	comp, createErr := koth.CreateNewComp(&request)
	require.NoError(t, createErr)
	require.NotNil(t, comp)
	defer func() {
		if err := koth.TeardownCompetition(comp); err != nil {
			t.Errorf("teardown mixed competition: %v", err)
		}
	}()
	require.Len(t, comp.ContainerIDs, 2)

	records := make(map[db.GuestKind]int)
	for _, id := range comp.ContainerIDs {
		record, recordErr := db.Containers.Select(id)
		require.NoError(t, recordErr)
		require.NotNil(t, record)
		records[record.GuestKind]++
	}
	assert.Equal(t, 1, records[db.GuestKindLXC])
	assert.Equal(t, 1, records[db.GuestKindQEMU])

	require.NoError(t, koth.BulkStartContainers(comp.ContainerIDs))
	comp.ScoringActive = true
	require.NoError(t, db.Competitions.Update(comp))
	var team *db.Team
	require.Eventually(t, func() bool {
		if scoreErr := koth.ScoreCompetitionNow(comp); scoreErr != nil {
			return false
		}
		team, err = db.Teams.Select(comp.TeamIDs[0])
		return err == nil && team != nil && team.Score == 13
	}, 90*time.Second, 10*time.Second)
	require.NotNil(t, team)
	assert.Equal(t, 13, team.Score)
}
