package tests

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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

	packageRoot := t.TempDir()
	archive, err := zip.OpenReader(filepath.Join("..", "examples", "mixed_vm_competition.zip"))
	require.NoError(t, err)
	defer archive.Close()
	for _, entry := range archive.File {
		name := filepath.Clean(entry.Name)
		require.False(t, filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)))
		target := filepath.Join(packageRoot, name)
		if entry.FileInfo().IsDir() {
			require.NoError(t, os.MkdirAll(target, 0o755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		source, openErr := entry.Open()
		require.NoError(t, openErr)
		destination, createErr := os.Create(target)
		require.NoError(t, createErr)
		_, copyErr := io.Copy(destination, source)
		require.NoError(t, source.Close())
		require.NoError(t, destination.Close())
		require.NoError(t, copyErr)
	}
	raw, err := os.ReadFile(filepath.Join(packageRoot, "config.json"))
	require.NoError(t, err)

	var request db.CreateCompetitionRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	suffix := time.Now().Unix()
	request.CompetitionID = fmt.Sprintf("mixedGuests%d", suffix)
	request.CompetitionName = fmt.Sprintf("Mixed Guest Test %d", suffix)
	request.PackagePath = packageRoot
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
	require.Len(t, comp.ContainerIDs, 6)

	records := make(map[db.GuestKind]int)
	for _, id := range comp.ContainerIDs {
		record, recordErr := db.Containers.Select(id)
		require.NoError(t, recordErr)
		require.NotNil(t, record)
		records[record.GuestKind]++
	}
	assert.Equal(t, 3, records[db.GuestKindLXC])
	assert.Equal(t, 3, records[db.GuestKindQEMU])

	require.NoError(t, koth.BulkStartContainers(comp.ContainerIDs))
	comp.ScoringActive = true
	require.NoError(t, db.Competitions.Update(comp))
	require.Eventually(t, func() bool {
		if scoreErr := koth.ScoreCompetitionNow(comp); scoreErr != nil {
			t.Logf("score competition: %v", scoreErr)
			return false
		}
		allPassed := true
		for _, teamID := range comp.TeamIDs {
			team, teamErr := db.Teams.Select(teamID)
			if teamErr != nil || team == nil || team.Score != 13 {
				if team != nil {
					t.Logf("team %d score: %d", teamID, team.Score)
				}
				allPassed = false
			}
		}
		return allPassed
	}, 3*time.Minute, 10*time.Second)
}
