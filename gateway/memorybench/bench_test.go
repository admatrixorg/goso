// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memorybench

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestValidateMinhAn(t *testing.T) {
	if err := Validate(Cases()); err != nil {
		t.Fatal(err)
	}
}

func TestBaseline9545d08(t *testing.T) {
	// RecallFor logs every turn. The score table below is the bench output.
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	// Recall stays on for this process. An operator shell may have turned it off.
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	// OpenSQLite refuses to open when this points at Postgres.
	t.Setenv("GOSO_DATABASE_URL", "")

	sqliteScore := runEngineTwice(t, "sqlite", func() store.StoreIface { return openSQLiteBench(t) })
	postgresScore, pgOK := runPostgres(t)

	fmt.Print("==== memory-vi ====\n")
	fmt.Print(FormatReport("sqlite", sqliteScore))
	if pgOK {
		fmt.Print(FormatReport("postgres", postgresScore))
	} else {
		fmt.Print("memory-vi postgres skipped (GOSO_TEST_POSTGRES_DSN unset)\n")
	}
	fmt.Print("==== memory-vi end ====\n")

	runs := []EngineScore{sqliteScore}
	if pgOK {
		runs = append(runs, postgresScore)
	}
	for _, run := range runs {
		for _, r := range run.Results {
			if r.ExpectZero && !r.Pass {
				t.Errorf("%s %s must return nothing: %s\nnote: %s", run.Engine, r.ID, r.Reason, r.Note)
			}
		}
	}
	if t.Failed() {
		return
	}

	if os.Getenv("GOSO_MEMORY_BENCH_RECORD") == "1" {
		if os.Getenv("CI") == "true" {
			t.Fatal("refusing to rewrite the baseline when CI=true")
		}
		if !pgOK {
			t.Fatal("recording the baseline needs Postgres")
		}
		file := NewBaseline(sqliteScore, postgresScore)
		if err := WriteBaseline(BaselinePath(), file); err != nil {
			t.Fatal(err)
		}
		if err := writeCasesJSON(filepath.Join(BenchDir(), "cases.json")); err != nil {
			t.Fatal(err)
		}
		if err := casesFileMatches(); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", BaselinePath())
		return
	}
	if err := casesFileMatches(); err != nil {
		t.Fatal(err)
	}

	base, err := LoadBaseline(BaselinePath())
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}
	checkEngine(t, "sqlite", base.Engines["sqlite"], sqliteScore)
	if pgOK {
		checkEngine(t, "postgres", base.Engines["postgres"], postgresScore)
	}
}

func openSQLiteBench(t *testing.T) store.StoreIface {
	t.Helper()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "memory-vi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func runPostgres(t *testing.T) (EngineScore, bool) {
	t.Helper()
	dsn := os.Getenv("GOSO_TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("GOSO_TEST_DATABASE_URL")
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		return EngineScore{}, false
	}
	return runEngineTwice(t, "postgres", func() store.StoreIface { return openPostgresBench(t, dsn) }), true
}

func runEngineTwice(t *testing.T, engine string, open func() store.StoreIface) EngineScore {
	t.Helper()
	st := open()
	first, err := Run(st, Cases())
	if err != nil {
		t.Fatalf("%s: %v", engine, err)
	}
	second, err := Run(st, Cases())
	if err != nil {
		t.Fatalf("%s rerun: %v", engine, err)
	}
	if !SamePass(first, second) {
		t.Fatalf("%s is not deterministic\nfirst:\n%s\nsecond:\n%s", engine, FormatReport(engine, first), FormatReport(engine, second))
	}
	first.Engine = engine
	return first
}

func checkEngine(t *testing.T, engine string, base BaselineEngine, got EngineScore) {
	t.Helper()
	if base.Total == 0 {
		t.Fatalf("%s missing from baseline", engine)
	}
	for _, msg := range Regressions(base, got) {
		r := got.ByID()
		extra := ""
		if row, ok := r[caseIDFromRegression(msg)]; ok {
			extra = "\nnote: " + row.Note + "\n" + row.Reason
		}
		t.Errorf("%s: %s%s", engine, msg, extra)
	}
}

func caseIDFromRegression(msg string) string {
	// Messages that start with a case id look like "tx-01 passed at ...".
	id := msg
	for i, r := range msg {
		if r == ' ' {
			id = msg[:i]
			break
		}
	}
	return id
}

func openPostgresBench(t *testing.T, dsn string) store.StoreIface {
	t.Helper()
	const schema = "goso_bench_memory_vi"
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	st, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
