// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionActivity(t *testing.T) {
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatal(err)
	}
	stores := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, stores, "activity_idle")
	for name, st := range stores {
		t.Run(name, func(t *testing.T) {
			lister, ok := st.(ActivityLister)
			if !ok {
				t.Fatal("store must implement ActivityLister")
			}
			ag, _ := st.CreateAgent(Agent{AgentKey: "act"})
			a, _ := st.CreateSession(Session{AgentID: ag.ID})
			b, _ := st.CreateSession(Session{AgentID: ag.ID})
			_, _ = st.CreateSession(Session{AgentID: ag.ID}) // no messages → absent
			_, _ = st.AddMessage(Message{SessionID: a.ID, Role: "user", Content: "x"})
			_, _ = st.AddMessage(Message{SessionID: b.ID, Role: "user", Content: "y"})
			if _, err := st.SaveSummary(b.ID, "note"); err != nil {
				t.Fatal(err)
			}
			got, err := lister.SessionActivity(time.Now().Add(-time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			by := map[string]SessionActivity{}
			for _, g := range got {
				by[g.SessionID] = g
			}
			if len(by) != 2 {
				t.Fatalf("want 2 active sessions, got %+v", got)
			}
			if by[a.ID].LastMessageAt.IsZero() || !by[a.ID].SummaryAt.IsZero() {
				t.Fatalf("session a: %+v", by[a.ID])
			}
			if by[b.ID].SummaryAt.IsZero() || by[b.ID].SummaryAt.Before(by[b.ID].LastMessageAt.Add(-time.Second)) {
				t.Fatalf("session b: %+v", by[b.ID])
			}
			if later, _ := lister.SessionActivity(time.Now().Add(time.Hour)); len(later) != 0 {
				t.Fatalf("since filter: %+v", later)
			}
		})
	}
}
