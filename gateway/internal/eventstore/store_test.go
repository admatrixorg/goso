// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package eventstore

import (
	"strings"
	"testing"
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
