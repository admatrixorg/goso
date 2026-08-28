// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func seedDB(t *testing.T, path string, name string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES (?)`, name); err != nil {
		t.Fatal(err)
	}
}

func countItems(t *testing.T, path, want string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if want != "" {
		var got string
		if err := db.QueryRow(`SELECT name FROM items ORDER BY id LIMIT 1`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("name %q want %q", got, want)
		}
	}
	return n
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	if strings.Contains(root, "goso-044-demo") {
		t.Fatal("must not use live demo db")
	}
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	seedDB(t, src, "alpha")
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", dir)

	live, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if _, err := live.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		t.Fatal(err)
	}

	res, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if res.Integrity != "ok" || res.Bytes <= 0 || res.File == "" {
		t.Fatalf("snapshot %+v", res)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("live file missing after snapshot: %v", err)
	}

	if _, err := live.Exec(`INSERT INTO items (name) VALUES ('beta')`); err != nil {
		t.Fatalf("live db locked after snapshot: %v", err)
	}
	if n := countItems(t, src, ""); n != 2 {
		t.Fatalf("live rows %d", n)
	}

	tmp, cleanup, err := RestoreToTemp(res.File)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if n := countItems(t, tmp, "alpha"); n != 1 {
		t.Fatalf("restored rows %d", n)
	}
}

func TestCorruptSnapshotRejected(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	seedDB(t, src, "alpha")
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", dir)
	res, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, res.File)
	raw, err := os.ReadFile(bad)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 16 {
		t.Fatalf("snapshot too small %d", len(raw))
	}
	copy(raw[:16], []byte("NOT A SQLITE DB!"))
	if err := os.WriteFile(bad, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := IntegrityCheck(bad); err == nil {
		t.Fatal("expected corrupt snapshot")
	}
	if _, _, err := RestoreToTemp(res.File); err == nil {
		t.Fatal("restore accepted corrupt snapshot")
	}
}

func TestPathEscapeRejected(t *testing.T) {
	t.Setenv("GOSO_BACKUP_DIR", t.TempDir())
	if _, err := Resolve("../secret.db"); err == nil {
		t.Fatal("expected escape")
	}
	if _, err := Resolve("/etc/passwd.db"); err == nil {
		t.Fatal("expected abs escape")
	}
}

func TestConcurrentSnapshotsKeepWinners(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	seedDB(t, src, "alpha")
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", dir)
	const n = 8
	var wg sync.WaitGroup
	files := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := Snapshot()
			if err != nil {
				t.Errorf("snapshot: %v", err)
				return
			}
			files <- res.File
		}()
	}
	wg.Wait()
	close(files)
	got := 0
	for name := range files {
		got++
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("winner %s missing: %v", name, err)
		}
	}
	if got == 0 {
		t.Fatal("no successful snapshots")
	}
}

func TestMemoryRejected(t *testing.T) {
	t.Setenv("GOSO_DB_PATH", "")
	t.Setenv("GOSO_BACKUP_DIR", t.TempDir())
	if _, err := Snapshot(); err != ErrNoFile {
		t.Fatalf("got %v", err)
	}
}

func TestApplyRoundTrip(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	dest := filepath.Join(root, "applied.db")
	seedDB(t, src, "alpha")
	t.Setenv("GOSO_BACKUP_DIR", dir)
	res, err := SnapshotFile(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(res.File, dest); err != nil {
		t.Fatal(err)
	}
	if n := countItems(t, dest, "alpha"); n != 1 {
		t.Fatalf("applied rows %d", n)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source deleted: %v", err)
	}
}
