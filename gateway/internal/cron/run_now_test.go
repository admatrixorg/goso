// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func newJob(t *testing.T, st *store.Store, enabled bool) *store.CronJob {
	t.Helper()
	a, err := st.CreateAgent(store.Agent{AgentKey: "cron-run", DisplayName: "Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.CreateCronJob(store.CronJob{Spec: "every:5m", SessionID: sess.ID, Message: "báo cáo sáng", Enabled: enabled})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestRunJobFiresDisabledJobAndStamps(t *testing.T) {
	st := store.New()
	job := newJob(t, st, false)
	var gotJob, gotMsg string
	err := RunJob(context.Background(), st, job, func(ctx context.Context, sid, msg string) error {
		gotJob, gotMsg = JobFrom(ctx), msg
		if _, ok := ctx.Deadline(); !ok {
			t.Error("no timeout on the run")
		}
		return nil
	})
	if err != nil || gotJob != job.ID || gotMsg != "báo cáo sáng" {
		t.Fatalf("err %v job %q msg %q", err, gotJob, gotMsg)
	}
	fresh, _ := st.GetCronJob(job.ID)
	if fresh.LastRun == nil || time.Since(*fresh.LastRun) > time.Minute || fresh.LastError != "" {
		t.Fatalf("not stamped %+v", fresh)
	}
	if Running(job.ID) {
		t.Fatal("still marked running")
	}
}

func TestRunJobRecordsFailure(t *testing.T) {
	st := store.New()
	job := newJob(t, st, true)
	err := RunJob(context.Background(), st, job, func(context.Context, string, string) error { return errors.New("provider down") })
	fresh, _ := st.GetCronJob(job.ID)
	if err == nil || fresh.LastError == "" || fresh.LastRun != nil {
		t.Fatalf("err %v job %+v", err, fresh)
	}
}

func TestRunJobRefusesConcurrentRun(t *testing.T) {
	st := store.New()
	job := newJob(t, st, true)
	release := make(chan struct{})
	started := make(chan struct{})
	if err := StartJob(st, job, func(context.Context, string, string) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := RunJob(context.Background(), st, job, nil); !errors.Is(err, ErrRunning) {
		t.Fatalf("second run: %v", err)
	}
	if err := StartJob(st, job, nil); !errors.Is(err, ErrRunning) {
		t.Fatalf("second start: %v", err)
	}
	// The ticker skips it too.
	fired := 0
	Tick(context.Background(), st, time.Now().Add(48*time.Hour), func(context.Context, string, string) error { fired++; return nil })
	if fired != 0 {
		t.Fatalf("tick fired a running job")
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for Running(job.ID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if Running(job.ID) {
		t.Fatal("never released")
	}
}

func TestJobFromEmpty(t *testing.T) {
	if JobFrom(context.Background()) != "" {
		t.Fatal("empty ctx has a job")
	}
}
