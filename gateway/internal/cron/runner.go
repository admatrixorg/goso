// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package cron

import (
	"context"
	"log"
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
		jobCtx, cancel := context.WithTimeout(ctx, JobTimeout)
		err := fire(jobCtx, j.SessionID, j.Message)
		cancel()
		if err != nil {
			log.Printf("goso cron: job %s: %v", j.ID, err)
			msg := err.Error()
			if len(msg) > 400 {
				msg = msg[:400]
			}
			_ = st.MarkCronError(j.ID, msg)
			continue
		}
		if err := st.MarkCronRun(j.ID, stampRun(now)); err != nil {
			log.Printf("goso cron: mark %s: %v", j.ID, err)
		}
	}
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
