// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func postgresTestDSN() string {
	if v := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
}

func dsnWithSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func prepareSchema(t *testing.T, dsn, schema string, old bool) string {
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
		stmts := []string{
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
				prompt_mode TEXT NOT NULL DEFAULT '',
				FOREIGN KEY(agent_id) REFERENCES ` + schema + `.agents(id)
			)`,
			`CREATE TABLE ` + schema + `.messages (
				id TEXT PRIMARY KEY,
				session_id TEXT NOT NULL,
				role TEXT NOT NULL,
				content TEXT NOT NULL,
				created_at TEXT NOT NULL,
				FOREIGN KEY(session_id) REFERENCES ` + schema + `.sessions(id)
			)`,
		}
		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	out, err := dsnWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPostgresSessionColumnsFreshAndUpgrade(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	t.Run("fresh", func(t *testing.T) {
		dsn := prepareSchema(t, base, "goso_sess_fresh", false)
		exerciseSessionMessages(t, dsn, true)
	})
	t.Run("upgrade", func(t *testing.T) {
		dsn := prepareSchema(t, base, "goso_sess_old", true)
		exerciseSessionMessages(t, dsn, true)
	})
}

func exerciseSessionMessages(t *testing.T, dsn string, checkTypes bool) {
	t.Helper()
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	migrated, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	migrated.Close()
	if checkTypes {
		assertSessionColumnTypes(t, st)
	}
	a, err := st.CreateAgent(Agent{AgentKey: "pg-sess-" + newID(), DisplayName: "A", Model: "gcli/grok-4.5-high", TenantID: DefaultTenant})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(Session{AgentID: a.ID, Label: "omni:t:a", TenantID: DefaultTenant, ModelOverride: "gcli/grok-4.5-high", ProviderOverride: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Archived {
		t.Fatal("new session archived")
	}
	upd := *sess
	upd.TouchModel = true
	upd.ModelOverride = "gcli/grok-4.5"
	upd.ProviderOverride = "custom"
	upd.TouchArchive = true
	upd.Archived = true
	got, err := st.UpdateSession(upd)
	if err != nil || got.ModelOverride != "gcli/grok-4.5" || !got.Archived {
		t.Fatalf("update %#v %v", got, err)
	}
	again, err := st.GetSession(sess.ID)
	if err != nil || again.ModelOverride != "gcli/grok-4.5" || again.ProviderOverride != "custom" || !again.Archived {
		t.Fatalf("get %#v %v", again, err)
	}
	if _, err := st.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "hello", AgentID: a.ID, MentionAgentID: a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(Message{SessionID: sess.ID, Role: "assistant", Content: "xin chao", AgentID: a.ID, Model: "gcli/grok-4.5", Provider: "custom"}); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range st.ListSessions() {
		if s != nil && s.ID == sess.ID && s.Archived && s.ModelOverride == "gcli/grok-4.5" {
			found = true
		}
	}
	if !found {
		t.Fatal("list sessions missed the row")
	}
	msgs, err := st.ListMessages(sess.ID)
	if err != nil || len(msgs) != 2 || msgs[0].MentionAgentID != a.ID || msgs[1].Model != "gcli/grok-4.5" || msgs[1].Provider != "custom" {
		t.Fatalf("messages %#v %v", msgs, err)
	}
}

func assertSessionColumnTypes(t *testing.T, st *PostgresStore) {
	t.Helper()
	var dataType, nullable, def string
	err := st.db.db.QueryRow(`
		SELECT data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'sessions' AND column_name = 'archived'`).Scan(&dataType, &nullable, &def)
	if err != nil {
		t.Fatal(err)
	}
	if dataType != "boolean" || nullable != "NO" || !strings.Contains(strings.ToLower(def), "false") {
		t.Fatalf("archived type=%s null=%s default=%s", dataType, nullable, def)
	}
	err = st.db.db.QueryRow(`
		SELECT data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'messages' AND column_name = 'model'`).Scan(&dataType, &nullable, &def)
	if err != nil {
		t.Fatal(err)
	}
	if dataType != "text" || nullable != "YES" || !strings.Contains(def, "''") {
		t.Fatalf("messages.model type=%s null=%s default=%s", dataType, nullable, def)
	}
}
