// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSanitizeDropsSecrets(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	db, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES ('keep')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE secrets (name TEXT PRIMARY KEY, nonce BLOB NOT NULL, ct BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO secrets (name, nonce, ct) VALUES ('prov', ?, ?)`, []byte{0}, []byte("sk-live-abcdefgh")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE connectors (name TEXT PRIMARY KEY, transport TEXT, credential_ref TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO connectors (name, transport, credential_ref) VALUES ('crm', 'http', 'secret:box')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	res, err := SnapshotFile(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.SecretPolicy != SecretExcluded {
		t.Fatalf("policy %q", res.SecretPolicy)
	}
	copyPath := filepath.Join(dir, res.File)
	out, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var n int
	if err := out.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("secrets remained %d", n)
	}
	var ref string
	if err := out.QueryRow(`SELECT credential_ref FROM connectors`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if ref != "" {
		t.Fatalf("credential_ref %q", ref)
	}
	if n := countItems(t, copyPath, "keep"); n != 1 {
		t.Fatalf("rows %d", n)
	}
	if _, err := os.ReadFile(src); err != nil {
		t.Fatal(err)
	}
	live, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("live secrets wiped %d", n)
	}
}

func TestTenantPruneKeepsOnlyTenant(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	db, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE agents (id TEXT PRIMARY KEY, agent_key TEXT, display_name TEXT, tenant_id TEXT NOT NULL DEFAULT 'default')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents (id, agent_key, display_name, tenant_id) VALUES ('a1','k1','one','alpha'),('a2','k2','two','beta')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE connectors (name TEXT PRIMARY KEY, transport TEXT, credential_ref TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO connectors (name, transport, credential_ref) VALUES ('crm','http','secret:box')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE channel_config (name TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1, agent_id TEXT NOT NULL DEFAULT '', dm_policy TEXT NOT NULL DEFAULT '', group_policy TEXT NOT NULL DEFAULT '', require_mention INTEGER NOT NULL DEFAULT 0, allow_from TEXT NOT NULL DEFAULT '[]', updated_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO channel_config (name, updated_at) VALUES ('telegram', 'now')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	res, err := snapshotFile(src, dir, CreateOpts{Scope: ScopeTenant, Tenant: "alpha", Destination: DestLocal})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scope != ScopeTenant || res.Tenant != "alpha" {
		t.Fatalf("%+v", res)
	}
	copyPath := filepath.Join(dir, res.File)
	out, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var n int
	if err := out.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("agents %d", n)
	}
	var tid string
	if err := out.QueryRow(`SELECT tenant_id FROM agents`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if tid != "alpha" {
		t.Fatalf("tenant %q", tid)
	}
	if err := out.QueryRow(`SELECT COUNT(*) FROM connectors`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("connectors leaked %d", n)
	}
	if err := out.QueryRow(`SELECT COUNT(*) FROM channel_config`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("channels leaked %d", n)
	}
}

func TestOpenSanitizedStripsOldSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOSO_BACKUP_DIR", dir)
	src := filepath.Join(dir, "goso-old.db")
	db, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE secrets (name TEXT PRIMARY KEY, nonce BLOB NOT NULL, ct BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO secrets (name, nonce, ct) VALUES ('prov', ?, ?)`, []byte{1}, []byte("sk-live-abcdefgh")); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	path, cleanup, err := OpenSanitized("goso-old.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	out, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var n int
	if err := out.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("download secrets %d", n)
	}
	live, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("stored copy wiped %d", n)
	}
}

func TestPreflightPostgresBlocksWithoutPgDump(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable")
	t.Setenv("GOSO_BACKUP_DIR", t.TempDir())
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("missing") }
	defer func() { lookPath = old }()
	pf := Preflight()
	if pf.Engine != "postgres" || pf.CanBackup {
		t.Fatalf("%+v", pf)
	}
	found := false
	for _, c := range pf.Checks {
		if c.ID == "pg_dump" && c.Blocking && !c.OK {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocking pg_dump %+v", pf.Checks)
	}
}

func TestPreflightSQLiteReady(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	seedDB(t, src, "alpha")
	t.Setenv("GOSO_DATABASE_URL", "")
	t.Setenv("GOSO_DB_PATH", src)
	t.Setenv("GOSO_BACKUP_DIR", filepath.Join(root, "backups"))
	pf := Preflight()
	if !pf.CanBackup || pf.Engine != "sqlite" {
		t.Fatalf("%+v", pf)
	}
}

func TestRestorePlanAndConfirm(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "live.db")
	dir := filepath.Join(root, "backups")
	seedDB(t, src, "alpha")
	res, err := SnapshotFile(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_BACKUP_DIR", dir)
	plan := PlanRestore(res.File)
	if !plan.Valid || !plan.CredentialsExcluded || !plan.ConfirmRequired {
		t.Fatalf("%+v", plan)
	}
	if err := ConfirmApply(res.File, res.File); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmApply(res.File, "nope"); !errors.Is(err, ErrConfirm) {
		t.Fatalf("got %v", err)
	}
}

func TestContainsSecrets(t *testing.T) {
	if !ContainsSecrets(map[string]any{"token": "abc"}) {
		t.Fatal("token")
	}
	if ContainsSecrets(map[string]any{"configured": true, "access_key_set": true, "endpoint": "http://127.0.0.1:9000"}) {
		t.Fatal("public s3")
	}
	if _, ok := AsPublicJSON(map[string]any{"secret": "x"}); ok {
		t.Fatal("public leaked")
	}
}

func TestS3PublicOmitsSecret(t *testing.T) {
	t.Setenv("GOSO_BACKUP_S3_ENDPOINT", "")
	t.Setenv("GOSO_BACKUP_S3_BUCKET", "")
	t.Setenv("GOSO_BACKUP_S3_ACCESS_KEY", "")
	t.Setenv("GOSO_BACKUP_S3_SECRET", "")
	r := NewRemote()
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.Contains(req.URL.RawQuery, "secret") || strings.Contains(req.URL.Path, "AKIA") {
			t.Errorf("secret in url %s", req.URL.String())
		}
		if req.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		if req.Body != nil {
			b, _ := io.ReadAll(io.LimitReader(req.Body, 64))
			if strings.Contains(string(b), "wJalr") {
				t.Errorf("secret in body")
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	pub, err := r.Put(S3Write{Endpoint: srv.URL, Bucket: "goso", Region: "us-east-1", AccessKey: "AKIAFAKE", Secret: "wJalrXUtnFEMI/K7MDENG"})
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Configured || !pub.AccessKeySet || pub.Endpoint == "" {
		t.Fatalf("%+v", pub)
	}
	raw, _ := AsPublicJSON(pub)
	if ContainsSecrets(raw) {
		t.Fatalf("leaked %+v", raw)
	}
	if err := r.Test(); err != nil {
		t.Fatal(err)
	}
	if !sawAuth {
		t.Fatal("expected signed request")
	}
	if _, err := r.Clear("nope"); !errors.Is(err, ErrConfirm) {
		t.Fatalf("got %v", err)
	}
	cleared, err := r.Clear("goso")
	if err != nil || cleared.Configured {
		t.Fatalf("%v %+v", err, cleared)
	}
}

func TestS3EnvOwnedRefusesSecretPut(t *testing.T) {
	t.Setenv("GOSO_BACKUP_S3_ACCESS_KEY", "env-key")
	t.Setenv("GOSO_BACKUP_S3_SECRET", "env-secret")
	t.Setenv("GOSO_BACKUP_S3_ENDPOINT", "http://127.0.0.1:9")
	t.Setenv("GOSO_BACKUP_S3_BUCKET", "env-bucket")
	r := NewRemote()
	pub := r.Public()
	if !pub.EnvOwned || !pub.Configured {
		t.Fatalf("%+v", pub)
	}
	if ContainsSecrets(pub) {
		t.Fatal("env secret leaked")
	}
	if _, err := r.Put(S3Write{AccessKey: "x", Secret: "y"}); !errors.Is(err, ErrEnvOwned) {
		t.Fatalf("got %v", err)
	}
}

func TestUploadFileStreams(t *testing.T) {
	t.Setenv("GOSO_BACKUP_S3_ENDPOINT", "")
	t.Setenv("GOSO_BACKUP_S3_BUCKET", "")
	t.Setenv("GOSO_BACKUP_S3_ACCESS_KEY", "")
	t.Setenv("GOSO_BACKUP_S3_SECRET", "")
	var got int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got++
		if req.Method != http.MethodPut {
			t.Errorf("method %s", req.Method)
		}
		b, _ := io.ReadAll(io.LimitReader(req.Body, 64))
		if strings.Contains(string(b), "wJalr") {
			t.Errorf("secret in body")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	r := NewRemote()
	if _, err := r.Put(S3Write{Endpoint: srv.URL, Bucket: "goso", AccessKey: "AKIAFAKE", Secret: "wJalrXUtnFEMI/K7MDENG"}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "goso-snap.db")
	if err := os.WriteFile(p, []byte("sqlite-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := r.UploadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if key != "goso-snap.db" || got == 0 {
		t.Fatalf("key %q got %d", key, got)
	}
}
