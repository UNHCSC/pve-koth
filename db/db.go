package db

import (
	"github.com/UNHCSC/pve-koth/config"
	"github.com/z46-dev/gomysql"
)

var (
	Teams        *gomysql.RegisteredStruct[Team]
	Containers   *gomysql.RegisteredStruct[Container]
	Competitions *gomysql.RegisteredStruct[Competition]
)

func Init() (err error) {
	if err = gomysql.Begin(config.Config.Database.File); err != nil {
		return
	}

	if Teams, err = gomysql.Register(Team{}); err != nil {
		return
	}

	if Containers, err = gomysql.Register(Container{}); err != nil {
		return
	}

	if Competitions, err = gomysql.Register(Competition{}); err != nil {
		return
	}

	return
}
