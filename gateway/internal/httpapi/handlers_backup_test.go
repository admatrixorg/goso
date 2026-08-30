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

func TestBackupPreflightAndS3HTTP(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	seedBackupDB(t, src)
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", dir)
	t.Setenv("GOSO_DATABASE_URL", "")
	t.Setenv("GOSO_BACKUP_S3_ENDPOINT", "")
	t.Setenv("GOSO_BACKUP_S3_BUCKET", "")
	t.Setenv("GOSO_BACKUP_S3_ACCESS_KEY", "")
	t.Setenv("GOSO_BACKUP_S3_SECRET", "")

	h := Router(store.New(), "0.1.0")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/backup/preflight", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"can_backup":true`) {
		t.Fatalf("preflight %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-") || strings.Contains(w.Body.String(), `"secret"`) {
		t.Fatalf("preflight leaked %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/system/backup/preflight", nil))
	if w.Code != 200 {
		t.Fatalf("v1 preflight %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/backup/s3", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("s3 get %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "access_key\":") && !strings.Contains(w.Body.String(), "access_key_set") {
		t.Fatalf("s3 get leaked key %s", w.Body.String())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.Contains(strings.ToLower(req.URL.RawQuery), "secret") {
			t.Errorf("secret query")
		}
		rw.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w = httptest.NewRecorder()
	body := `{"endpoint":` + jsonQuote(srv.URL) + `,"bucket":"goso","region":"us-east-1","access_key":"AKIAFAKE","secret":"wJalrXUtnFEMI/K7MDENG"}`
	req := httptest.NewRequest(http.MethodPut, "/api/system/backup/s3", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":true`) {
		t.Fatalf("s3 put %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "wJalr") || strings.Contains(w.Body.String(), "AKIAFAKE") {
		t.Fatalf("s3 put echoed secret %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/backup/s3/test", bytes.NewBufferString("{}"))
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("s3 test %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/system/backup", bytes.NewBufferString(`{"scope":"system"}`)))
	if w.Code != 200 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var snap map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	file, _ := snap["file"].(string)
	if file == "" || snap["secret_policy"] != "excluded" {
		t.Fatalf("snap %+v", snap)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/backup/download?file="+file, nil))
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatalf("download %d %d", w.Code, w.Body.Len())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/backup/validate", bytes.NewBufferString(`{"file":`+jsonQuote(file)+`}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Fatalf("validate %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/restore/plan", bytes.NewBufferString(`{"file":`+jsonQuote(file)+`}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"confirm_required":true`) {
		t.Fatalf("plan %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/restore", bytes.NewBufferString(`{"file":`+jsonQuote(file)+`,"apply":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm") {
		t.Fatalf("apply no confirm %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/restore", bytes.NewBufferString(`{"file":`+jsonQuote(file)+`,"apply":true,"confirm":`+jsonQuote(file)+`}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "CLI-only") {
		t.Fatalf("apply confirmed %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/system/backup/s3/clear", bytes.NewBufferString(`{"confirm":"goso"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
}

func TestBackupTenantScopeHTTP(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	db, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE agents (id TEXT PRIMARY KEY, agent_key TEXT, tenant_id TEXT NOT NULL DEFAULT 'default')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents (id, agent_key, tenant_id) VALUES ('a1','k1','alpha'),('a2','k2','beta')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", dir)
	t.Setenv("GOSO_DATABASE_URL", "")
	h := Router(store.New(), "0.1.0")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/system/backup", bytes.NewBufferString(`{"scope":"tenant","tenant":"alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("tenant backup %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"scope":"tenant"`) {
		t.Fatalf("%s", w.Body.String())
	}
}
