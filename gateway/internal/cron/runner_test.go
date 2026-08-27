// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestTick_EmptyNoop(t *testing.T) {
	st := store.New()
	var n atomic.Int32
	Tick(context.Background(), st, time.Now().UTC(), func(context.Context, string, string) error {
		n.Add(1)
		return nil
	})
	if n.Load() != 0 {
		t.Fatalf("empty list fired %d", n.Load())
	}
}

func TestTick_DueIntervalFiresOnce(t *testing.T) {
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "cron", DisplayName: "C"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.CreateCronJob(store.CronJob{
		Spec: "every:1m", SessionID: sess.ID, Message: "ping", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	fire := func(context.Context, string, string) error {
		n.Add(1)
		return nil
	}
	Tick(context.Background(), st, now, fire)
	if n.Load() != 1 {
		t.Fatalf("first tick %d", n.Load())
	}
	got, _ := st.GetCronJob(job.ID)
	if got.LastRun == nil {
		t.Fatal("last_run not marked")
	}
	Tick(context.Background(), st, now.Add(30*time.Second), fire)
	if n.Load() != 1 {
		t.Fatalf("too soon %d", n.Load())
	}
	Tick(context.Background(), st, now.Add(time.Minute), fire)
	if n.Load() != 2 {
		t.Fatalf("second tick %d", n.Load())
	}
}

func TestTick_FireErrorDoesNotMark(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "err", DisplayName: "E"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	job, err := st.CreateCronJob(store.CronJob{
		Spec: "every:1m", SessionID: sess.ID, Message: "x", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	var n atomic.Int32
	fail := func(context.Context, string, string) error {
		n.Add(1)
		return context.DeadlineExceeded
	}
	Tick(context.Background(), st, now, fail)
	got, _ := st.GetCronJob(job.ID)
	if got.LastRun != nil {
		t.Fatalf("failed fire must not mark last_run %+v", got.LastRun)
	}
	Tick(context.Background(), st, now.Add(time.Second), fail)
	if n.Load() != 2 {
		t.Fatalf("retry %d", n.Load())
	}
}

func TestLoop_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Loop(ctx, store.New(), nil)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop did not stop")
	}
}

func TestTick_DisabledSkipped(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "d", DisplayName: "D"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	_, err := st.CreateCronJob(store.CronJob{
		Spec: "every:1m", SessionID: sess.ID, Message: "no", Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	Tick(context.Background(), st, time.Now().UTC(), func(context.Context, string, string) error {
		n.Add(1)
		return nil
	})
	if n.Load() != 0 {
		t.Fatalf("disabled fired %d", n.Load())
	}
}
