// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSQLiteDSN_Pragmas(t *testing.T) {
	t.Setenv("GOSO_SQLITE_WAL", "")
	got := sqliteDSN("/data/goso.db")
	for _, want := range []string{"?_pragma=busy_timeout(5000)", "_pragma=journal_mode(WAL)", "_txlock=immediate"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dsn %q missing %q", got, want)
		}
	}
	if got := sqliteDSN(":memory:"); strings.Contains(got, "journal_mode") || !strings.Contains(got, "busy_timeout(5000)") {
		t.Fatalf("memory dsn %q", got)
	}
	if got := sqliteDSN("file:x.db?_pragma=busy_timeout(100)&_txlock=deferred"); strings.Count(got, "busy_timeout") != 1 ||
		strings.Count(got, "_txlock") != 1 || !strings.Contains(got, "&_pragma=journal_mode(WAL)") {
		t.Fatalf("existing keys must be kept: %q", got)
	}
	t.Setenv("GOSO_SQLITE_WAL", "0")
	if got := sqliteDSN("/data/goso.db"); strings.Contains(got, "journal_mode") {
		t.Fatalf("GOSO_SQLITE_WAL=0 must keep journal mode: %q", got)
	}
}

// Every pooled connection gets busy_timeout and WAL, not just the first.
func TestSQLite_EveryConnectionHasBusyTimeoutAndWAL(t *testing.T) {
	t.Setenv("GOSO_SQLITE_WAL", "")
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "busy.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := st.db.db
	db.SetMaxIdleConns(4)
	conns := make([]*sql.Conn, 3)
	for i := range conns {
		c, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		conns[i] = c
	}
	for i, c := range conns {
		var ms int
		if err := c.QueryRowContext(t.Context(), `PRAGMA busy_timeout`).Scan(&ms); err != nil || ms != SQLiteBusyTimeoutMS {
			t.Fatalf("conn %d busy_timeout=%d err=%v", i, ms, err)
		}
		var mode string
		if err := c.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil || strings.ToLower(mode) != "wal" {
			t.Fatalf("conn %d journal_mode=%q err=%v", i, mode, err)
		}
	}
}

// A writer holding the lock (like prune at second 60) must make the store
// wait, not fail with SQLITE_BUSY, and concurrent store writers all succeed.
func TestSQLite_ConcurrentWritersDoNotGetBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(Agent{AgentKey: "busy"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(Session{AgentID: ag.ID})
	if err != nil {
		t.Fatal(err)
	}

	// A second, independent handle with no busy_timeout takes the write lock.
	other, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	other.SetMaxOpenConns(1)
	tx, err := other.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`DELETE FROM memories WHERE created_at < '1970'`); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = tx.Commit()
		close(released)
	}()

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if _, err := st.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "hi"}); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := st.SaveSummary(sess.ID, "note"); err != nil {
			errs <- err
		}
		if _, err := st.PruneEpisodic(time.Now().Add(-time.Hour)); err != nil {
			errs <- err
		}
	}()
	wg.Wait()
	<-released
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent write failed: %v", err)
	}
	msgs, err := st.ListMessages(sess.ID)
	if err != nil || len(msgs) != 40 {
		t.Fatalf("messages=%d err=%v", len(msgs), err)
	}
}
