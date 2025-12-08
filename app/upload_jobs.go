package app

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/UNHCSC/pve-koth/db"
	"github.com/UNHCSC/pve-koth/koth"
)

type uploadJob struct {
	ID        string
	Owner     string
	CreatedAt time.Time

	mu          sync.Mutex
	logs        []string
	status      string
	errorMsg    string
	errorDetail string
	listeners   map[chan string]struct{}
	done        bool
}

var (
	uploadJobs   = map[string]*uploadJob{}
	uploadJobsMu sync.RWMutex
)

func newUploadJob(user *auth.AuthUser) *uploadJob {
	var job = &uploadJob{
		ID:        fmt.Sprintf("job_%d", time.Now().UnixNano()),
		Owner:     uploadActor(user),
		CreatedAt: time.Now(),
		status:    "pending",
		logs:      []string{},
		listeners: make(map[chan string]struct{}),
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
	job.mu.Lock()
	job.logs = append(job.logs, message)
	for listener := range job.listeners {
		select {
		case listener <- message:
		default:
		}
	}
	job.mu.Unlock()
}

func (job *uploadJob) setStatus(status string) {
	job.mu.Lock()
	job.status = status
	job.mu.Unlock()
}

func (job *uploadJob) fail(message string, detail error) {
	job.mu.Lock()
	job.status = "failed"
	job.errorMsg = message
	if detail != nil {
		job.errorDetail = detail.Error()
	}
	job.done = true

	for listener := range job.listeners {
		close(listener)
		delete(job.listeners, listener)
	}
	job.mu.Unlock()
}

func (job *uploadJob) complete() {
	job.mu.Lock()
	job.status = "completed"
	job.done = true
	for listener := range job.listeners {
		close(listener)
		delete(job.listeners, listener)
	}
	job.mu.Unlock()
}

func (job *uploadJob) subscribe() chan string {
	job.mu.Lock()
	defer job.mu.Unlock()

	bufferSize := len(job.logs) + 16
	if bufferSize < 16 {
		bufferSize = 16
	}

	ch := make(chan string, bufferSize)
	for _, entry := range job.logs {
		ch <- entry
	}

	if job.done {
		close(ch)
	} else {
		job.listeners[ch] = struct{}{}
	}

	return ch
}

func (job *uploadJob) unsubscribe(ch chan string) {
	job.mu.Lock()
	if _, ok := job.listeners[ch]; ok {
		delete(job.listeners, ch)
		close(ch)
	}
	job.mu.Unlock()
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

func (job *uploadJob) summary() map[string]any {
	job.mu.Lock()
	defer job.mu.Unlock()

	return map[string]any{
		"id":          job.ID,
		"owner":       job.Owner,
		"createdAt":   job.CreatedAt,
		"status":      job.status,
		"error":       job.errorMsg,
		"errorDetail": job.errorDetail,
		"logCount":    len(job.logs),
	}
}

func sanitizeLogMessage(message string) string {
	return strings.ReplaceAll(message, "\n", " ")
}

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
