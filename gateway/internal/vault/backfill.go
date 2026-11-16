// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// BackfillMax caps one backfill request.
const BackfillMax = 1000

// BackfillRun is the state of the last background backfill of one tenant.
type BackfillRun struct {
	Running    bool           `json:"running"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Result     BackfillResult `json:"result"`
}

var (
	backfillMu   sync.Mutex
	backfillRuns = map[string]*BackfillRun{}
)

// StartBackfill runs Backfill for tenant in the background. It returns false
// when one is already running for that tenant.
func (ix *Indexer) StartBackfill(tenant string, limit int) bool {
	if ix == nil {
		return false
	}
	tenant = store.NormalizeTenant(tenant)
	backfillMu.Lock()
	if run := backfillRuns[tenant]; run != nil && run.Running {
		backfillMu.Unlock()
		return false
	}
	run := &BackfillRun{Running: true, StartedAt: time.Now().UTC()}
	backfillRuns[tenant] = run
	backfillMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		res, _ := ix.Backfill(ctx, tenant, limit)
		now := time.Now().UTC()
		backfillMu.Lock()
		run.Running = false
		run.FinishedAt = &now
		run.Result = res
		backfillMu.Unlock()
	}()
	return true
}

// LastBackfill is a copy of the tenant's last background run, or nil.
func LastBackfill(tenant string) *BackfillRun {
	backfillMu.Lock()
	defer backfillMu.Unlock()
	run := backfillRuns[store.NormalizeTenant(tenant)]
	if run == nil {
		return nil
	}
	cp := *run
	return &cp
}
