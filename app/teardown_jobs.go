package app

import (
	"fmt"
	"sync"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/koth"
)

type teardownJob struct {
	*streamJob
	compID string
}

var (
	teardownJobs   = map[string]*teardownJob{}
	teardownJobsMu sync.RWMutex
)

func newTeardownJob(user *auth.AuthUser, compID string) *teardownJob {
	job := &teardownJob{
		streamJob: newStreamJob("teardown_job", uploadActor(user)),
		compID:    compID,
	}
	registerTeardownJob(job)
	return job
}

func registerTeardownJob(job *teardownJob) {
	teardownJobsMu.Lock()
	teardownJobs[job.ID] = job
	teardownJobsMu.Unlock()
}

func getTeardownJob(id string) *teardownJob {
	teardownJobsMu.RLock()
	defer teardownJobsMu.RUnlock()
	return teardownJobs[id]
}

func (job *teardownJob) canView(user *auth.AuthUser) bool {
	return job.Owner == uploadActor(user)
}

func (job *teardownJob) subscribe() chan string {
	return job.streamJob.subscribe()
}

func (job *teardownJob) unsubscribe(ch chan string) {
	job.streamJob.unsubscribe(ch)
}

func (job *teardownJob) Status(message string) {
	job.logMessage(message)
}

func (job *teardownJob) Statusf(format string, args ...any) {
	job.logMessage(fmt.Sprintf(format, args...))
}

func (job *teardownJob) Errorf(format string, args ...any) {
	job.logMessage(fmt.Sprintf("ERROR: "+format, args...))
}

func (job *teardownJob) Successf(format string, args ...any) {
	job.logMessage(fmt.Sprintf(format, args...))
}

func startTeardownJob(job *teardownJob) {
	go func() {
		defer job.markDone()
		job.Statusf("Teardown job started for competition %s", job.compID)
		appLog.Basicf("teardown[%s] job %s started for %s\n", job.Owner, job.ID, job.compID)

		comp, err := loadCompetitionByIdentifier(job.compID)
		if err != nil {
			job.Errorf("Failed to resolve competition %s: %v", job.compID, err)
			appLog.Errorf("teardown[%s] failed to resolve competition %s: %v\n", job.Owner, job.compID, err)
			return
		}
		if comp == nil {
			job.Errorf("Competition %s not found", job.compID)
			return
		}

		if err := koth.TeardownCompetitionWithLogger(comp, job); err != nil {
			job.Errorf("Teardown failed: %v", err)
			appLog.Errorf("teardown[%s] job %s failed: %v\n", job.Owner, job.ID, err)
			return
		}

		job.Successf("Competition %s torn down successfully.", comp.SystemID)
		appLog.Basicf("teardown[%s] job %s completed for %s\n", job.Owner, job.ID, job.compID)
	}()
}
