package app

import (
	"fmt"
	"sync"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
)

type uploadJob struct {
	*streamJob

	status      string
	errorMsg    string
	errorDetail string
	metaMu      sync.Mutex
}

var (
	uploadJobs   = map[string]*uploadJob{}
	uploadJobsMu sync.RWMutex
)

func newUploadJob(user *auth.AuthUser) *uploadJob {
	var job = &uploadJob{
		streamJob: newStreamJob("job", uploadActor(user)),
		status:    "pending",
	}

	registerUploadJob(job)
	return job
}

func registerUploadJob(job *uploadJob) {
	uploadJobsMu.Lock()
	uploadJobs[job.ID] = job
	uploadJobsMu.Unlock()
}

func getUploadJob(id string) *uploadJob {
	uploadJobsMu.RLock()
	defer uploadJobsMu.RUnlock()
	return uploadJobs[id]
}

func (job *uploadJob) canView(user *auth.AuthUser) bool {
	return job.Owner == uploadActor(user)
}

func (job *uploadJob) appendLogs(logs []string) {
	for _, entry := range logs {
		job.log(entry)
	}
}

func (job *uploadJob) log(message string) {
	job.logMessage(message)
}

func (job *uploadJob) setStatus(status string) {
	job.metaMu.Lock()
	job.status = status
	job.metaMu.Unlock()
}

func (job *uploadJob) fail(message string, detail error) {
	job.metaMu.Lock()
	job.status = "failed"
	job.errorMsg = message
	if detail != nil {
		job.errorDetail = detail.Error()
	}
	job.metaMu.Unlock()
	job.markDone()
}

func (job *uploadJob) complete() {
	job.metaMu.Lock()
	job.status = "completed"
	job.metaMu.Unlock()
	job.markDone()
}

func (job *uploadJob) subscribe() chan string {
	return job.streamJob.subscribe()
}

func (job *uploadJob) unsubscribe(ch chan string) {
	job.streamJob.unsubscribe(ch)
}

// Implement koth.ProgressLogger

func (job *uploadJob) Status(message string) {
	job.log(message)
}

func (job *uploadJob) Statusf(format string, args ...any) {
	job.log(fmt.Sprintf(format, args...))
}

func (job *uploadJob) Errorf(format string, args ...any) {
	job.log(fmt.Sprintf("ERROR: "+format, args...))
}

func (job *uploadJob) Successf(format string, args ...any) {
	job.log(fmt.Sprintf(format, args...))
}

// func (job *uploadJob) summary() map[string]any {
// 	job.metaMu.Lock()
// 	defer job.metaMu.Unlock()

// 	return map[string]any{
// 		"id":          job.ID,
// 		"owner":       job.Owner,
// 		"createdAt":   job.CreatedAt,
// 		"status":      job.status,
// 		"error":       job.errorMsg,
// 		"errorDetail": job.errorDetail,
// 		"logCount":    job.logCount(),
// 	}
// }

func startProvisioningJob(job *uploadJob, req db.CreateCompetitionRequest) {
	go func() {
		job.setStatus("provisioning")
		job.log("provisioning job started")
		if _, err := koth.CreateNewCompWithLogger(&req, job); err != nil {
			job.log(fmt.Sprintf("Provisioning failed: %v", err))
			job.fail("provisioning failed", err)
			return
		}

		job.log("Provisioning completed successfully")
		job.complete()
	}()
}
