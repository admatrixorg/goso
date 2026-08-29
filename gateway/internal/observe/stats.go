// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Stats is a JSON snapshot of process counters (SPEC 008 AC-03).
// LastHeartbeat is omitted until POST /api/system/heartbeat or the optional ticker fires.
type Stats struct {
	UptimeSeconds int64  `json:"uptime_seconds"`
	StartedAt     string `json:"started_at"`
	RequestCount  int64  `json:"request_count"`
	LLMCallCount  int64  `json:"llm_call_count"`
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
	WsUp          bool   `json:"ws_up"`
}

// Snapshot returns current uptime and counters.
func (o *Observer) Snapshot() Stats {
	now := time.Now().UTC()
	started := o.started
	if started.IsZero() {
		started = now
	}
	sec := int64(now.Sub(started).Seconds())
	if sec < 0 {
		sec = 0
	}
	return Stats{
		UptimeSeconds: sec,
		StartedAt:     started.Format(time.RFC3339),
		RequestCount:  o.reqs.Load(),
		LLMCallCount:  o.llms.Load(),
		LastHeartbeat: o.LastHeartbeat(),
		WsUp:          o.WsUp(),
	}
}

// HandleStats serves GET /api/stats as JSON.
func (o *Observer) HandleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(o.Snapshot())
}

// HandleHeartbeat serves POST /api/system/heartbeat. Request body is ignored (idempotent no-op).
func (o *Observer) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if o == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "observer required"})
		return
	}
	o.RecordHeartbeat(time.Now().UTC())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "last_heartbeat": o.LastHeartbeat()})
}

// HandleMetrics serves GET /metrics as Prometheus text exposition (minimal).
func (o *Observer) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	s := o.Snapshot()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP goso_uptime_seconds Seconds since process start.\n")
	fmt.Fprintf(w, "# TYPE goso_uptime_seconds gauge\n")
	fmt.Fprintf(w, "goso_uptime_seconds %d\n", s.UptimeSeconds)
	fmt.Fprintf(w, "# HELP goso_requests_total Total HTTP requests.\n")
	fmt.Fprintf(w, "# TYPE goso_requests_total counter\n")
	fmt.Fprintf(w, "goso_requests_total %d\n", s.RequestCount)
	fmt.Fprintf(w, "# HELP goso_llm_calls_total Total LLM calls.\n")
	fmt.Fprintf(w, "# TYPE goso_llm_calls_total counter\n")
	fmt.Fprintf(w, "goso_llm_calls_total %d\n", s.LLMCallCount)
}
