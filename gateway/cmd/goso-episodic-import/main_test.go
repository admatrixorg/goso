// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	denyPath := filepath.Join(dir, "denylist.txt")
	if err := os.WriteFile(denyPath, []byte("denied-shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	customers := filepath.Join("..", "..", "internal", "episodicimport", "testdata", "customers.json")
	args := []string{"--export", export, "--source-tenant", "11111111-1111-4111-8111-111111111111", "--tenant", "minh-an", "--tenants", tenants, "--agents", agents, "--customers", customers, "--denylist", denyPath, "--db", dbPath, "--now", "2026-06-01T00:00:00Z", "--dry-run"}
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
	var sessions, expired, team, unmapped int
	for _, sec := range rep.Tenants {
		sessions += sec.Sessions
		expired += sec.Skipped.Expired
		team += sec.Skipped.TeamScope
		unmapped += sec.Skipped.Unmapped
	}
	if sessions != 8 || expired != 1 || team != 2 || unmapped != 3 {
		t.Fatalf("counts sessions=%d expired=%d team=%d unmapped=%d", sessions, expired, team, unmapped)
	}
	if strings.Contains(stdout.String(), "user-minh-an-01") || strings.Contains(stdout.String(), "ext-minh-an-0000") || strings.Contains(stderr.String(), "user-minh-an-01") {
		t.Fatal("report printed a customer id")
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
	rm := []string{"--remove", "--tenant", "minh-an", "--denylist", denyPath, "--db", dbPath, "--dry-run"}
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
	if code := run([]string{"--remove", "--tenant", "minh-an", "--denylist", denyPath, "--db", dbPath, "--apply"}, &stdout, &stderr); code != 0 {
		t.Fatalf("remove %d %s", code, stderr.String())
	}
	if code := run([]string{"--db", "postgres://example.com/goso"}, &stdout, &stderr); code != 2 {
		t.Fatal("usage")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--export", export, "--source-tenant", "11111111-1111-4111-8111-111111111111", "--tenant", "minh-an", "--tenants", tenants, "--agents", agents, "--denylist", denyPath, "--db", "postgres://db.example/goso"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "non-local") {
		t.Fatalf("remote db %d %s", code, stderr.String())
	}
}

func TestCLIDenylistStops(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	export := filepath.Join("..", "..", "internal", "episodicimport", "testdata", "minh-an-episodic.jsonl")
	tenants := filepath.Join("..", "..", "internal", "episodicimport", "testdata", "tenants.jsonl")
	agents := filepath.Join("..", "..", "internal", "episodicimport", "testdata", "agents.jsonl")
	base := []string{"--export", export, "--source-tenant", "11111111-1111-4111-8111-111111111111", "--tenant", "minh-an", "--tenants", tenants, "--agents", agents, "--db", dbPath, "--dry-run"}
	cases := []struct {
		name string
		path string
		body string
		want string
	}{
		{name: "missing", path: filepath.Join(dir, "nope.txt"), want: "denylist is missing"},
		{name: "empty", path: filepath.Join(dir, "empty.txt"), body: "", want: "denylist is empty"},
		{name: "malformed", path: filepath.Join(dir, "bad.txt"), body: "not a tenant\n", want: "denylist is malformed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name != "missing" {
				if err := os.WriteFile(tc.path, []byte(tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			args := append(append([]string{}, base...), "--denylist", tc.path)
			if code := run(args, &stdout, &stderr); code == 0 {
				t.Fatal("expected non-zero")
			} else if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr %q", stderr.String())
			}
			if _, err := os.Stat(dbPath); err == nil {
				st, err := store.OpenSQLite(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				db, _ := st.SQL()
				var n int
				if err := db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n); err != nil || n != 0 {
					t.Fatalf("rows %d %v", n, err)
				}
				_ = st.Close()
			}
		})
	}
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("minh-an\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := append(append([]string{}, base...), "--denylist", denyPath)
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatal("denied tenant should fail")
	}
	if strings.Contains(stdout.String(), "minh-an") || strings.Contains(stderr.String(), "minh-an") {
		t.Fatalf("denied name leaked stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"denied_tenants": 1`) && !strings.Contains(stdout.String(), `"denied_tenants":1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if st, err := store.OpenSQLite(dbPath); err == nil {
		db, _ := st.SQL()
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n)
		_ = st.Close()
		if n != 0 {
			t.Fatalf("denied wrote %d", n)
		}
	}
}

func TestCLIRemoveDenylistAndApply(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "minh-an-cskh", DisplayName: "CSKH"})
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if _, err := st.InsertGCSession(store.Session{ID: "gc-sess:remove-minh-an", AgentID: ag.ID, TenantID: "minh-an", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCMemory(store.Memory{ID: "gc-ep:remove-minh-an", SessionID: "gc-sess:remove-minh-an", TenantID: "minh-an", Body: "episodic minh an", CreatedAt: created, CustomerKey: "goclaw:user-minh-an"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCSession(store.Session{ID: "gc-docsess:remove-minh-an", AgentID: ag.ID, TenantID: "minh-an", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertGCMemory(store.Memory{ID: "gc-doc:remove-minh-an", SessionID: "gc-docsess:remove-minh-an", TenantID: "minh-an", Body: "document minh an", CreatedAt: created, CustomerKey: "goclaw:user-minh-an"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	allow := filepath.Join(dir, "allow.txt")
	if err := os.WriteFile(allow, []byte("denied-shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(dir, "blocked.txt")
	if err := os.WriteFile(blocked, []byte("minh-an\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--remove", "--tenant", "minh-an", "--db", dbPath}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "denylist is missing") {
		t.Fatalf("missing denylist code=%d stderr=%s", code, stderr.String())
	}
	if n := countMem(t, dbPath, "gc-ep:remove-minh-an"); n != 1 {
		t.Fatalf("missing denylist deleted %d", 1-n)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--remove", "--tenant", "minh-an", "--denylist", blocked, "--db", dbPath, "--apply"}, &stdout, &stderr); code != 1 {
		t.Fatalf("denied remove code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "minh-an") || strings.Contains(stderr.String(), "minh-an") {
		t.Fatalf("denied id leaked stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"denied_tenants": 1`) && !strings.Contains(stdout.String(), `"denied_tenants":1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if n := countMem(t, dbPath, "gc-ep:remove-minh-an"); n != 1 {
		t.Fatal("denied remove deleted a row")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--remove", "--tenant", "minh-an", "--denylist", allow, "--db", dbPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry remove %d %s", code, stderr.String())
	}
	var dry struct {
		DryRun   bool `json:"dry_run"`
		Memories int  `json:"memories"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &dry); err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.Memories != 1 {
		t.Fatalf("dry %+v", dry)
	}
	if n := countMem(t, dbPath, "gc-ep:remove-minh-an"); n != 1 {
		t.Fatal("dry-run deleted")
	}
	stdout.Reset()
	if code := run([]string{"--remove", "--apply", "--tenant", "minh-an", "--denylist", allow, "--db", dbPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("apply %d %s", code, stderr.String())
	}
	if n := countMem(t, dbPath, "gc-ep:remove-minh-an"); n != 0 {
		t.Fatal("apply left the episodic row")
	}
	if n := countMem(t, dbPath, "gc-doc:remove-minh-an"); n != 1 {
		t.Fatal("apply removed the other prefix")
	}
}

func countMem(t *testing.T, dbPath, id string) int {
	t.Helper()
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, _ := st.SQL()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memories WHERE id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
