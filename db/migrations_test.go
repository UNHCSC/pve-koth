package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateDatabaseAddsGuestMetadataToLegacyContainerTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite", path)
	require.NoError(t, err)

	_, err = database.Exec(`CREATE TABLE "Container" (id INTEGER PRIMARY KEY, status TEXT)`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO "Container" (id, status) VALUES (101, 'stopped')`)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	require.NoError(t, migrateDatabase(path))
	require.NoError(t, migrateDatabase(path), "migration must be idempotent")

	database, err = sql.Open("sqlite", path)
	require.NoError(t, err)
	defer database.Close()

	var kind, osType, shell, templateRef string
	var templateVMID int
	err = database.QueryRow(`SELECT guest_kind, guest_os, script_shell, template_ref, template_vmid FROM "Container" WHERE id = 101`).Scan(&kind, &osType, &shell, &templateRef, &templateVMID)
	require.NoError(t, err)
	assert.Equal(t, string(GuestKindLXC), kind)
	assert.Equal(t, string(GuestOSLinux), osType)
	assert.Equal(t, string(ScriptShellBash), shell)
	assert.Empty(t, templateRef)
	assert.Zero(t, templateVMID)
}

func TestMigrateDatabaseAllowsFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	require.NoError(t, migrateDatabase(path))
}
