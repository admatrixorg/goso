// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cronTenantStores(t *testing.T) map[string]StoreIface {
	t.Helper()
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "cron183.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	out := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, out, "goso_cron183")
	return out
}

func minhAnPair(t *testing.T, s StoreIface, tenant, key string) (*Agent, *Session) {
	t.Helper()
	ag, err := s.CreateAgent(Agent{AgentKey: key, DisplayName: "Cố vấn Minh An", TenantID: tenant})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(Session{AgentID: ag.ID, TenantID: tenant, Label: "tư vấn"})
	if err != nil {
		t.Fatal(err)
	}
	return ag, sess
}

func TestCronTenant_StampListCapDelete(t *testing.T) {
	for name, st := range cronTenantStores(t) {
		t.Run(name, func(t *testing.T) {
			_, sales := minhAnPair(t, st, "minh-an-sales", "co-van-sales")
			_, home := minhAnPair(t, st, "default", "co-van-home")
			job, err := st.CreateCronJob(CronJob{
				Spec: "every:1h", SessionID: sales.ID, Message: "nhắc lịch tư vấn", Enabled: true,
				TenantID: "default",
			})
			if err != nil {
				t.Fatal(err)
			}
			if job.TenantID != "minh-an-sales" {
				t.Fatalf("caller tenant stored: %+v", job)
			}
			got, err := st.GetCronJob(job.ID)
			if err != nil || got.TenantID != "minh-an-sales" {
				t.Fatalf("get %v %+v", err, got)
			}
			if n := len(st.ListCronJobsTenant("minh-an-sales")); n != 1 {
				t.Fatalf("sales list %d", n)
			}
			if n := len(st.ListCronJobsTenant("default")); n != 0 {
				t.Fatalf("default list saw sales job: %d", n)
			}
			for i := 1; i < CronJobCap; i++ {
				if _, err := st.CreateCronJob(CronJob{Spec: "every:1h", SessionID: sales.ID, Message: "nhắc lịch tư vấn", Enabled: true}); err != nil {
					t.Fatalf("sales job %d: %v", i, err)
				}
			}
			if _, err := st.CreateCronJob(CronJob{Spec: "every:1h", SessionID: sales.ID, Message: "nhắc lịch tư vấn", Enabled: true}); err != ErrCronCap {
				t.Fatalf("sales cap %v", err)
			}
			homeJob, err := st.CreateCronJob(CronJob{Spec: "every:1h", SessionID: home.ID, Message: "nhắc lịch tư vấn", Enabled: true})
			if err != nil || homeJob.TenantID != "default" {
				t.Fatalf("default tenant still under its own cap: %v %+v", err, homeJob)
			}
			if len(st.ListCronJobs()) != CronJobCap+1 {
				t.Fatalf("global list %d", len(st.ListCronJobs()))
			}
			if err := st.DeleteSession(sales.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := st.GetCronJob(job.ID); err != ErrNotFound {
				t.Fatalf("sales job survived DeleteSession: %v", err)
			}
			left := st.ListCronJobs()
			if len(left) != 1 || left[0].ID != homeJob.ID {
				t.Fatalf("after delete %+v", left)
			}
			if _, err := st.GetCronJob(homeJob.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCronTenant_DeleteAgentDropsJobs(t *testing.T) {
	for name, st := range cronTenantStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, sess := minhAnPair(t, st, "minh-an-sales", "co-van-drop")
			_, other := minhAnPair(t, st, "default", "co-van-keep")
			job, err := st.CreateCronJob(CronJob{Spec: "every:1h", SessionID: sess.ID, Message: "nhắc lịch tư vấn", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			keep, err := st.CreateCronJob(CronJob{Spec: "every:1h", SessionID: other.ID, Message: "nhắc lịch tư vấn", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := st.DeleteAgent(ag.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := st.GetCronJob(job.ID); err != ErrNotFound {
				t.Fatalf("agent delete left job: %v", err)
			}
			if _, err := st.GetCronJob(keep.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCronTenantRolloutSQL_IsSeparateFromStartup(t *testing.T) {
	if len(cronTenantRolloutStmts) != 3 {
		t.Fatalf("stmts %d", len(cronTenantRolloutStmts))
	}
	joined := strings.Join(cronTenantRolloutStmts, "\n")
	if !strings.Contains(joined, "UPDATE cron_jobs AS c") || !strings.Contains(joined, "IS DISTINCT FROM") {
		t.Fatal("backfill must copy sessions.tenant_id")
	}
	if !strings.Contains(joined, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cron_jobs_tenant") {
		t.Fatal("tenant index must be CONCURRENTLY")
	}
	if !strings.Contains(joined, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cron_jobs_session") {
		t.Fatal("session index must be CONCURRENTLY")
	}
	if strings.Contains(joined, "CREATE INDEX IF NOT EXISTS") {
		t.Fatal("startup form of CREATE INDEX must not be the rollout")
	}
	if strings.Contains(joined, "DELETE ") {
		t.Fatal("rollout must not delete rows")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "qa", "183-cron-per-tenant", "migration.sql"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, stmt := range cronTenantRolloutStmts {
		if !strings.Contains(doc, stmt) {
			t.Fatalf("migration.sql missing %q", stmt)
		}
	}
	if !strings.Contains(doc, "Dat") {
		t.Fatal("migration.sql must wait for approval")
	}
	src, err := os.ReadFile("postgres.go")
	if err != nil {
		t.Fatal(err)
	}
	pg := string(src)
	if strings.Contains(pg, "UPDATE cron_jobs") || strings.Contains(pg, "idx_cron_jobs_tenant") || strings.Contains(pg, "cronTenantRolloutStmts[") {
		t.Fatal("postgres startup must stay add-only")
	}
}

func TestSQLite_CronTenantUpgradeBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			label TEXT,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default'
		)`,
		`CREATE TABLE cron_jobs (
			id TEXT PRIMARY KEY,
			spec TEXT NOT NULL,
			session_id TEXT NOT NULL,
			message TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_run TEXT
		)`,
		`INSERT INTO sessions(id, agent_id, label, created_at, tenant_id)
			VALUES ('sess-minh', 'ag-minh', 'tư vấn', '2026-11-01T00:00:00Z', 'minh-an-sales')`,
		`INSERT INTO cron_jobs(id, spec, session_id, message, enabled)
			VALUES ('job-minh', 'every:1h', 'sess-minh', 'nhắc lịch tư vấn', 1)`,
		`INSERT INTO cron_jobs(id, spec, session_id, message, enabled)
			VALUES ('job-orphan', 'every:1h', 'missing-session', 'nhắc lịch tư vấn', 1)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.GetCronJob("job-minh")
	if err != nil || job.TenantID != "minh-an-sales" {
		t.Fatalf("backfill %+v %v", job, err)
	}
	orphan, err := st.GetCronJob("job-orphan")
	if err != nil || orphan.TenantID != DefaultTenant {
		t.Fatalf("orphan %+v %v", orphan, err)
	}
	for _, idx := range []string{"idx_cron_jobs_tenant", "idx_cron_jobs_session"} {
		var n int
		if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s: %d %v", idx, n, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	job, err = again.GetCronJob("job-minh")
	if err != nil || job.TenantID != "minh-an-sales" {
		t.Fatalf("reopen %+v %v", job, err)
	}
}

func TestPostgres_CronTenantStartupAddOnly(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_cron183_old", false)
	t.Cleanup(func() { dropSchema(t, base, "goso_cron183_old") })
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cron_jobs (
		id TEXT PRIMARY KEY,
		spec TEXT NOT NULL,
		session_id TEXT NOT NULL,
		message TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		last_run TEXT,
		last_error TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cron_jobs(id, spec, session_id, message, enabled, last_error)
		VALUES ('job-minh', 'every:1h', 'sess-minh', 'nhắc lịch tư vấn', 1, '')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	job, err := st.GetCronJob("job-minh")
	if err != nil || job.TenantID != DefaultTenant {
		t.Fatalf("add-only startup %+v %v", job, err)
	}
	assertCronIndexAbsent(t, st)
	if _, err := st.db.db.Exec(`INSERT INTO agents(id, agent_key, display_name, created_at, tenant_id, enabled, updated_at)
		VALUES ('ag-minh', 'co-van', 'Cố vấn Minh An', '2026-11-01T00:00:00Z', 'minh-an-sales', 1, '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.db.Exec(`INSERT INTO sessions(id, agent_id, label, created_at, tenant_id, prompt_mode, model_override, provider_override, archived)
		VALUES ('sess-minh', 'ag-minh', 'tư vấn', '2026-11-01T00:00:00Z', 'minh-an-sales', '', '', '', false)`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	job, err = st.GetCronJob("job-minh")
	if err != nil || job.TenantID != DefaultTenant {
		t.Fatalf("second startup backfilled %+v %v", job, err)
	}
	assertCronIndexAbsent(t, st)
	for _, stmt := range cronTenantRolloutStmts {
		if _, err := st.db.db.Exec(stmt); err != nil {
			t.Fatalf("rollout %q: %v", stmt, err)
		}
	}
	job, err = st.GetCronJob("job-minh")
	if err != nil || job.TenantID != "minh-an-sales" {
		t.Fatalf("after rollout %+v %v", job, err)
	}
	for _, idx := range []string{"idx_cron_jobs_tenant", "idx_cron_jobs_session"} {
		var n int
		if err := st.db.db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, idx).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s: %d %v", idx, n, err)
		}
	}
}

func assertCronIndexAbsent(t *testing.T, st *PostgresStore) {
	t.Helper()
	for _, idx := range []string{"idx_cron_jobs_tenant", "idx_cron_jobs_session"} {
		var n int
		if err := st.db.db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=$1`, idx).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("index %s must not be created at startup", idx)
		}
	}
}
