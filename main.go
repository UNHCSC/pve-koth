package main

import (
	"github.com/UNHCSC/pve-koth/app"
	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
	"github.com/z46-dev/go-logger"
)

var mainLog *logger.Logger = logger.NewLogger().SetPrefix("[KOTH]", logger.BoldBlue)

func main() {
	var err error

	if err = config.Init("config.toml"); err != nil {
		mainLog.Errorf("failed to initialize environment: %v\n", err)
		return
	}

	if err = db.Init(); err != nil {
		mainLog.Errorf("failed to initialize database: %v\n", err)
		return
	}

	if err = koth.Init(); err != nil {
		mainLog.Errorf("failed to initialize koth module: %v\n", err)
		return
	}

	koth.StartScoringLoop()

	mainLog.Errorf("fiber log: %v\n", app.StartApp())
}
