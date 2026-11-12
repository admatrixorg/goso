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

func TestCLIDryRunThenApply(t *testing.T) {
	export, src := fixturePaths(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.Mkdir(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "rename-map.json")
	args := []string{
		"--export", export,
		"--tenant", "minh-an",
		"--skills-dir", skillsDir,
		"--src-dir", src,
		"--rename-map", mapPath,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(args, "--dry-run"), stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	var rep struct {
		DryRun  bool `json:"dry_run"`
		Tenants []struct {
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
	if !rep.DryRun {
		t.Fatal("dry_run")
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
	raw, err := os.ReadFile(mapPath)
	if err != nil || !bytes.Contains(raw, []byte(`"new": "bao-gia-minh-an"`)) {
		t.Fatalf("rename file %v %s", err, raw)
	}
	ents, err := os.ReadDir(skillsDir)
	if err != nil || len(ents) != 0 {
		t.Fatalf("dry-run wrote %#v %v", ents, err)
	}
	joined := stdout.String() + stderr.String() + string(raw)
	for _, leak := range []string{"LATEST-BAO-GIA", "EMBEDMARKERvector", "token-should-not-leak", "OLD-VERSION-BODY", "KHAC-ONLY"} {
		if strings.Contains(joined, leak) {
			t.Fatal("leaked", leak)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply", "--dry-run"), stdout, stderr); code != 2 {
		t.Fatalf("combined exit %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	body, err := os.ReadFile(filepath.Join(skillsDir, "bao-gia-minh-an", "SKILL.md"))
	if err != nil || !bytes.Contains(body, []byte("LATEST-BAO-GIA")) {
		t.Fatalf("skill %v %s", err, body)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "khac-don")); !os.IsNotExist(err) {
		t.Fatal("apply wrote the other tenant")
	}
	stdout.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("second exit %d %s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"inserted": 0`)) || !bytes.Contains(stdout.Bytes(), []byte(`"skipped": 6`)) {
		t.Fatalf("second stdout %s", stdout.String())
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
