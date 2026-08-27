// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

func TestTraceBuffer_RingEviction(t *testing.T) {
	b := NewBuffer(3)
	for i := 1; i <= 5; i++ {
		b.Add(Trace{Provider: "echo", Model: fmt.Sprintf("m%d", i)})
	}
	if b.Len() != 3 {
		t.Fatalf("len %d", b.Len())
	}
	got := b.Recent(10)
	if len(got) != 3 {
		t.Fatalf("recent %d", len(got))
	}
	// newest first: m5, m4, m3
	if got[0].Model != "m5" || got[1].Model != "m4" || got[2].Model != "m3" {
		t.Fatalf("order %+v", got)
	}
}

func TestTraceBuffer_Capacity200(t *testing.T) {
	b := NewBuffer(DefaultTraceCapacity)
	for i := 0; i < DefaultTraceCapacity+50; i++ {
		b.Add(Trace{Provider: "echo", LatencyMS: int64(i)})
	}
	if b.Len() != DefaultTraceCapacity {
		t.Fatalf("len %d want %d", b.Len(), DefaultTraceCapacity)
	}
	got := b.Recent(1)
	if got[0].LatencyMS != DefaultTraceCapacity+49 {
		t.Fatalf("newest latency %d", got[0].LatencyMS)
	}
}

func TestTraceBuffer_RecentLimit(t *testing.T) {
	b := NewBuffer(10)
	for i := 0; i < 5; i++ {
		b.Add(Trace{Provider: "echo"})
	}
	if n := len(b.Recent(2)); n != 2 {
		t.Fatalf("limit 2 -> %d", n)
	}
	if n := len(b.Recent(0)); n != 5 { // 0 => default 20, but only 5 stored
		t.Fatalf("default limit -> %d", n)
	}
}

func TestTraceBuffer_Concurrent(t *testing.T) {
	b := NewBuffer(200)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b.Add(Trace{Provider: "echo"})
			}
		}()
	}
	wg.Wait()
	if b.Len() != 200 {
		t.Fatalf("len %d", b.Len())
	}
}

type errProvider struct{}

func (errProvider) Name() string { return "boom" }
func (errProvider) Chat(context.Context, []llm.Message) (string, error) {
	return "", errors.New("upstream failed")
}

func TestTracedProvider_RecordsSuccessAndError(t *testing.T) {
	var buf bytes.Buffer
	obs := NewWithWriter(&buf)
	p := obs.Wrap(llm.Echo{})
	ctx := WithRequestID(context.Background(), "req-llm-1")
	reply, err := p.Chat(ctx, []llm.Message{{Role: "user", Content: "hi secret-payload"}})
	if err != nil || reply != "echo: hi secret-payload" {
		t.Fatalf("echo %v %q", err, reply)
	}
	traces := obs.Traces().Recent(5)
	if len(traces) != 1 {
		t.Fatalf("traces %d", len(traces))
	}
	tr := traces[0]
	if tr.Provider != "echo" || tr.Model != "echo" || tr.Error != "" || tr.RequestID != "req-llm-1" {
		t.Fatalf("trace %+v", tr)
	}
	if tr.CacheReadTokens != 0 {
		t.Fatalf("cache_read_tokens %d", tr.CacheReadTokens)
	}
	if obs.Snapshot().LLMCallCount != 1 {
		t.Fatalf("llm count %d", obs.Snapshot().LLMCallCount)
	}
	if strings.Contains(buf.String(), "secret-payload") {
		t.Fatalf("llm log leaked prompt: %s", buf.String())
	}

	_, err = obs.Wrap(errProvider{}).Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	tr = obs.Traces().Recent(1)[0]
	if tr.Provider != "boom" || tr.Error != "upstream failed" {
		t.Fatalf("error trace %+v", tr)
	}
	if obs.Snapshot().LLMCallCount != 2 {
		t.Fatalf("llm count %d", obs.Snapshot().LLMCallCount)
	}
}

func TestWrap_PreservesToolChatOnlyWhenInnerHasIt(t *testing.T) {
	obs := NewWithWriter(&bytes.Buffer{})
	if _, ok := obs.Wrap(llm.Echo{}).(llm.ToolChat); ok {
		t.Fatal("echo wrap must not grow ToolChat")
	}
	inner := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	if _, ok := obs.Wrap(inner).(llm.ToolChat); !ok {
		t.Fatal("scripted wrap must keep ToolChat")
	}
}

func TestTracedProvider_ForwardsChatTools(t *testing.T) {
	obs := New()
	inner := &llm.Scripted{Replies: []llm.Reply{{Text: "via-tools"}}}
	p := obs.Wrap(inner)
	tc, ok := p.(llm.ToolChat)
	if !ok {
		t.Fatal("wrap must keep ToolChat")
	}
	reply, err := tc.ChatTools(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil || reply.Text != "via-tools" {
		t.Fatalf("ChatTools %v %+v", err, reply)
	}
	if obs.Snapshot().LLMCallCount != 1 {
		t.Fatalf("llm count %d", obs.Snapshot().LLMCallCount)
	}
}

func TestHandleTraces(t *testing.T) {
	obs := NewWithWriter(&bytes.Buffer{})
	for i := 0; i < 5; i++ {
		obs.Record(Trace{Provider: "echo", Model: fmt.Sprintf("m%d", i)})
	}
	mux := http.NewServeMux()
	obs.Register(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/traces?limit=2", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var body struct {
		Traces []Trace `json:"traces"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Traces) != 2 {
		t.Fatalf("len %d body %s", len(body.Traces), w.Body.String())
	}
	if body.Traces[0].Model != "m4" {
		t.Fatalf("newest %s", body.Traces[0].Model)
	}
	if body.Traces[0].CacheReadTokens != 0 {
		t.Fatalf("cache_read_tokens %d", body.Traces[0].CacheReadTokens)
	}
	if !strings.Contains(w.Body.String(), `"spans"`) {
		t.Fatalf("GET /api/traces missing spans: %s", w.Body.String())
	}
}
