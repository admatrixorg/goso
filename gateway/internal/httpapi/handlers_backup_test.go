// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"

	_ "modernc.org/sqlite"
)

func seedBackupDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES ('alpha')`); err != nil {
		t.Fatal(err)
	}
}

func TestSystemBackupRestoreHTTP(t *testing.T) {
	root := t.TempDir()
	if strings.Contains(root, "goso-044-demo") {
		t.Fatal("must not use live demo db")
	}
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	seedBackupDB(t, src)
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", dir)

	st := store.New()
	h := Router(st, "0.1.0")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/system/backup", nil)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("backup %d %s", w.Code, w.Body.String())
	}
	var snap map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap["integrity"] != "ok" {
		t.Fatalf("integrity %v", snap["integrity"])
	}
	file, _ := snap["file"].(string)
	if file == "" {
		t.Fatal("missing file")
	}
	if _, ok := snap["bytes"].(float64); !ok {
		t.Fatalf("bytes %T", snap["bytes"])
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("live missing: %v", err)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/backup", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	body := `{"file":` + jsonQuote(file) + `}`
	req = httptest.NewRequest(http.MethodPost, "/api/system/restore", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("restore %d %s", w.Code, w.Body.String())
	}
	var rest map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rest)
	if rest["integrity"] != "ok" || rest["applied"] != false {
		t.Fatalf("restore body %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/restore", bytes.NewBufferString(`{"file":`+jsonQuote(file)+`,"apply":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("apply via HTTP want 400 got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/system/backup", nil)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("v1 backup %d %s", w.Code, w.Body.String())
	}

	bad := filepath.Join(dir, file)
	raw, err := os.ReadFile(bad)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) >= 16 {
		copy(raw[:16], []byte("NOT A SQLITE DB!"))
		if err := os.WriteFile(bad, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/restore", bytes.NewBufferString(`{"file":`+jsonQuote(file)+`}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("corrupt restore want 400 got %d %s", w.Code, w.Body.String())
	}
}

func TestSystemBackupMemoryRejected(t *testing.T) {
	t.Setenv("GOSO_DB_PATH", "")
	t.Setenv("GOSO_BACKUP_DIR", t.TempDir())
	st := store.New()
	h := Router(st, "0.1.0")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/system/backup", nil))
	if w.Code != 400 {
		t.Fatalf("want 400 got %d %s", w.Code, w.Body.String())
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
