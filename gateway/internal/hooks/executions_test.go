// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package hooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// TestEngineRecordsExecution proves the ledger grows from a real dispatch, with
// the decision, the duration and the hook identity filled in.
func TestEngineRecordsExecution(t *testing.T) {
	var seen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := store.New()
	if _, err := st.CreateHook(store.Hook{
		TenantID: "qc-tenant", Event: pipeline.PreToolUse, Matcher: "*",
		HandlerType: HandlerHTTP, Config: `{"url":"` + srv.URL + `"}`, Enabled: true,
	}); err != nil {
		t.Fatalf("create hook: %v", err)
	}
	Invalidate()
	e := NewEngine(st)
	e.Run(context.Background(), pipeline.Event{
		Name: pipeline.PreToolUse, TenantID: "qc-tenant", SessionID: "sess-1", Tool: "shell",
	})
	e.Wait()

	rows, total := st.ListHookExecutions("qc-tenant", "", "", "", 10, 0)
	if total != 1 || len(rows) != 1 {
		t.Fatalf("executions %d %d", total, len(rows))
	}
	row := rows[0]
	if row.Event != string(pipeline.PreToolUse) || row.Decision != "allow" {
		t.Fatalf("row %#v", row)
	}
	if row.SessionID != "sess-1" || row.HookID == "" {
		t.Fatalf("identity %#v", row)
	}
	if seen != 1 {
		t.Fatalf("handler saw %d calls", seen)
	}
	stats := st.HookExecutionStats("qc-tenant")
	if stats["allow"] != 1 {
		t.Fatalf("stats %#v", stats)
	}
}

// TestEngineRecordsBlockDecision proves a block handler leaves a block row.
func TestEngineRecordsBlockDecision(t *testing.T) {
	st := store.New()
	if _, err := st.CreateHook(store.Hook{
		TenantID: "qc-block", Event: pipeline.PreToolUse, Matcher: "*",
		HandlerType: HandlerBlock, Config: `{"message":"denied"}`, Enabled: true,
	}); err != nil {
		t.Fatalf("create hook: %v", err)
	}
	Invalidate()
	e := NewEngine(st)
	dec := e.Run(context.Background(), pipeline.Event{Name: pipeline.PreToolUse, TenantID: "qc-block", Tool: "shell"})
	if !dec.Block {
		t.Fatal("expected a block decision")
	}
	rows, total := st.ListHookExecutions("qc-block", "", "", "block", 10, 0)
	if total != 1 || len(rows) != 1 {
		t.Fatalf("block rows %d", total)
	}
}
