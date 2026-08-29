// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const postgresPingTimeout = 3 * time.Second

// PostgresStore is StoreIface on Postgres 16. Embeds the SQL method set;
// lexical search uses strpos/LIKE (FTS5 is SQLite-only). Vector search is
// not wired — optional pgvector only adds a nullable kg_entities.embedding.
type PostgresStore struct {
	*SQLiteStore
}

var _ StoreIface = (*PostgresStore)(nil)

// OpenPostgres opens a Postgres DSN with pgx via database/sql, migrates the
// StoreIface schema, and optionally enables pgvector. Connect failure is
// returned as-is — never a SQLite fallback.
func OpenPostgres(dsn string) (*PostgresStore, error) {
	dsn = strings.TrimSpace(dsn)
	if !IsPostgresDSN(dsn) {
		return nil, fmt.Errorf("not a postgres DSN")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), postgresPingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	inner := &SQLiteStore{db: &sqlHandle{db: db, pg: true}, pg: true}
	s := &PostgresStore{SQLiteStore: inner}
	if err := s.migratePostgres(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *PostgresStore) migratePostgres() error {
	for _, stmt := range postgresSchema {
		if _, err := s.db.db.Exec(stmt); err != nil {
			return fmt.Errorf("postgres migrate: %w", err)
		}
	}
	_, _ = s.db.db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS enabled INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.db.Exec(`ALTER TABLE agents ADD COLUMN IF NOT EXISTS updated_at TEXT NOT NULL DEFAULT ''`)
	s.tryVector()
	return nil
}

func (s *PostgresStore) tryVector() {
	if _, err := s.db.db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		log.Printf("goso store: CREATE EXTENSION vector skipped: %v", err)
		return
	}
	if _, err := s.db.db.Exec(`ALTER TABLE kg_entities ADD COLUMN IF NOT EXISTS embedding vector`); err != nil {
		log.Printf("goso store: kg_entities.embedding skipped: %v", err)
		return
	}
	s.vector = true
}

// HasVector reports whether CREATE EXTENSION vector succeeded (embeddings
// themselves are filled by a later SPEC; search stays lexical).
func (s *PostgresStore) HasVector() bool { return s.vector }

// postgresSchema covers current SQLite StoreIface tables, including tenant_id
// where SQLite already has it. BYTEA replaces SQLite BLOB. No FTS5.
var postgresSchema = []string{
	`CREATE TABLE IF NOT EXISTS agents (
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
	`CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		label TEXT,
		created_at TEXT NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		prompt_mode TEXT NOT NULL DEFAULT '',
		FOREIGN KEY(agent_id) REFERENCES agents(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
	`CREATE TABLE IF NOT EXISTS connectors (
		name TEXT PRIMARY KEY,
		transport TEXT NOT NULL,
		endpoint TEXT,
		credential_ref TEXT,
		schema_version TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		manifest_url TEXT,
		manifest_json TEXT,
		timeout_ms INTEGER,
		retries INTEGER,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS agent_connectors (
		agent_id TEXT NOT NULL,
		connector_name TEXT NOT NULL,
		PRIMARY KEY(agent_id, connector_name),
		FOREIGN KEY(agent_id) REFERENCES agents(id),
		FOREIGN KEY(connector_name) REFERENCES connectors(name)
	)`,
	`CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		FOREIGN KEY(session_id) REFERENCES sessions(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_tenant ON memories(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS vault_docs (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		path TEXT NOT NULL UNIQUE,
		sha256 TEXT NOT NULL,
		mtime TEXT NOT NULL,
		body TEXT,
		tenant_id TEXT NOT NULL DEFAULT 'default'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_vault_docs_title ON vault_docs(title)`,
	`CREATE INDEX IF NOT EXISTS idx_vault_docs_tenant ON vault_docs(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS vault_links (
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL DEFAULT '',
		raw TEXT NOT NULL,
		PRIMARY KEY(from_id, raw)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_vault_links_to ON vault_links(to_id)`,
	`CREATE TABLE IF NOT EXISTS teams (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		lead_agent_id TEXT,
		created_at TEXT NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT 'default'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_teams_tenant ON teams(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS team_members (
		team_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		role TEXT NOT NULL,
		PRIMARY KEY(team_id, agent_id)
	)`,
	`CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT PRIMARY KEY,
		team_id TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL,
		assignee_agent_id TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_team_tasks_team ON team_tasks(team_id, status)`,
	`CREATE TABLE IF NOT EXISTS team_messages (
		id TEXT PRIMARY KEY,
		team_id TEXT NOT NULL,
		from_agent_id TEXT,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_team_messages_team ON team_messages(team_id, created_at)`,
	`CREATE TABLE IF NOT EXISTS agent_links (
		from_agent_id TEXT NOT NULL,
		to_agent_id TEXT NOT NULL,
		PRIMARY KEY(from_agent_id, to_agent_id)
	)`,
	`CREATE TABLE IF NOT EXISTS agent_metrics (
		agent_id TEXT PRIMARY KEY,
		chat_runs INTEGER NOT NULL DEFAULT 0,
		tool_errors INTEGER NOT NULL DEFAULT 0,
		tool_uses_json TEXT,
		advertised_json TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS evolution_applies (
		agent_id TEXT NOT NULL,
		suggestion_id TEXT NOT NULL,
		PRIMARY KEY(agent_id, suggestion_id)
	)`,
	`CREATE TABLE IF NOT EXISTS evolution_guardrails (
		agent_id TEXT PRIMARY KEY,
		payload TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS secrets (
		name TEXT PRIMARY KEY,
		nonce BYTEA NOT NULL,
		ct BYTEA NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tool_flags (
		name TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS cron_jobs (
		id TEXT PRIMARY KEY,
		spec TEXT NOT NULL,
		session_id TEXT NOT NULL,
		message TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		last_run TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS llm_providers (
		name TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		base_url TEXT,
		model TEXT,
		tenant_id TEXT NOT NULL DEFAULT 'default'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_llm_providers_tenant ON llm_providers(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS webhooks (
		id TEXT PRIMARY KEY,
		name TEXT,
		kind TEXT NOT NULL DEFAULT 'llm',
		agent_id TEXT,
		token_prefix TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		hmac_enc TEXT,
		require_hmac INTEGER NOT NULL DEFAULT 0,
		revoked INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		tenant_id TEXT NOT NULL DEFAULT 'default'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_webhooks_token_hash ON webhooks(token_hash)`,
	`CREATE INDEX IF NOT EXISTS idx_webhooks_tenant ON webhooks(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS webhook_jobs (
		id TEXT PRIMARY KEY,
		webhook_id TEXT NOT NULL,
		status TEXT NOT NULL,
		input TEXT,
		reply TEXT,
		error TEXT,
		callback_url TEXT,
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TEXT,
		idempotency_key TEXT,
		body_hash TEXT,
		lease_token TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_webhook_jobs_claim ON webhook_jobs(status, next_attempt_at)`,
	`CREATE INDEX IF NOT EXISTS idx_webhook_jobs_idem ON webhook_jobs(webhook_id, idempotency_key)`,
	`CREATE TABLE IF NOT EXISTS kg_entities (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		name TEXT NOT NULL,
		kind TEXT,
		body TEXT,
		valid_from TEXT NOT NULL,
		valid_until TEXT,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_kg_entities_tenant ON kg_entities(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS kg_relations (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		rel TEXT NOT NULL,
		body TEXT,
		valid_from TEXT NOT NULL,
		valid_until TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_kg_relations_tenant ON kg_relations(tenant_id)`,
	`CREATE INDEX IF NOT EXISTS idx_kg_relations_from ON kg_relations(from_id)`,
	`CREATE INDEX IF NOT EXISTS idx_kg_relations_to ON kg_relations(to_id)`,
	`CREATE TABLE IF NOT EXISTS channel_config (
		name TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 1,
		agent_id TEXT NOT NULL DEFAULT '',
		dm_policy TEXT NOT NULL DEFAULT '',
		group_policy TEXT NOT NULL DEFAULT '',
		require_mention INTEGER NOT NULL DEFAULT 0,
		allow_from TEXT NOT NULL DEFAULT '[]',
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS channel_pairing (
		id TEXT PRIMARY KEY,
		channel TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		code_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		expires_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		approved_at TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_channel_pairing_sender ON channel_pairing(channel, sender_id, status)`,
}
