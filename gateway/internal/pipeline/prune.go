// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Episodic TTL (SPEC 132). GOSO_MEMORY_TTL_DAYS, default 90; 0 disables.
const (
	PruneDefaultDays = 90
	pruneInterval    = 6 * time.Hour
	// pruneStartDelay keeps the first sweep off the boot path, where the
	// SQLite file is still busy with schema/startup writers.
	pruneStartDelay = time.Minute
)

// PruneTTL returns the configured episodic retention; 0 means off.
func PruneTTL() time.Duration {
	v := strings.TrimSpace(config.Lookup("GOSO_MEMORY_TTL_DAYS"))
	if v == "" {
		return PruneDefaultDays * 24 * time.Hour
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return PruneDefaultDays * 24 * time.Hour
	}
	return time.Duration(n) * 24 * time.Hour
}

// PruneOnce deletes episodic notes older than the TTL. Returns rows removed.
func PruneOnce(st store.StoreIface) (int, error) {
	ttl := PruneTTL()
	if st == nil || ttl <= 0 {
		return 0, nil
	}
	return st.PruneEpisodic(time.Now().UTC().Add(-ttl))
}

// PruneLoop runs PruneOnce at start and every pruneInterval until ctx ends.
func PruneLoop(ctx context.Context, st store.StoreIface) {
	if st == nil || PruneTTL() <= 0 {
		return
	}
	run := func() {
		n, err := PruneOnce(st)
		if err != nil {
			log.Printf("goso memory: prune error: %v", err)
			return
		}
		if n > 0 {
			log.Printf("goso memory: pruned %d episodic notes older than %s", n, PruneTTL())
		}
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(pruneStartDelay):
	}
	run()
	t := time.NewTicker(pruneInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
