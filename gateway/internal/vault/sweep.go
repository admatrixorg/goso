// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"log"
	"time"
)

// Docs written straight to the database (importers, a restore) never reach
// Notify, and the Notify queue does not survive a restart. The sweep embeds
// that backlog without anyone calling the backfill API.
var (
	sweepFirstDelay = time.Minute
	sweepInterval   = 30 * time.Minute
)

// sweep runs backfill for every tenant after firstDelay, keeps going while a
// run reports more work, then waits interval and runs again. An error waits
// for the next interval so a bad key or quota does not loop.
func sweep(ctx context.Context, firstDelay, interval time.Duration, backfill func(context.Context) (BackfillResult, error)) {
	wait := firstDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		total := 0
		for {
			res, err := backfill(ctx)
			total += res.Embedded
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("vault embeddings sweep: embedded=%d stopped: %v", total, err)
				}
				break
			}
			if !res.Remain || res.Embedded == 0 {
				break
			}
		}
		if total > 0 {
			log.Printf("vault embeddings sweep: embedded=%d", total)
		}
		wait = interval
	}
}
