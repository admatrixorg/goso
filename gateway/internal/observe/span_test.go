// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartSpan_AgentParentLLMAndToolChildren(t *testing.T) {
	ctx, agent := StartSpan(nil, KindAgent, "agent")
	_, llm := StartSpan(ctx, KindLLM, "echo")
	llm.SetCacheReadTokens(0)
	llm.End(nil)
	_, tool := StartSpan(ctx, KindTool, "zalocrm__contact_search")
	tool.End(nil)
	agent.End(nil)

	spans := SpansFrom(ctx)
	if len(spans) != 3 {
		t.Fatalf("spans %d", len(spans))
	}
	if spans[0].Kind != KindAgent || spans[0].ParentID != "" {
		t.Fatalf("agent %+v", spans[0])
	}
	if spans[1].Kind != KindLLM || spans[1].ParentID != spans[0].SpanID {
		t.Fatalf("llm parent %+v", spans[1])
	}
	if spans[2].Kind != KindTool || spans[2].ParentID != spans[0].SpanID {
		t.Fatalf("tool parent %+v", spans[2])
	}
	if spans[1].CacheReadTokens != 0 {
		t.Fatalf("cache_read_tokens default %d", spans[1].CacheReadTokens)
	}
	if spans[0].TraceID == "" || spans[0].TraceID != spans[1].TraceID || spans[1].TraceID != spans[2].TraceID {
		t.Fatalf("trace ids %+v", spans)
	}
}

func TestStartSpan_IsolatesNewCollectorPerRun(t *testing.T) {
	outer := NewCollector()
	ctx := WithCollector(context.Background(), outer)
	_, parent := StartSpan(ctx, KindAgent, "outer")
	parent.End(nil)

	nestedCtx := WithCollector(ctx, NewCollector())
	_, child := StartSpan(nestedCtx, KindAgent, "inner")
	child.End(nil)

	if n := len(outer.Snapshot()); n != 1 || outer.Snapshot()[0].Name != "outer" {
		t.Fatalf("outer polluted: %+v", outer.Snapshot())
	}
	got := SpansFrom(nestedCtx)
	if len(got) != 1 || got[0].Name != "inner" {
		t.Fatalf("inner %+v", got)
	}
	if got[0].TraceID == outer.Snapshot()[0].TraceID {
		t.Fatal("nested run must not reuse parent trace_id")
	}
}

func TestHandleTraces_SpanTrees(t *testing.T) {
	obs := NewWithWriter(&bytes.Buffer{})
	obs.RecordSpans([]Span{
		{TraceID: "t1", SpanID: "a", Kind: KindAgent, Name: "agent", CacheReadTokens: 0},
		{TraceID: "t1", SpanID: "l", ParentID: "a", Kind: KindLLM, Name: "echo", CacheReadTokens: 0},
	})
	mux := http.NewServeMux()
	obs.Register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/traces", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var body struct {
		Traces []Trace    `json:"traces"`
		Spans  []SpanTree `json:"spans"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Traces == nil {
		t.Fatal("traces must be an array")
	}
	if len(body.Spans) != 1 || body.Spans[0].TraceID != "t1" || len(body.Spans[0].Spans) != 2 {
		t.Fatalf("spans %+v", body.Spans)
	}
	raw := w.Body.String()
	if !bytes.Contains(w.Body.Bytes(), []byte(`"cache_read_tokens":0`)) {
		t.Fatalf("expected cache_read_tokens default 0 in %s", raw)
	}

	wV1 := httptest.NewRecorder()
	mux.ServeHTTP(wV1, httptest.NewRequest("GET", "/v1/traces", nil))
	if wV1.Code != w.Code || wV1.Body.String() != w.Body.String() {
		t.Fatalf("GET /v1/traces %d %s vs /api/traces %d %s", wV1.Code, wV1.Body.String(), w.Code, w.Body.String())
	}
}
