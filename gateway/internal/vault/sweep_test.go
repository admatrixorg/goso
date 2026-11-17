// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSweepDrainsBacklogThenWaits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	calls := 0
	results := []BackfillResult{
		{Embedded: 1000, Remain: true},
		{Embedded: 600},          // backlog drained: wait for the interval
		{Embedded: 3, Failed: 1}, // next interval
		{}, {}, {},
	}
	errs := []error{nil, nil, errors.New("quota"), nil, nil, nil}
	done := make(chan struct{})
	go func() {
		sweep(ctx, time.Millisecond, 40*time.Millisecond, func(context.Context) (BackfillResult, error) {
			mu.Lock()
			defer mu.Unlock()
			i := calls
			calls++
			if i >= len(results)-1 {
				select {
				case <-done:
				default:
					close(done)
				}
				return BackfillResult{}, nil
			}
			return results[i], errs[i]
		})
	}()
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if calls != 2 {
		mu.Unlock()
		t.Fatalf("after first pass calls=%d, want 2 (drain until no Remain, then wait)", calls)
	}
	mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not run again after the interval")
	}
	cancel()
}

func TestSweepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		sweep(ctx, time.Hour, time.Hour, func(context.Context) (BackfillResult, error) {
			t.Error("backfill ran before the first delay")
			return BackfillResult{}, nil
		})
		close(stopped)
	}()
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("sweep did not stop on cancel")
	}
}
