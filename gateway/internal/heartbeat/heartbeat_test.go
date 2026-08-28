// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package heartbeat

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/observe"
)

func TestEnabled_DefaultOff(t *testing.T) {
	t.Setenv("GOSO_HEARTBEAT", "")
	if Enabled() {
		t.Fatal("default must be off")
	}
	t.Setenv("GOSO_HEARTBEAT", "1")
	if !Enabled() {
		t.Fatal("1 must enable")
	}
}

func TestInterval_Min30s(t *testing.T) {
	t.Setenv("GOSO_HEARTBEAT_INTERVAL_SEC", "")
	if Interval() != DefaultInterval {
		t.Fatalf("default %s", Interval())
	}
	t.Setenv("GOSO_HEARTBEAT_INTERVAL_SEC", "10")
	if Interval() != MinInterval {
		t.Fatalf("clamp %s", Interval())
	}
	t.Setenv("GOSO_HEARTBEAT_INTERVAL_SEC", "90")
	if Interval() != 90*time.Second {
		t.Fatalf("90s %s", Interval())
	}
	t.Setenv("GOSO_HEARTBEAT_INTERVAL_SEC", "nope")
	if Interval() != DefaultInterval {
		t.Fatalf("invalid %s", Interval())
	}
}

func TestTick_RecordsLastHeartbeat(t *testing.T) {
	obs := observe.NewWithWriter(&bytes.Buffer{})
	if obs.LastHeartbeat() != "" {
		t.Fatal("never fired")
	}
	Tick(obs)
	got := obs.LastHeartbeat()
	if got == "" {
		t.Fatal("expected stamp")
	}
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("rfc3339 %q %v", got, err)
	}
	Tick(nil)
}

func TestLoop_StopsOnCancel(t *testing.T) {
	obs := observe.NewWithWriter(&bytes.Buffer{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Loop(ctx, obs)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop did not stop")
	}
}
