// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package eventstore

import (
	"strings"
	"testing"
	"time"
)

func TestEventStore_AppendFilter(t *testing.T) {
	s := New(32)
	s.Append(Event{TraceID: "t1", Connector: "zalocrm", Tool: "contact_search", Kind: KindAttempt, Summary: `{"query":"A"}`})
	s.Append(Event{TraceID: "t1", Connector: "zalocrm", Tool: "contact_search", Kind: KindSuccess, Summary: `{"n":1}`})
	s.Append(Event{TraceID: "t2", Connector: "pos", Tool: "price_change", Kind: KindHumanFeedback, Summary: `{"decision":"reject"}`})

	all := s.Filter("", "", 10)
	if len(all) != 3 {
		t.Fatalf("all %d", len(all))
	}
	crm := s.Filter("", "zalocrm", 10)
	if len(crm) != 2 {
		t.Fatalf("crm %d", len(crm))
	}
	fb := s.Filter(KindHumanFeedback, "", 10)
	if len(fb) != 1 || fb[0].Tool != "price_change" {
		t.Fatalf("fb %v", fb)
	}
}

func TestEventStore_NoCredentials(t *testing.T) {
	s := New(32)
	e := s.Append(Event{
		Connector: "zalocrm",
		Tool:      "x",
		Kind:      KindAttempt,
		Summary:   `{"query":"A","token":"super-secret","Authorization":"Bearer abc"}`,
	})
	if strings.Contains(e.Summary, "super-secret") || strings.Contains(e.Summary, "Bearer abc") {
		t.Fatalf("leaked credentials: %s", e.Summary)
	}
	if strings.Contains(strings.ToLower(e.Summary), `"token"`) || strings.Contains(e.Summary, "Authorization") {
		t.Fatalf("payload secret keys present: %s", e.Summary)
	}
}

func TestEventStore_DropsMessageToolPayloads(t *testing.T) {
	s := New(32)
	e := s.Append(Event{
		Type:    TypeMessage,
		Kind:    KindSuccess,
		Summary: `{"action":"create","body":"secret chat","arguments":{"token":"nope"},"from_agent_id":"a1"}`,
	})
	if strings.Contains(e.Summary, "secret chat") || strings.Contains(e.Summary, "nope") {
		t.Fatalf("payload leaked: %s", e.Summary)
	}
	if strings.Contains(e.Summary, `"body"`) || strings.Contains(e.Summary, `"arguments"`) {
		t.Fatalf("payload keys present: %s", e.Summary)
	}
	if !strings.Contains(e.Summary, "a1") {
		t.Fatalf("kept actor metadata: %s", e.Summary)
	}
}

func TestEventStore_QueryTypeActorAndSubscribe(t *testing.T) {
	s := New(32)
	ch, cancel := s.Subscribe(4)
	defer cancel()
	a := s.Append(Event{Type: TypeAgent, Kind: KindSuccess, Actor: "operator", AgentID: "ag1", Action: "create"})
	_ = s.Append(Event{Type: TypeTeam, Kind: KindSuccess, Actor: "operator", TeamID: "tm1", Action: "create"})
	select {
	case got := <-ch:
		if got.Seq != a.Seq || got.Type != TypeAgent {
			t.Fatalf("sub %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe timeout")
	}
	agents := s.Query(Query{Type: TypeAgent, Limit: 10})
	if len(agents) != 1 || agents[0].AgentID != "ag1" {
		t.Fatalf("type filter %v", agents)
	}
	byActor := s.Query(Query{Actor: "ag1", Limit: 10})
	if len(byActor) != 1 {
		t.Fatalf("actor %v", byActor)
	}
	after := s.Query(Query{AfterSeq: a.Seq, Limit: 10})
	if len(after) != 1 || after[0].Type != TypeTeam {
		t.Fatalf("after %v", after)
	}
}

func TestEventStore_TokenShapes(t *testing.T) {
	s := New(32)
	e := s.Append(Event{
		Kind:    KindAttempt,
		Summary: "key sk-abcdefghijklmnopqrstuvwxyz Bearer abcdefghijklmnop leftover",
	})
	if strings.Contains(e.Summary, "sk-abcdefghijklmnopqrstuvwxyz") || strings.Contains(e.Summary, "Bearer abcdefghijklmnop") {
		t.Fatalf("token shape leaked: %s", e.Summary)
	}
	if !strings.Contains(e.Summary, "[redacted]") {
		t.Fatalf("expected redaction, got %s", e.Summary)
	}
}

func TestEventStore_RingCap(t *testing.T) {
	s := New(32)
	for i := 0; i < 40; i++ {
		s.Append(Event{Kind: KindAttempt, Connector: "c", Tool: "t"})
	}
	all := s.Filter("", "", 100)
	if len(all) != 32 {
		t.Fatalf("cap %d", len(all))
	}
}
