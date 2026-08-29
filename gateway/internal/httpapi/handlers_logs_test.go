// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/logstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func logsServer(t *testing.T) (*logstore.Store, http.Handler) {
	t.Helper()
	st := store.New()
	lg := logstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Logs: lg})
	return lg, h
}

func TestLogs_ListRedactsAndV1(t *testing.T) {
	lg, h := logsServer(t)
	lg.Append(logstore.Entry{
		Level:     logstore.LevelInfo,
		Component: logstore.ComponentHTTP,
		Message:   `{"path":"/api/agents","token":"super-secret","Authorization":"Bearer abcdefghijklmnop"}`,
	})
	lg.Append(logstore.Entry{
		Level:     logstore.LevelError,
		Component: logstore.ComponentLLM,
		Message:   "provider failed sk-abcdefghijk123",
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "super-secret") || strings.Contains(body, "sk-abcdefghijk123") || strings.Contains(body, "Bearer abc") {
		t.Fatalf("leaked %s", body)
	}
	if strings.Contains(strings.ToLower(body), `"token"`) {
		t.Fatalf("secret keys %s", body)
	}
	if !strings.Contains(body, "[redacted]") {
		t.Fatalf("expected redaction %s", body)
	}
	assertSameGET(t, h, "/api/logs", "/v1/logs")
}

func TestLogs_FilterComponentTextLevelLimit(t *testing.T) {
	lg, h := logsServer(t)
	lg.Append(logstore.Entry{Level: logstore.LevelDebug, Component: logstore.ComponentHTTP, Message: "get /healthz"})
	lg.Append(logstore.Entry{Level: logstore.LevelInfo, Component: logstore.ComponentHTTP, Message: "get /api/agents"})
	lg.Append(logstore.Entry{Level: logstore.LevelWarn, Component: logstore.ComponentLLM, Message: "slow"})
	lg.Append(logstore.Entry{Level: logstore.LevelError, Component: logstore.ComponentGateway, Message: "boom"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?component=http", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"component":"http"`) {
		t.Fatalf("component %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"component":"llm"`) {
		t.Fatalf("component leak %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?level=error", nil))
	if !strings.Contains(w.Body.String(), "boom") || strings.Contains(w.Body.String(), "slow") {
		t.Fatalf("level %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?q=agents", nil))
	if !strings.Contains(w.Body.String(), "agents") || strings.Contains(w.Body.String(), "healthz") {
		t.Fatalf("q %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?limit=2", nil))
	if w.Code != 200 {
		t.Fatalf("limit %d", w.Code)
	}
	if strings.Count(w.Body.String(), `"seq":`) != 2 {
		t.Fatalf("limit body %s", w.Body.String())
	}
}

func TestLogs_StreamLiveAndReconnect(t *testing.T) {
	lg, h := logsServer(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	ready := make(chan struct{})
	got := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
		var block []string
		flush := func() {
			joined := strings.Join(block, "\n")
			block = nil
			if strings.Contains(joined, "event: ready") {
				select {
				case <-ready:
				default:
					close(ready)
				}
			}
			if strings.Contains(joined, "event: log") {
				got <- joined
			}
		}
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				flush()
				continue
			}
			block = append(block, line)
		}
		if len(block) > 0 {
			flush()
		}
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("ready timeout")
	}

	lg.Append(logstore.Entry{
		Level: logstore.LevelInfo, Component: logstore.ComponentHTTP,
		Message: `live-1 token=super-secret Bearer abcdefghijklmnop`,
	})

	var payload string
	select {
	case payload = <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("log timeout")
	}
	if strings.Contains(payload, "super-secret") || strings.Contains(payload, "Bearer abc") {
		t.Fatalf("stream leaked %s", payload)
	}
	if !strings.Contains(payload, "live-1") {
		t.Fatalf("missing live event %s", payload)
	}

	first := lg.Append(logstore.Entry{Level: logstore.LevelWarn, Component: logstore.ComponentGateway, Message: "replay-me"})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	req2, _ := http.NewRequestWithContext(ctx2, http.MethodGet, srv.URL+"/api/logs/stream", nil)
	req2.Header.Set("Last-Event-ID", logstore.SeqString(first.Seq-1))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp2.Body, 8192))
	if !strings.Contains(string(b), "replay-me") {
		t.Fatalf("replay %s", b)
	}
}

func TestLogs_ViewTokenGET(t *testing.T) {
	lg, inner := logsServer(t)
	lg.Append(logstore.Entry{Level: logstore.LevelInfo, Component: logstore.ComponentHTTP, Message: "ok"})
	h := auth.RequireTokens("admin-111", "view-111", []string{"/healthz"})(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	req.Header.Set("Authorization", "Bearer view-111")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/logs", nil)
	req.Header.Set("Authorization", "Bearer view-111")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("view v1 %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/logs", nil)
	req.Header.Set("Authorization", "Bearer view-111")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("view POST %d", w.Code)
	}

	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sreq, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	sreq.Header.Set("Authorization", "Bearer view-111")
	sreq.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(sreq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("view stream %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if !strings.Contains(string(b), "event: ready") {
		t.Fatalf("view stream body %s", b)
	}
}
