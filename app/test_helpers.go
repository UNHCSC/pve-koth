package app

import "fmt"

// NewStreamJobForTests exposes the internal stream job constructor to test suites.
func NewStreamJobForTests(prefix, owner string) *streamJob {
	return newStreamJob(prefix, owner)
}

// TestStreamJobSubscribe exposes the internal subscribe helper for tests.
func TestStreamJobSubscribe(job *streamJob) chan string {
	return job.subscribe()
}

// TestStreamJobUnsubscribe exposes the internal unsubscribe helper for tests.
func TestStreamJobUnsubscribe(job *streamJob, ch chan string) {
	job.unsubscribe(ch)
}

// TestStreamJobMarkDone finalizes the stream job in tests.
func TestStreamJobMarkDone(job *streamJob) {
	job.markDone()
}

// StreamJobStatus writes a status message to the stream job.
func StreamJobStatus(job *streamJob, message string) {
	job.logMessage(message)
}

// StreamJobStatusf formats a status message for the stream job.
func StreamJobStatusf(job *streamJob, format string, args ...any) {
	job.logMessage(fmt.Sprintf(format, args...))
}

// StreamJobSuccessf writes a success message to the stream job.
func StreamJobSuccessf(job *streamJob, format string, args ...any) {
	job.logMessage(fmt.Sprintf(format, args...))
}

// StreamJobErrorf writes an error message to the stream job.
func StreamJobErrorf(job *streamJob, format string, args ...any) {
	job.logMessage(fmt.Sprintf("ERROR: "+format, args...))
}

// NewTeardownJobForTests creates a teardown job without side effects for unit tests.
func NewTeardownJobForTests(owner, compID string) *teardownJob {
	job := &teardownJob{
		streamJob: newStreamJob("teardown_job", owner),
		compID:    compID,
	}
	registerTeardownJob(job)
	return job
}

// TestTeardownJobSubscribe exposes subscribe for teardown jobs.
func TestTeardownJobSubscribe(job *teardownJob) chan string {
	return job.subscribe()
}

// TestTeardownJobUnsubscribe exposes unsubscribe for teardown jobs.
func TestTeardownJobUnsubscribe(job *teardownJob, ch chan string) {
	job.unsubscribe(ch)
}

// TestTeardownJobMarkDone finalizes the teardown job stream.
func TestTeardownJobMarkDone(job *teardownJob) {
	job.markDone()
}

// TestTeardownJobRemove removes the job from the registry.
func TestTeardownJobRemove(job *teardownJob) {
	teardownJobsMu.Lock()
	delete(teardownJobs, job.ID)
	teardownJobsMu.Unlock()
}
