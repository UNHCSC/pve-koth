package app

import (
	"fmt"
	"sync"
	"time"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
)

type redeployJob struct {
	*streamJob
	containerIDs []int64
	startAfter   bool
}

var (
	redeployJobs   = map[string]*redeployJob{}
	redeployJobsMu sync.RWMutex
)

func newRedeployJob(user *auth.AuthUser, ids []int64, startAfter bool) *redeployJob {
	var job = &redeployJob{
		streamJob:    newStreamJob("redeploy_job", uploadActor(user)),
		containerIDs: append([]int64(nil), ids...),
		startAfter:   startAfter,
	}

	registerRedeployJob(job)
	return job
}

func registerRedeployJob(job *redeployJob) {
	redeployJobsMu.Lock()
	redeployJobs[job.ID] = job
	redeployJobsMu.Unlock()
}

func getRedeployJob(id string) *redeployJob {
	redeployJobsMu.RLock()
	defer redeployJobsMu.RUnlock()
	return redeployJobs[id]
}

func (job *redeployJob) canView(user *auth.AuthUser) bool {
	return job.Owner == uploadActor(user)
}

func (job *redeployJob) subscribe() chan string {
	return job.streamJob.subscribe()
}

func (job *redeployJob) unsubscribe(ch chan string) {
	job.streamJob.unsubscribe(ch)
}

func (job *redeployJob) markDone() {
	job.streamJob.markDone()
}

func (job *redeployJob) Status(message string) {
	job.logMessage(message)
}

func (job *redeployJob) Statusf(format string, args ...any) {
	job.logMessage(fmt.Sprintf(format, args...))
}

func (job *redeployJob) Errorf(format string, args ...any) {
	job.logMessage(fmt.Sprintf("ERROR: "+format, args...))
}

func (job *redeployJob) Successf(format string, args ...any) {
	job.logMessage(fmt.Sprintf(format, args...))
}

func startRedeployJob(job *redeployJob) {
	markContainersRedeploying(job.containerIDs)
	go func() {
		defer job.markDone()
		job.Statusf("Redeploy job started for containers: %v (start when finished: %t)", job.containerIDs, job.startAfter)
		if err := koth.RedeployContainersWithLogger(job.containerIDs, job, job.startAfter); err != nil {
			job.Errorf("Redeploy failed: %v", err)
		} else {
			job.Successf("Redeploy completed successfully")
		}

		if refreshErr := koth.RefreshContainerStatuses(job.containerIDs); refreshErr != nil {
			job.Errorf("failed to refresh container statuses: %v", refreshErr)
		}
	}()
}

func markContainersRedeploying(ids []int64) {
	if len(ids) == 0 {
		return
	}

	now := time.Now()
	for _, id := range ids {
		record, err := db.Containers.Select(id)
		if err != nil || record == nil {
			continue
		}
		record.Status = "redeploying"
		record.LastUpdated = now
		_ = db.Containers.Update(record)
	}
}
