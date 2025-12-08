package db

import (
	"github.com/UNHCSC/pve-koth/config"
	"github.com/z46-dev/gomysql"
)

var (
	Teams               *gomysql.RegisteredStruct[Team]
	Containers          *gomysql.RegisteredStruct[Container]
	ScoreResults        *gomysql.RegisteredStruct[ScoreResult]
	Competitions        *gomysql.RegisteredStruct[Competition]
	CompetitionPackages *gomysql.RegisteredStruct[CompetitionPackage]
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

	if ScoreResults, err = gomysql.Register(ScoreResult{}); err != nil {
		return
	}

	if Competitions, err = gomysql.Register(Competition{}); err != nil {
		return
	}

	if CompetitionPackages, err = gomysql.Register(CompetitionPackage{}); err != nil {
		return
	}

	return
}

func Close() error {
	return gomysql.Close()
}

func DatabaseFilePath() string {
	return config.Config.Database.File
}

func GetCompetitionBySystemID(systemID string) (comp *Competition, err error) {
	var filter = gomysql.NewFilter().KeyCmp(Competitions.FieldBySQLName("competition_id"), gomysql.OpEqual, systemID)
	var results []*Competition
	if results, err = Competitions.SelectAllWithFilter(filter); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

func GetCompetitionPackageBySystemID(systemID string) (pkg *CompetitionPackage, err error) {
	var filter = gomysql.NewFilter().KeyCmp(CompetitionPackages.FieldBySQLName("competition_id"), gomysql.OpEqual, systemID)
	var results []*CompetitionPackage
	if results, err = CompetitionPackages.SelectAllWithFilter(filter); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}
