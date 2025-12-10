package tests

import (
	"testing"
	"time"

	"github.com/UNHCSC/pve-koth/app"
	"github.com/stretchr/testify/require"
)

func TestStreamJobReplaysLogsAndCloses(t *testing.T) {
	job := app.NewStreamJobForTests("stream_test", "tester")
	defer app.TestStreamJobMarkDone(job)

	app.StreamJobStatus(job, "first message")
	app.StreamJobStatusf(job, "second message: %d", 2)

	ch := app.TestStreamJobSubscribe(job)
	defer app.TestStreamJobUnsubscribe(job, ch)

	readLog(t, ch, "first message")
	readLog(t, ch, "second message: 2")

	app.StreamJobSuccessf(job, "completed")
	readLog(t, ch, "completed")

	app.TestStreamJobMarkDone(job)
	select {
	case _, ok := <-ch:
		require.False(t, ok, "channel should close after mark done")
	case <-time.After(time.Second):
		t.Fatalf("expected stream channel to close after job completion")
	}
}

func TestTeardownJobStreamPublishesStatus(t *testing.T) {
	job := app.NewTeardownJobForTests("tester", "comp-123")
	ch := app.TestTeardownJobSubscribe(job)
	defer func() {
		app.TestTeardownJobUnsubscribe(job, ch)
		app.TestTeardownJobRemove(job)
	}()

	job.Status("beginning teardown")
	readLog(t, ch, "beginning teardown")

	job.Errorf("failed to stop container %s", "ct-1")
	readLog(t, ch, "ERROR: failed to stop container ct-1")

	job.Successf("teardown complete for %s", "comp-123")
	readLog(t, ch, "teardown complete for comp-123")
}

func readLog(t *testing.T, ch <-chan string, expect string) {
	select {
	case msg := <-ch:
		require.Equal(t, expect, msg)
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for log %q", expect)
	}
}
