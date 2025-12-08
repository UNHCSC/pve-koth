package koth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/z46-dev/go-logger"
	"github.com/z46-dev/gomysql"
)

func TeardownCompetition(comp *db.Competition) error {
	return TeardownCompetitionWithLogger(comp, nil)
}

func TeardownCompetitionWithLogger(comp *db.Competition, logSink ProgressLogger) error {
	if comp == nil {
		return fmt.Errorf("competition is nil")
	}

	var log ProgressLogger
	if logSink != nil {
		log = logSink
	} else {
		log = logger.NewLogger().SetPrefix(fmt.Sprintf("[TEARDOWN %s]", comp.SystemID), logger.BoldRed).IncludeTimestamp()
	}

	if api == nil {
		return fmt.Errorf("proxmox API is not initialized")
	}

	log.Statusf("Destroying competition: %s\n", comp.Name)
	var combinedErr error

	if err := stopAndDeleteContainers(comp, log); err != nil {
		combinedErr = errors.Join(combinedErr, err)
	}

	if err := purgeContainerRecords(comp, log); err != nil {
		combinedErr = errors.Join(combinedErr, err)
	}

	if err := purgeTeamRecords(comp, log); err != nil {
		combinedErr = errors.Join(combinedErr, err)
	}

	if err := purgeScoreResults(comp, log); err != nil {
		combinedErr = errors.Join(combinedErr, err)
	}

	if err := db.Competitions.Delete(comp.ID); err != nil {
		log.Errorf("Failed to delete competition record %d: %v\n", comp.ID, err)
		combinedErr = errors.Join(combinedErr, err)
	}

	if err := removeCompetitionData(comp, log); err != nil {
		combinedErr = errors.Join(combinedErr, err)
	}

	if err := removeCompetitionPackage(comp, log); err != nil {
		combinedErr = errors.Join(combinedErr, err)
	}

	if combinedErr == nil {
		log.Successf("Competition %s torn down successfully.\n", comp.SystemID)
	}

	return combinedErr
}

func stopAndDeleteContainers(comp *db.Competition, log ProgressLogger) error {
	if len(comp.ContainerIDs) == 0 {
		log.Status("No containers recorded; skipping stop/delete.")
		return nil
	}

	var ids []int
	for _, id := range comp.ContainerIDs {
		ids = append(ids, int(id))
	}

	log.Status("Stopping containers...")
	if err := api.BulkCTActionWithRetries(api.BulkStop, ids, 1+len(ids)/4); err != nil {
		log.Errorf("Failed to stop containers: %v\n", err)
		return err
	}

	log.Status("Deleting containers...")
	if err := api.BulkCTActionWithRetries(api.BulkDelete, ids, 1+len(ids)/4); err != nil {
		log.Errorf("Failed to delete containers: %v\n", err)
		return err
	}

	return nil
}

func purgeContainerRecords(comp *db.Competition, log ProgressLogger) error {
	var combined error
	for _, id := range comp.ContainerIDs {
		if err := db.Containers.Delete(id); err != nil {
			log.Errorf("Failed to remove container record %d: %v\n", id, err)
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func purgeTeamRecords(comp *db.Competition, log ProgressLogger) error {
	var combined error
	for _, teamID := range comp.TeamIDs {
		if err := db.Teams.Delete(teamID); err != nil {
			log.Errorf("Failed to remove team record %d: %v\n", teamID, err)
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func purgeScoreResults(comp *db.Competition, log ProgressLogger) error {
	var combined error
	for _, teamID := range comp.TeamIDs {
		filter := gomysql.NewFilter().KeyCmp(db.ScoreResults.FieldBySQLName("team_id"), gomysql.OpEqual, teamID)
		results, err := db.ScoreResults.SelectAllWithFilter(filter)
		if err != nil {
			log.Errorf("Failed to load score results for team %d: %v\n", teamID, err)
			combined = errors.Join(combined, err)
			continue
		}
		for _, record := range results {
			if err := db.ScoreResults.Delete(record.ID); err != nil {
				log.Errorf("Failed to delete score result %d: %v\n", record.ID, err)
				combined = errors.Join(combined, err)
			}
		}
	}
	return combined
}

func removeCompetitionData(comp *db.Competition, log ProgressLogger) error {
	if comp.SystemID == "" {
		return nil
	}

	target := filepath.Join(config.StorageBasePath(), "competitions", comp.SystemID)
	if err := os.RemoveAll(target); err != nil {
		log.Errorf("Failed to remove competition data at %s: %v\n", target, err)
		return err
	}

	log.Statusf("Removed competition data directory %s\n", target)
	return nil
}

func removeCompetitionPackage(comp *db.Competition, log ProgressLogger) error {
	var combined error
	if comp.PackageStoragePath != "" {
		if err := os.RemoveAll(comp.PackageStoragePath); err != nil {
			log.Errorf("Failed to remove package directory %s: %v\n", comp.PackageStoragePath, err)
			combined = errors.Join(combined, err)
		} else {
			log.Statusf("Removed package directory %s\n", comp.PackageStoragePath)
		}
	}

	if comp.SystemID != "" {
		if pkg, err := db.GetCompetitionPackageBySystemID(comp.SystemID); err != nil {
			log.Errorf("Failed to load package record for %s: %v\n", comp.SystemID, err)
			combined = errors.Join(combined, err)
		} else if pkg != nil {
			if err := db.CompetitionPackages.Delete(pkg.ID); err != nil {
				log.Errorf("Failed to remove package record %d: %v\n", pkg.ID, err)
				combined = errors.Join(combined, err)
			} else {
				log.Statusf("Removed package record %d for %s\n", pkg.ID, comp.SystemID)
			}
		}
	}

	return combined
}
