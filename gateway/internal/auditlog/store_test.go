// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auditlog

import (
	"strings"
	"testing"
	"time"
)

func TestAppend_ImmutableAndRedactsSecrets(t *testing.T) {
	s := New(64)
	first := s.Append(Record{
		Action:   "create",
		Actor:    "operator",
		Entity:   "agent",
		EntityID: "ag1",
		IP:       "10.0.0.1",
		After: map[string]any{
			"enabled": true,
			"api_key": "sk-live-abcdefghijk",
			"token":   "super-secret",
			"body":    "should-drop",
		},
	})
	if first.Seq != 1 || first.ID == "" {
		t.Fatalf("first %#v", first)
	}
	got := s.Query(Query{Limit: 10})
	if got.Total != 1 || len(got.Records) != 1 {
		t.Fatalf("page %+v", got)
	}
	row := got.Records[0]
	if row.After["enabled"] != true {
		t.Fatalf("kept metadata %#v", row.After)
	}
	if _, ok := row.After["api_key"]; ok {
		t.Fatalf("api_key leaked %#v", row.After)
	}
	if _, ok := row.After["token"]; ok {
		t.Fatalf("token leaked %#v", row.After)
	}
	if _, ok := row.After["body"]; ok {
		t.Fatalf("body leaked %#v", row.After)
	}
	second := s.Append(Record{Action: "delete", Actor: "operator", Entity: "agent", EntityID: "ag1"})
	if second.Seq != 2 {
		t.Fatalf("seq %d", second.Seq)
	}
	again := s.Query(Query{Limit: 10})
	var created Record
	for _, r := range again.Records {
		if r.Seq == 1 {
			created = r
		}
	}
	if created.Action != "create" || created.EntityID != "ag1" || created.IP != "10.0.0.1" {
		t.Fatalf("mutated %#v", created)
	}
}

func TestQuery_FiltersAndStableBefore(t *testing.T) {
	s := New(64)
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 6; i++ {
		s.Append(Record{
			Action:   "update",
			Actor:    "operator",
			Entity:   "agent",
			EntityID: "ag1",
			IP:       "10.0.0.1",
			TS:       base.Add(time.Duration(i) * time.Minute),
		})
	}
	s.Append(Record{Action: "approve", Actor: "alice", Entity: "node", EntityID: "nd1", IP: "10.0.0.9", TS: base.Add(10 * time.Minute)})
	byAction := s.Query(Query{Action: "approve"})
	if byAction.Total != 1 || byAction.Records[0].Entity != "node" {
		t.Fatalf("action %+v", byAction)
	}
	byActor := s.Query(Query{Actor: "alice"})
	if byActor.Total != 1 {
		t.Fatalf("actor %+v", byActor)
	}
	byIP := s.Query(Query{IP: "10.0.0.9"})
	if byIP.Total != 1 {
		t.Fatalf("ip %+v", byIP)
	}
	byEntity := s.Query(Query{Entity: "agent"})
	if byEntity.Total != 6 {
		t.Fatalf("entity %+v", byEntity)
	}
	window := s.Query(Query{Since: base.Add(3 * time.Minute), Until: base.Add(5 * time.Minute)})
	if window.Total != 2 {
		t.Fatalf("time %+v", window)
	}
	page1 := s.Query(Query{Entity: "agent", Limit: 2})
	if len(page1.Records) != 2 || page1.NextBefore == 0 {
		t.Fatalf("page1 %+v", page1)
	}
	snap := page1.Records[0].Seq
	s.Append(Record{Action: "update", Actor: "operator", Entity: "agent", EntityID: "ag-new", TS: base.Add(20 * time.Minute)})
	page2 := s.Query(Query{Entity: "agent", Limit: 2, BeforeSeq: page1.NextBefore})
	if len(page2.Records) != 2 {
		t.Fatalf("page2 %+v", page2)
	}
	for _, r := range page2.Records {
		if r.Seq >= snap {
			t.Fatalf("unstable page %#v vs first seq %d", r, snap)
		}
	}
}

func TestPublicMeta_TokenShapeAndOverflowCap(t *testing.T) {
	meta := PublicMeta(map[string]any{
		"note":   "Bearer abcdefghijk",
		"status": "paired",
	})
	if meta["status"] != "paired" {
		t.Fatalf("status %#v", meta)
	}
	if s, _ := meta["note"].(string); !strings.Contains(s, "[redacted]") {
		t.Fatalf("token shape %#v", meta["note"])
	}
	s := New(64)
	for i := 0; i < 80; i++ {
		s.Append(Record{Action: "x", Actor: "op", Entity: "e"})
	}
	got := s.Query(Query{Limit: 200})
	if got.Total != 64 {
		t.Fatalf("cap total %d", got.Total)
	}
	if got.Records[0].Seq < got.Records[len(got.Records)-1].Seq {
		t.Fatalf("not newest-first")
	}
}
