// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIDryRunThenApplyRefused(t *testing.T) {
	export, src := fixturePaths(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.Mkdir(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	deny := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(deny, []byte("not-this-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "rename-map.json")
	args := []string{
		"--export", export,
		"--source-tenant", "minh-an",
		"--tenant", "minh-an",
		"--skills-dir", skillsDir,
		"--src-dir", src,
		"--rename-map", mapPath,
		"--deny-tenants", deny,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(args, "--dry-run"), stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	var rep struct {
		DryRun        bool `json:"dry_run"`
		DeniedTenants int  `json:"denied_tenants"`
		Tenants       []struct {
			TenantID string         `json:"tenant_id"`
			Inserted int            `json:"inserted"`
			Rejected int            `json:"rejected"`
			Reasons  map[string]int `json:"reasons"`
		} `json:"tenants"`
		Renames []struct {
			Old   string `json:"old"`
			New   string `json:"new"`
			Apply bool   `json:"apply"`
		} `json:"renames"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err, stdout.String())
	}
	if !rep.DryRun || rep.DeniedTenants != 0 {
		t.Fatalf("dry %+v", rep)
	}
	var sawMinh, sawKhac bool
	for _, c := range rep.Tenants {
		if c.TenantID == "minh-an" && c.Inserted == 6 && c.Rejected == 6 {
			sawMinh = true
		}
		if c.TenantID == "khac" && c.Reasons["foreign-tenant"] == 2 {
			sawKhac = true
		}
	}
	if !sawMinh || !sawKhac {
		t.Fatalf("counts %+v", rep.Tenants)
	}
	var sawRename bool
	for _, r := range rep.Renames {
		if r.Old == "Bao_Gia_Minh_An" && r.New == "bao-gia-minh-an" && r.Apply {
			sawRename = true
		}
	}
	if !sawRename {
		t.Fatal("stdout missing rename")
	}
	ents, err := os.ReadDir(skillsDir)
	if err != nil || len(ents) != 0 {
		t.Fatalf("dry-run wrote %#v %v", ents, err)
	}
	joined := stdout.String() + stderr.String()
	for _, leak := range []string{"LATEST-BAO-GIA", "EMBEDMARKERvector", "token-should-not-leak", "OLD-VERSION-BODY", "KHAC-ONLY"} {
		if strings.Contains(joined, leak) {
			t.Fatal("leaked", leak)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code == 0 {
		t.Fatal("apply should fail")
	}
	if !strings.Contains(stderr.String(), "apply refused") {
		t.Fatal(stderr.String())
	}
	ents, err = os.ReadDir(skillsDir)
	if err != nil || len(ents) != 0 {
		t.Fatalf("apply wrote %#v", ents)
	}
}

func TestCLIDenyFile(t *testing.T) {
	export, src := fixturePaths(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	base := []string{
		"--export", export,
		"--source-tenant", "minh-an",
		"--tenant", "minh-an",
		"--skills-dir", skillsDir,
		"--src-dir", src,
		"--rename-map", filepath.Join(dir, "map.json"),
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(base, "--deny-tenants", filepath.Join(dir, "nope.txt")), stdout, stderr); code == 0 {
		t.Fatal("missing deny file exited 0")
	}
	if _, err := os.Stat(skillsDir); !os.IsNotExist(err) {
		t.Fatal("missing deny file wrote skills")
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(base, "--deny-tenants", empty), stdout, stderr); code == 0 {
		t.Fatal("empty deny file exited 0")
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("bad id!!!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(base, "--deny-tenants", bad), stdout, stderr); code == 0 {
		t.Fatal("malformed deny file exited 0")
	}
	listed := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(listed, []byte("fake-co\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	denied := []string{
		"--export", export,
		"--source-tenant", "fake-co",
		"--tenant", "minh-an",
		"--skills-dir", skillsDir,
		"--src-dir", src,
		"--rename-map", filepath.Join(dir, "map.json"),
		"--deny-tenants", listed,
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(denied, stdout, stderr); code == 0 {
		t.Fatal("denied tenant exited 0")
	}
	if strings.Contains(stdout.String()+stderr.String(), "fake-co") {
		t.Fatal("denied name was printed")
	}
	if !strings.Contains(stdout.String(), `"denied_tenants": 1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if _, err := os.Stat(skillsDir); !os.IsNotExist(err) {
		t.Fatal("denied tenant wrote skills")
	}
}

func TestCLIUsage(t *testing.T) {
	if code := run(nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatal(code)
	}
}

func fixturePaths(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "internal", "skillimport", "testdata")
	return filepath.Join(root, "minh-an.json"), filepath.Join(root, "src")
}
