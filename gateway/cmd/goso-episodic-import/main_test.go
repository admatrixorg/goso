// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCLIDryRunThenIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	a1, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "minh-an-cskh", DisplayName: "CSKH"})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "minh-an-sales", DisplayName: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateAgent(store.Agent{TenantID: "tenant-b", AgentKey: "tenant-b-cskh", DisplayName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(dir, "agents.jsonl")
	body := strings.Join([]string{
		`{"source_agent":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","dest_agent":"` + a1.ID + `","dest_tenant":"minh-an"}`,
		`{"source_agent":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2","dest_agent":"` + a2.ID + `","dest_tenant":"minh-an"}`,
		`{"source_agent":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3","dest_agent":"missing-minh-an-agent","dest_tenant":"minh-an"}`,
		`{"source_agent":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1","dest_agent":"` + b.ID + `","dest_tenant":"tenant-b"}`,
		"",
	}, "\n")
	if err := os.WriteFile(agents, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	export := filepath.Join("..", "..", "internal", "episodicimport", "testdata", "minh-an-episodic.jsonl")
	tenants := filepath.Join("..", "..", "internal", "episodicimport", "testdata", "tenants.jsonl")
	args := []string{"--export", export, "--tenants", tenants, "--agents", agents, "--db", dbPath, "--now", "2026-06-01T00:00:00Z", "--dry-run"}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run %d %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatal(stderr.String())
	}
	var rep struct {
		DryRun  bool `json:"dry_run"`
		Tenants []struct {
			Source   string `json:"source_tenant"`
			Write    int    `json:"write"`
			Sessions int    `json:"sessions"`
			Skipped  struct {
				Expired   int `json:"expired"`
				TeamScope int `json:"team/scope"`
				MQ        int `json:"mq"`
				Unmapped  int `json:"unmapped"`
			} `json:"skipped"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Fatal("dry_run")
	}
	var sessions, expired, team, mq, unmapped int
	for _, sec := range rep.Tenants {
		sessions += sec.Sessions
		expired += sec.Skipped.Expired
		team += sec.Skipped.TeamScope
		mq += sec.Skipped.MQ
		unmapped += sec.Skipped.Unmapped
	}
	if sessions != 9 || expired != 1 || team != 2 || mq != 3 || unmapped != 2 {
		t.Fatalf("counts sessions=%d expired=%d team=%d mq=%d unmapped=%d", sessions, expired, team, mq, unmapped)
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := st.SQL()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry-run wrote %d %v", n, err)
	}
	_ = st.Close()

	stdout.Reset()
	if code := run(args[:len(args)-1], &stdout, &stderr); code != 0 {
		t.Fatalf("apply %d %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(args[:len(args)-1], &stdout, &stderr); code != 0 {
		t.Fatalf("second %d %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	writes := 0
	sessions = 0
	for _, sec := range rep.Tenants {
		writes += sec.Write
		sessions += sec.Sessions
	}
	if writes != 0 || sessions != 0 {
		t.Fatalf("second run wrote %d sessions %d", writes, sessions)
	}

	stdout.Reset()
	rm := []string{"--remove", "--tenant", "minh-an", "--db", dbPath, "--dry-run"}
	if code := run(rm, &stdout, &stderr); code != 0 {
		t.Fatalf("remove dry %d %s", code, stderr.String())
	}
	var removed struct {
		DryRun   bool `json:"dry_run"`
		Memories int  `json:"memories"`
		Sessions int  `json:"sessions"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &removed); err != nil {
		t.Fatal(err)
	}
	if !removed.DryRun || removed.Memories != 9 || removed.Sessions != 8 {
		t.Fatalf("remove dry %+v", removed)
	}
	if code := run([]string{"--remove", "--tenant", "minh-an", "--db", dbPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("remove %d %s", code, stderr.String())
	}
	if code := run([]string{"--db", "postgres://example.com/goso"}, &stdout, &stderr); code != 2 {
		t.Fatal("usage")
	}
	if code := run([]string{"--export", export, "--tenants", tenants, "--agents", agents, "--db", "postgres://db.example/goso"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "non-local") {
		t.Fatalf("remote db %d %s", code, stderr.String())
	}
}
