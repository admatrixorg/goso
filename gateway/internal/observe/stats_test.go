// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleStatsAndMetrics(t *testing.T) {
	obs := NewWithWriter(&bytes.Buffer{})
	obs.reqs.Store(4)
	obs.llms.Store(2)
	obs.started = time.Now().UTC().Add(-3 * time.Second)

	mux := http.NewServeMux()
	obs.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/stats", nil))
	if w.Code != 200 {
		t.Fatalf("stats %d", w.Code)
	}
	var s Stats
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.RequestCount != 4 || s.LLMCallCount != 2 {
		t.Fatalf("stats %+v", s)
	}
	if s.UptimeSeconds < 3 {
		t.Fatalf("uptime %d", s.UptimeSeconds)
	}
	if s.StartedAt == "" {
		t.Fatal("missing started_at")
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 200 {
		t.Fatalf("metrics %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"goso_uptime_seconds",
		"goso_requests_total 4",
		"goso_llm_calls_total 2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q in %s", want, body)
		}
	}
}
