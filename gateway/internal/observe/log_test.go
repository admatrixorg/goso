// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/logstore"
)

func TestLogMiddleware_GeneratesAndEchoesRequestID(t *testing.T) {
	var buf bytes.Buffer
	obs := NewWithWriter(&buf)
	h := obs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Error("missing request id on context")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest("POST", "/api/agents", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("status %d", w.Code)
	}
	rid := w.Header().Get(HeaderRequestID)
	if rid == "" {
		t.Fatal("expected X-Request-ID on response")
	}
	var line accessLog
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log json: %v %q", err, buf.String())
	}
	if line.Level != "info" || line.Method != "POST" || line.Path != "/api/agents" || line.Status != 201 {
		t.Fatalf("log %+v", line)
	}
	if line.RequestID != rid {
		t.Fatalf("request_id log %q header %q", line.RequestID, rid)
	}
	if line.LatencyMS < 0 {
		t.Fatalf("latency %d", line.LatencyMS)
	}
}

func TestLogMiddleware_PreservesIncomingRequestID(t *testing.T) {
	var buf bytes.Buffer
	obs := NewWithWriter(&buf)
	h := obs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set(HeaderRequestID, "req-fixed-1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get(HeaderRequestID); got != "req-fixed-1" {
		t.Fatalf("header %q", got)
	}
	var line accessLog
	_ = json.Unmarshal(buf.Bytes(), &line)
	if line.RequestID != "req-fixed-1" {
		t.Fatalf("log request_id %q", line.RequestID)
	}
}

func TestLogMiddleware_NoSecrets(t *testing.T) {
	var buf bytes.Buffer
	obs := NewWithWriter(&buf)
	h := obs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/agents?token=supersecret-token&key=sk-live-abcdef", nil)
	req.Header.Set("Authorization", "Bearer supersecret-token")
	req.Header.Set("X-Api-Key", "sk-live-abcdef")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	out := buf.String()
	for _, secret := range []string{"supersecret-token", "sk-live-abcdef", "Bearer", "token="} {
		if strings.Contains(out, secret) {
			t.Fatalf("log leaked %q: %s", secret, out)
		}
	}
	var line accessLog
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log json: %v %q", err, out)
	}
	if line.Path != "/api/agents" {
		t.Fatalf("path should omit query, got %q", line.Path)
	}
}

func TestLogMiddleware_RecordsToLogstoreWithoutSecrets(t *testing.T) {
	var buf bytes.Buffer
	obs := NewWithWriter(&buf)
	lg := logstore.New(32)
	obs.SetLogs(lg)
	h := obs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/agents?token=supersecret-token&key=sk-live-abcdef", nil)
	req.Header.Set("Authorization", "Bearer supersecret-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	rows := lg.Query(logstore.Query{Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].Component != logstore.ComponentHTTP || rows[0].Level != logstore.LevelInfo {
		t.Fatalf("entry %+v", rows[0])
	}
	if rows[0].Message != "GET /api/agents 200 0ms" && !strings.Contains(rows[0].Message, "GET /api/agents 200") {
		t.Fatalf("message %q", rows[0].Message)
	}
	if strings.Contains(rows[0].Message, "supersecret") || strings.Contains(rows[0].Message, "sk-live") || strings.Contains(rows[0].Message, "token=") {
		t.Fatalf("leaked %s", rows[0].Message)
	}
}

func TestLogMiddleware_CountsRequests(t *testing.T) {
	obs := NewWithWriter(&bytes.Buffer{})
	h := obs.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	}
	if got := obs.Snapshot().RequestCount; got != 3 {
		t.Fatalf("request_count %d", got)
	}
}
