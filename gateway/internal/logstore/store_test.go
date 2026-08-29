// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package logstore

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLogStore_AppendQueryAndCap(t *testing.T) {
	s := New(32)
	for i := 0; i < 33; i++ {
		s.Append(Entry{Level: LevelInfo, Component: ComponentHTTP, Message: "n" + strconv.Itoa(i)})
	}
	all := s.Query(Query{Limit: 50})
	if len(all) != 32 {
		t.Fatalf("cap drop oldest: %d", len(all))
	}
	if all[0].Message != "n32" {
		t.Fatalf("newest first: %+v", all[0])
	}

	s = New(32)
	s.Append(Entry{Level: LevelDebug, Component: ComponentHTTP, Message: "get /healthz"})
	s.Append(Entry{Level: LevelInfo, Component: ComponentHTTP, Message: "get /api/agents"})
	s.Append(Entry{Level: LevelWarn, Component: ComponentLLM, Message: "slow"})
	s.Append(Entry{Level: LevelError, Component: ComponentGateway, Message: "boom"})

	httpRows := s.Query(Query{Component: ComponentHTTP, Limit: 10})
	if len(httpRows) != 2 {
		t.Fatalf("component %d", len(httpRows))
	}
	errs := s.Query(Query{Level: LevelError, Limit: 10})
	if len(errs) != 1 || errs[0].Level != LevelError {
		t.Fatalf("level %+v", errs)
	}
	warns := s.Query(Query{Level: "warn,error", Limit: 10})
	if len(warns) != 2 {
		t.Fatalf("multi level %d", len(warns))
	}
	text := s.Query(Query{Q: "agents", Limit: 10})
	if len(text) != 1 || !strings.Contains(text[0].Message, "agents") {
		t.Fatalf("text %+v", text)
	}
	comps := s.Components()
	if len(comps) < 2 {
		t.Fatalf("components %v", comps)
	}
}

func TestLogStore_NoCredentials(t *testing.T) {
	s := New(32)
	e := s.Append(Entry{
		Level:     LevelInfo,
		Component: ComponentAuth,
		Message:   `{"path":"/api/agents","token":"super-secret","Authorization":"Bearer abcdefghijklmnop"}`,
	})
	if strings.Contains(e.Message, "super-secret") || strings.Contains(e.Message, "Bearer abc") {
		t.Fatalf("leaked credentials: %s", e.Message)
	}
	if strings.Contains(strings.ToLower(e.Message), `"token"`) || strings.Contains(e.Message, "Authorization") {
		t.Fatalf("secret keys present: %s", e.Message)
	}
	sk := s.Append(Entry{Message: "provider failed sk-abcdefghijk123"})
	if strings.Contains(sk.Message, "sk-abcdefghijk123") {
		t.Fatalf("sk- leaked: %s", sk.Message)
	}
	if !strings.Contains(sk.Message, "[redacted]") {
		t.Fatalf("expected redaction: %s", sk.Message)
	}
	as := s.Append(Entry{Message: "live-1 token=super-secret"})
	if strings.Contains(as.Message, "super-secret") {
		t.Fatalf("token= leaked: %s", as.Message)
	}
}

func TestLogStore_QueryAfterAndSubscribe(t *testing.T) {
	s := New(32)
	ch, cancel := s.Subscribe(4)
	defer cancel()
	a := s.Append(Entry{Level: LevelInfo, Component: ComponentHTTP, Message: "one"})
	select {
	case got := <-ch:
		if got.Seq != a.Seq {
			t.Fatalf("sub %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe timeout")
	}
	_ = s.Append(Entry{Level: LevelInfo, Component: ComponentHTTP, Message: "two"})
	later := s.Query(Query{AfterSeq: a.Seq, Limit: 10})
	if len(later) != 1 || later[0].Message != "two" {
		t.Fatalf("after %+v", later)
	}
}

func TestLogStore_SlowSubscriberDropped(t *testing.T) {
	s := New(32)
	ch, cancel := s.Subscribe(1)
	defer cancel()
	s.Append(Entry{Message: "fill"})
	s.Append(Entry{Message: "overflow"})
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("expected slow subscriber close")
		}
	}
}

func TestLogStore_NormalizeLevelComponent(t *testing.T) {
	if NormalizeLevel("WARNING") != LevelWarn {
		t.Fatalf("warn alias")
	}
	if NormalizeLevel("") != LevelInfo {
		t.Fatalf("default info")
	}
	if NormalizeComponent(" HTTP ") != ComponentHTTP {
		t.Fatalf("component trim")
	}
	if NormalizeComponent("") != ComponentGateway {
		t.Fatalf("default component")
	}
}

func TestLogStore_PublicMessageCap(t *testing.T) {
	long := strings.Repeat("n", MaxMessageBytes+20)
	got := PublicMessage(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("cap suffix %q", got)
	}
}
