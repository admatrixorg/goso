// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package heartbeat is an application-level in-process stamp (not WebSocket ping/pong).
// It does not run HEARTBEAT.md, channel delivery, or goclaw tables. Cron (054) is unchanged.
package heartbeat

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/observe"
)

const (
	// MinInterval is the floor for GOSO_HEARTBEAT_INTERVAL_SEC (30s).
	MinInterval = 30 * time.Second
	// DefaultInterval is used when the env is unset or invalid (60s).
	DefaultInterval = 60 * time.Second
)

// Enabled reports GOSO_HEARTBEAT=1/true/yes/on. Default off.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(config.Lookup("GOSO_HEARTBEAT"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Interval is GOSO_HEARTBEAT_INTERVAL_SEC, default 60s, clamped to MinInterval.
func Interval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("GOSO_HEARTBEAT_INTERVAL_SEC"))
	if raw == "" {
		return DefaultInterval
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return DefaultInterval
	}
	d := time.Duration(n) * time.Second
	if d < MinInterval {
		return MinInterval
	}
	return d
}

// Tick stamps Observer last_heartbeat. Nil observer is a no-op.
func Tick(obs *observe.Observer) {
	if obs == nil {
		return
	}
	obs.RecordHeartbeat(time.Now().UTC())
}

// Loop runs Tick on Interval() until ctx is done. Does not spawn OS cron.
func Loop(ctx context.Context, obs *observe.Observer) {
	if ctx == nil {
		ctx = context.Background()
	}
	d := Interval()
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			Tick(obs)
		}
	}
}
