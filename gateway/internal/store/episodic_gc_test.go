// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneKeepsImportedEpisodic(t *testing.T) {
	created := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)
	t.Run("memory", func(t *testing.T) {
		st := New()
		st.memories["sess-minh-an"] = []*Memory{
			{ID: "gc-ep:minh-an-old", SessionID: "sess-minh-an", Kind: KindEpisodic, Body: "imported minh an", CreatedAt: created, TenantID: "minh-an"},
			{ID: "native-old-minh-an", SessionID: "sess-minh-an", Kind: KindEpisodic, Body: "native minh an", CreatedAt: created, TenantID: "minh-an"},
		}
		n, err := st.PruneEpisodic(before)
		if err != nil || n != 1 {
			t.Fatalf("pruned %d %v", n, err)
		}
		if _, err := st.GetMemory("gc-ep:minh-an-old"); err != nil {
			t.Fatal(err)
		}
		if _, err := st.GetMemory("native-old-minh-an"); !errors.Is(err, ErrNotFound) {
			t.Fatal("native row survived prune")
		}
	})
	t.Run("sqlite", func(t *testing.T) {
		st, err := OpenSQLite(filepath.Join(t.TempDir(), "prune.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		assertPruneSQL(t, st, created, before)
	})
	t.Run("postgres", func(t *testing.T) {
		st := openSpec154(t, "goso_ep179_prune")
		assertPruneSQL(t, st.SQLiteStore, created, before)
	})
}

func assertPruneSQL(t *testing.T, st *SQLiteStore, created, before time.Time) {
	t.Helper()
	ag, err := st.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "minh-an-prune", DisplayName: "Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := st.InsertGCSession(Session{ID: "gc-sess:prune-minh-an", AgentID: ag.ID, TenantID: "minh-an", CreatedAt: created})
	if err != nil || !ok {
		t.Fatal(err)
	}
	ok, err = st.InsertGCMemory(Memory{
		ID: "gc-ep:prune-minh-an", SessionID: "gc-sess:prune-minh-an", TenantID: "minh-an",
		Body: "imported minh an", CreatedAt: created, CustomerKey: "goclaw:user-minh-an",
	})
	if err != nil || !ok {
		t.Fatal(err)
	}
	live, err := st.CreateSession(Session{AgentID: ag.ID, TenantID: "minh-an", Label: "web-minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		"native-old-minh-an", live.ID, KindEpisodic, "native minh an", formatTime(created), "minh-an", ""); err != nil {
		t.Fatal(err)
	}
	n, err := st.PruneEpisodic(before)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d %v", n, err)
	}
	if _, err := st.GetMemory("gc-ep:prune-minh-an"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetMemory("native-old-minh-an"); !errors.Is(err, ErrNotFound) {
		t.Fatal("native row survived prune")
	}
}

func TestDeleteEpisodicGCLeavesOtherPrefix(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		st, err := OpenSQLite(filepath.Join(t.TempDir(), "rollback.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		assertRollbackLeavesDoc(t, st)
	})
	t.Run("postgres", func(t *testing.T) {
		st := openSpec154(t, "goso_ep179_rollback")
		assertRollbackLeavesDoc(t, st.SQLiteStore)
	})
}

func assertRollbackLeavesDoc(t *testing.T, st *SQLiteStore) {
	t.Helper()
	created := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	ag, err := st.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "minh-an-rollback", DisplayName: "Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCSession(Session{ID: "gc-sess:rollback-minh-an", AgentID: ag.ID, TenantID: "minh-an", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCMemory(Memory{
		ID: "gc-ep:rollback-minh-an", SessionID: "gc-sess:rollback-minh-an", TenantID: "minh-an",
		Body: "episodic minh an", CreatedAt: created, CustomerKey: "goclaw:user-minh-an",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCSession(Session{ID: "gc-docsess:rollback-minh-an", AgentID: ag.ID, TenantID: "minh-an", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCMemory(Memory{
		ID: "gc-doc:rollback-minh-an", SessionID: "gc-docsess:rollback-minh-an", TenantID: "minh-an",
		Body: "document minh an", CreatedAt: created, CustomerKey: "goclaw:user-minh-an",
	}); err != nil {
		t.Fatal(err)
	}
	var docsBefore int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE id=?`, "gc-doc:rollback-minh-an").Scan(&docsBefore); err != nil || docsBefore != 1 {
		t.Fatalf("docs %d %v", docsBefore, err)
	}
	memN, sessN, err := st.DeleteEpisodicGC("minh-an")
	if err != nil || memN != 1 || sessN != 1 {
		t.Fatalf("removed memories=%d sessions=%d %v", memN, sessN, err)
	}
	if _, err := st.GetMemory("gc-ep:rollback-minh-an"); !errors.Is(err, ErrNotFound) {
		t.Fatal("episodic row survived rollback")
	}
	if _, err := st.GetSession("gc-sess:rollback-minh-an"); !errors.Is(err, ErrNotFound) {
		t.Fatal("episodic session survived rollback")
	}
	var docsAfter int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE id=?`, "gc-doc:rollback-minh-an").Scan(&docsAfter); err != nil || docsAfter != docsBefore {
		t.Fatalf("doc count %d want %d %v", docsAfter, docsBefore, err)
	}
	if _, err := st.GetSession("gc-docsess:rollback-minh-an"); err != nil {
		t.Fatal(err)
	}
}
