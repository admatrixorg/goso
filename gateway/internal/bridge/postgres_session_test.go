// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestBridgeReplySessionOnPostgres(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if base == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	for _, tc := range []struct {
		name string
		old  bool
	}{
		{name: "fresh", old: false},
		{name: "upgrade", old: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := bridgeSchema(t, base, "goso_bridge_"+tc.name, tc.old)
			st, err := store.OpenPostgres(dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			replyOnPostgres(t, st)
		})
	}
}

func replyOnPostgres(t *testing.T, st *store.PostgresStore) {
	t.Helper()
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_LLM_API_KEY", "")
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)

	const (
		tenant  = "00000000-0000-0000-0000-000000000001"
		agentID = "agent-pg"
		prompt  = "hello from bridge"
	)
	svc := &Service{
		Secret:  "test-secret",
		Version: "test",
		Catalog: newMem(),
		Store:   st,
		Complete: func(context.Context, Completion) (Result, error) {
			return Result{Text: "pong"}, nil
		},
	}
	addr := startBridge(t, svc)
	base := "http://" + addr
	prov := []byte(`{"name":"9router","provider_type":"custom","base_url":"http://9router:20128/v1","api_key":"fixture-key","models":["gcli/grok-4.5-high"],"is_active":true}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/router", svc.Secret, tenant, prov); res.StatusCode != 200 {
		t.Fatalf("provider %d %s", res.StatusCode, res.Body)
	}
	agent := []byte(`{"name":"pg","harness_type":"GOSO","model_name":"gcli/grok-4.5-high"}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/agents/"+agentID, svc.Secret, tenant, agent); res.StatusCode != 200 {
		t.Fatalf("agent %d %s", res.StatusCode, res.Body)
	}
	body := []byte(`{"agent_id":"` + agentID + `","model":"gcli/grok-4.5-high","prompt":"` + prompt + `"}`)
	res := doSigned(t, http.MethodPost, base+"/v1/chat/reply/stream", svc.Secret, tenant, body)
	if res.StatusCode != 200 || strings.Contains(res.Body, "cannot open session") {
		t.Fatalf("stream %d %s", res.StatusCode, res.Body)
	}
	unary := doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, body)
	if unary.StatusCode != 200 || !strings.Contains(unary.Body, `"reply_text":"pong"`) || strings.Contains(unary.Body, "cannot open session") {
		t.Fatalf("reply %d %s", unary.StatusCode, unary.Body)
	}
	var sessions int
	var messages int
	for _, sess := range st.ListSessions() {
		if sess == nil {
			continue
		}
		sessions++
		msgs, err := st.ListMessages(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		messages += len(msgs)
	}
	if sessions == 0 || messages < 2 {
		t.Fatalf("sessions=%d messages=%d", sessions, messages)
	}
}

func bridgeSchema(t *testing.T, dsn, schema string, old bool) string {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	if old {
		for _, stmt := range []string{
			`CREATE TABLE ` + schema + `.agents (
				id TEXT PRIMARY KEY,
				agent_key TEXT NOT NULL UNIQUE,
				display_name TEXT,
				model TEXT,
				llm_provider TEXT,
				instructions TEXT,
				orchestration_mode TEXT,
				created_at TEXT NOT NULL,
				tenant_id TEXT NOT NULL DEFAULT 'default',
				enabled INTEGER NOT NULL DEFAULT 1,
				updated_at TEXT NOT NULL DEFAULT ''
			)`,
			`CREATE TABLE ` + schema + `.sessions (
				id TEXT PRIMARY KEY,
				agent_id TEXT NOT NULL,
				label TEXT,
				created_at TEXT NOT NULL,
				tenant_id TEXT NOT NULL DEFAULT 'default',
				prompt_mode TEXT NOT NULL DEFAULT ''
			)`,
			`CREATE TABLE ` + schema + `.messages (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				role TEXT NOT NULL,
				content TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
		} {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String()
}
