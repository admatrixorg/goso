// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCLIDryRunThenApplyIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sales, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{AgentKey: "other-co-sales", DisplayName: "Cố vấn khác", TenantID: "other-co"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: "minh-an", Label: "lich"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join("..", "..", "internal", "scheduleimport", "testdata", "minh-an.json")
	args := []string{"--export", exportPath, "--source-tenant", "minh-an", "--tenant", "minh-an", "--db", dbPath}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"needs_credential": true`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "fixture-secret-minh-an-do-not-log") || strings.Contains(stderr.String(), "fixture-secret-minh-an-do-not-log") {
		t.Fatal("cli leaked a credential")
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ListCronJobs()) != 0 {
		t.Fatal("dry-run wrote a cron job")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "fixture-secret-minh-an-do-not-log") {
		t.Fatal("apply leaked a credential")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("second exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"skipped": 2`) || strings.Contains(stdout.String(), `"written": 2`) {
		t.Fatalf("second stdout %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply", "--dry-run"), stdout, stderr); code != 2 {
		t.Fatalf("conflict exit %d", code)
	}

	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n := 0
	for _, j := range st.ListCronJobs() {
		n++
		if j.Enabled {
			t.Fatalf("enabled %+v", j)
		}
	}
	if n != 2 {
		t.Fatalf("jobs %d", n)
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram")
	if err != nil || cfg.Enabled {
		t.Fatalf("channel %+v %v", cfg, err)
	}
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("fixture-secret-minh-an-do-not-log")) {
		t.Fatal("db contains the credential")
	}
}

func TestCLIRefusesRemotePostgres(t *testing.T) {
	code := run([]string{
		"--export", "x", "--source-tenant", "minh-an", "--tenant", "minh-an",
		"--db", "postgres://user@db.example:5432/goso",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code == 0 {
		t.Fatal("remote postgres was accepted")
	}
}
