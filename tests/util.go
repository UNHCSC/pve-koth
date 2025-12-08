package tests

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
)

func setup(t *testing.T) {
	var err error

	if err = config.Init("../config.toml"); err != nil {
		t.Fatalf("Failed to load config.toml: %v", err)
	}
	// Randomize the database file name to avoid conflicts
	config.Config.Database.File = fmt.Sprintf("test_db_%d.db", time.Now().UnixNano())

	if err = db.Init(); err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
}

func cleanup(t *testing.T) {
	var err error

	if err = db.Close(); err != nil {
		t.Fatalf("Failed to close DB: %v", err)
	}

	if err = os.Remove(db.DatabaseFilePath()); err != nil {
		t.Fatalf("Failed to remove test database file: %v", err)
	}
}
