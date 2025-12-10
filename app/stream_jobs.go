package app

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type streamJob struct {
	ID        string
	Owner     string
	CreatedAt time.Time

	mu        sync.Mutex
	logs      []string
	listeners map[chan string]struct{}
	done      bool
}

func newStreamJob(prefix, owner string) *streamJob {
	var job = &streamJob{
		ID:        fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()),
		Owner:     owner,
		CreatedAt: time.Now(),
		logs:      []string{},
		listeners: make(map[chan string]struct{}),
	}
	return job
}

func (job *streamJob) logMessage(message string) {
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

func (job *streamJob) subscribe() chan string {
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

func (job *streamJob) unsubscribe(ch chan string) {
	job.mu.Lock()
	if _, ok := job.listeners[ch]; ok {
		delete(job.listeners, ch)
		close(ch)
	}
	job.mu.Unlock()
}

// func (job *streamJob) logCount() int {
// 	job.mu.Lock()
// 	defer job.mu.Unlock()
// 	return len(job.logs)
// }

func (job *streamJob) markDone() {
	job.mu.Lock()
	if job.done {
		job.mu.Unlock()
		return
	}
	job.done = true
	for listener := range job.listeners {
		close(listener)
		delete(job.listeners, listener)
	}
	job.mu.Unlock()
}

func sanitizeLogMessage(message string) string {
	return strings.ReplaceAll(message, "\n", " ")
}
