// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package cron

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// FireFunc posts chat into a session. Nil is treated as a no-op fire.
type FireFunc func(ctx context.Context, sessionID, message string) error

// JobTimeout bounds one scheduled chat so a hung provider cannot stall the ticker.
const JobTimeout = 45 * time.Second

func stampRun(now time.Time) time.Time {
	stamp := now.UTC()
	if wall := time.Now().UTC(); wall.After(stamp) {
		return wall
	}
	return stamp
}

// Tick fires every enabled due job. Empty job list is a no-op.
// Failed fires are logged and left unmarked so the next tick retries.
func Tick(ctx context.Context, st store.StoreIface, now time.Time, fire FireFunc) {
	if st == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Global on purpose: every tenant's due jobs run from this process.
	// HTTP listing is ListCronJobsTenant.
	jobs := st.ListCronJobs()
	if len(jobs) == 0 {
		return
	}
	if fire == nil {
		fire = func(context.Context, string, string) error { return nil }
	}
	now = now.UTC()
	for _, j := range jobs {
		if j == nil || !j.Enabled {
			continue
		}
		if !Due(j.Spec, j.LastRun, now) {
			continue
		}
		if !claim(j.ID) {
			// A manual run of this job is still going.
			continue
		}
		_ = runClaimed(ctx, st, j, now, fire)
	}
}

// ErrRunning is returned when a job is asked to run while it is running.
var ErrRunning = errors.New("job is already running")

// running holds the ids of jobs being fired, by the ticker or by hand.
var running sync.Map

func claim(id string) bool {
	_, busy := running.LoadOrStore(id, struct{}{})
	return !busy
}

// Running reports whether job id is being fired now.
func Running(id string) bool {
	_, busy := running.Load(id)
	return busy
}

// runClaimed fires one claimed job, records the outcome and releases it.
// A success stamps last_run, so the next scheduled run counts from now.
func runClaimed(ctx context.Context, st store.StoreIface, j *store.CronJob, now time.Time, fire FireFunc) error {
	defer running.Delete(j.ID)
	if fire == nil {
		fire = func(context.Context, string, string) error { return nil }
	}
	jobCtx, cancel := context.WithTimeout(WithJob(ctx, j.ID), JobTimeout)
	err := fire(jobCtx, j.SessionID, j.Message)
	cancel()
	if err != nil {
		log.Printf("goso cron: job %s: %v", j.ID, err)
		msg := err.Error()
		if len(msg) > 400 {
			msg = msg[:400]
		}
		_ = st.MarkCronError(j.ID, msg)
		return err
	}
	if err := st.MarkCronRun(j.ID, stampRun(now)); err != nil {
		log.Printf("goso cron: mark %s: %v", j.ID, err)
	}
	return nil
}

// RunJob fires job j now and waits for it, whatever its schedule and even
// when it is disabled. It returns ErrRunning when j is already being fired.
// The outcome is recorded as for a scheduled run.
func RunJob(ctx context.Context, st store.StoreIface, j *store.CronJob, fire FireFunc) error {
	if st == nil || j == nil || j.ID == "" {
		return errors.New("job not found")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !claim(j.ID) {
		return ErrRunning
	}
	return runClaimed(ctx, st, j, time.Now(), fire)
}

// StartJob is RunJob in the background: it returns once j is claimed (or
// ErrRunning) and fires it on its own clock, so the caller may finish first.
func StartJob(st store.StoreIface, j *store.CronJob, fire FireFunc) error {
	if st == nil || j == nil || j.ID == "" {
		return errors.New("job not found")
	}
	if !claim(j.ID) {
		return ErrRunning
	}
	go func() { _ = runClaimed(context.Background(), st, j, time.Now(), fire) }()
	return nil
}

type jobKey struct{}

// WithJob marks ctx as the turn of cron job id.
func WithJob(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, jobKey{}, id)
}

// JobFrom returns the cron job whose run started this turn, "" for none.
func JobFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(jobKey{}).(string)
	return id
}

// Loop runs Tick on a 1-minute ticker until ctx is done. Does not spawn OS cron.
func Loop(ctx context.Context, st store.StoreIface, fire FireFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			Tick(ctx, st, now, fire)
		}
	}
}
