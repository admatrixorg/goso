// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleTraces_FiltersAndPagination(t *testing.T) {
	obs := NewWithWriter(nil)
	t0 := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	obs.RecordSpans([]Span{
		{TraceID: "ok1", SpanID: "a", Kind: KindAgent, Name: "agent", Start: t0, LatencyMS: 12, Status: "ok", Attributes: map[string]string{"agent_id": "ag-a", "channel": "telegram"}},
		{TraceID: "ok1", SpanID: "l", ParentID: "a", Kind: KindLLM, Name: "echo", Start: t0, LatencyMS: 10, Status: "ok", InputTokens: 3, OutputTokens: 5},
	})
	obs.RecordSpans([]Span{
		{TraceID: "err1", SpanID: "a", Kind: KindAgent, Name: "agent", Start: t1, LatencyMS: 40, Status: "error", Error: "upstream failed", Attributes: map[string]string{"agent_id": "ag-b"}},
		{TraceID: "err1", SpanID: "l", ParentID: "a", Kind: KindLLM, Name: "boom", Start: t1, Status: "error", Error: "upstream failed"},
	})
	mux := http.NewServeMux()
	obs.Register(mux)

	get := func(path string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s status %d %s", path, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	all := get("/api/traces?limit=20")
	if n := len(asArr(all["items"])); n != 2 {
		t.Fatalf("items %d %v", n, all["items"])
	}
	if int(all["total"].(float64)) != 2 {
		t.Fatalf("total %v", all["total"])
	}

	agent := get("/api/traces?agent=ag-a")
	items := asArr(agent["items"])
	if len(items) != 1 || asObj(items[0])["trace_id"] != "ok1" {
		t.Fatalf("agent filter %+v", items)
	}

	ch := get("/api/traces?channel=telegram")
	if len(asArr(ch["items"])) != 1 {
		t.Fatalf("channel %+v", ch["items"])
	}

	st := get("/api/traces?status=error")
	if len(asArr(st["items"])) != 1 || asObj(asArr(st["items"])[0])["trace_id"] != "err1" {
		t.Fatalf("status %+v", st["items"])
	}

	q := get("/api/traces?q=boom")
	if len(asArr(q["items"])) != 1 {
		t.Fatalf("q %+v", q["items"])
	}

	from := get("/api/traces?from=" + t1.Format(time.RFC3339))
	if len(asArr(from["items"])) != 1 || asObj(asArr(from["items"])[0])["trace_id"] != "err1" {
		t.Fatalf("from %+v", from["items"])
	}

	page := get("/api/traces?limit=1&offset=0")
	if len(asArr(page["items"])) != 1 || page["truncated"] != true {
		t.Fatalf("page %+v", page)
	}
	page2 := get("/api/traces?limit=1&offset=1")
	if len(asArr(page2["items"])) != 1 || page2["truncated"] != false {
		t.Fatalf("page2 %+v", page2)
	}

	groups := asArr(all["error_groups"])
	if len(groups) != 1 || asObj(groups[0])["count"].(float64) != 1 {
		t.Fatalf("error_groups %+v", groups)
	}
}

func TestHandleTraceDetail_RedactsAndBounds(t *testing.T) {
	obs := NewWithWriter(nil)
	obs.RecordSpans([]Span{
		{
			TraceID: "t-secret", SpanID: "a", Kind: KindAgent, Name: "agent", Status: "ok",
			Attributes: map[string]string{
				"agent_id":    "ag-1",
				"prompt":      "system secret-prompt",
				"api_key":     "sk-live-abcdef",
				"arguments":   `{"token":"wh_secret"}`,
				"tool_result": "result-secret",
				"tenant_id":   "default",
			},
		},
		{TraceID: "t-secret", SpanID: "l", ParentID: "a", Kind: KindLLM, Name: "echo", Status: "ok", InputTokens: 2, OutputTokens: 4, LatencyMS: 9},
	})
	mux := http.NewServeMux()
	obs.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/traces/t-secret", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	for _, leak := range []string{"secret-prompt", "sk-live-abcdef", "wh_secret", "result-secret"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("leaked %q: %s", leak, raw)
		}
	}
	var body struct {
		TraceID string `json:"trace_id"`
		Item    Item   `json:"item"`
		Spans   []Span `json:"spans"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TraceID != "t-secret" || body.Item.AgentID != "ag-1" {
		t.Fatalf("detail %+v", body)
	}
	if body.Item.InputTokens != 2 || body.Item.OutputTokens != 4 {
		t.Fatalf("tokens %+v", body.Item)
	}
	if len(body.Spans) != 2 {
		t.Fatalf("spans %d", len(body.Spans))
	}
	if _, ok := body.Spans[0].Attributes["prompt"]; ok {
		t.Fatal("prompt attr must be dropped")
	}
	if body.Spans[0].Attributes["api_key"] != "[redacted]" {
		t.Fatalf("api_key %+v", body.Spans[0].Attributes)
	}
	if body.Spans[0].Attributes["tenant_id"] != "default" {
		t.Fatalf("tenant_id redacted: %+v", body.Spans[0].Attributes)
	}

	w404 := httptest.NewRecorder()
	mux.ServeHTTP(w404, httptest.NewRequest("GET", "/api/traces/missing", nil))
	if w404.Code != 404 {
		t.Fatalf("missing %d", w404.Code)
	}

	wV1 := httptest.NewRecorder()
	mux.ServeHTTP(wV1, httptest.NewRequest("GET", "/v1/traces/t-secret", nil))
	if wV1.Code != 200 {
		t.Fatalf("v1 detail %d", wV1.Code)
	}
}

func TestHandleTraces_TenantIsolation(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-101")
	obs := NewWithWriter(nil)
	obs.RecordSpans([]Span{
		{TraceID: "acme-t", SpanID: "a", Kind: KindAgent, Name: "agent", Attributes: map[string]string{"tenant_id": "acme", "agent_id": "ag-acme"}},
	})
	obs.RecordSpans([]Span{
		{TraceID: "other-t", SpanID: "a", Kind: KindAgent, Name: "agent", Attributes: map[string]string{"tenant_id": "other", "agent_id": "ag-other"}},
	})
	mux := http.NewServeMux()
	obs.Register(mux)

	req := httptest.NewRequest("GET", "/api/traces", nil)
	req.Header.Set("Authorization", "Bearer admin-101")
	req.Header.Set("X-Goso-Tenant", "acme")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].TraceID != "acme-t" {
		t.Fatalf("items %+v", body.Items)
	}

	reqD := httptest.NewRequest("GET", "/api/traces/other-t", nil)
	reqD.Header.Set("Authorization", "Bearer admin-101")
	reqD.Header.Set("X-Goso-Tenant", "acme")
	wD := httptest.NewRecorder()
	mux.ServeHTTP(wD, reqD)
	if wD.Code != 404 {
		t.Fatalf("cross-tenant detail %d %s", wD.Code, wD.Body.String())
	}
}

func TestPublicSpan_DropsToolPayloads(t *testing.T) {
	s := PublicSpan(Span{
		Kind: KindTool,
		Attributes: map[string]string{
			"tool_input":  "secret-args",
			"tool_result": "secret-out",
			"agent_id":    "a1",
		},
		Error: "Bearer supersecret-token failed",
	})
	if s.Attributes["agent_id"] != "a1" {
		t.Fatalf("kept %+v", s.Attributes)
	}
	if _, ok := s.Attributes["tool_input"]; ok {
		t.Fatal("tool_input kept")
	}
	if strings.Contains(s.Error, "supersecret-token") {
		t.Fatalf("error leaked %q", s.Error)
	}
}

func asArr(v any) []any {
	a, _ := v.([]any)
	if a == nil {
		return []any{}
	}
	return a
}

func asObj(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
