// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestList_EmptyEnvFailClosed(t *testing.T) {
	t.Setenv("GOSO_SKILLS_DIR", "")
	t.Setenv("GOSO_WORKSPACE", "")
	if Configured() {
		t.Fatal("empty env is not configured")
	}
	list, err := List()
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("list %v %v", list, err)
	}
	if list != nil {
		t.Fatalf("must not walk: %+v", list)
	}
	doc, err := Load("demo")
	if !errors.Is(err, ErrNotConfigured) || doc != nil {
		t.Fatalf("load %v %v", doc, err)
	}
}

func TestLoad_TempDirOneSkill(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Demo\n\nhello skill"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\necho pwned\n"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("root must be ignored"), 0o644)
	nested := filepath.Join(root, "demo", "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("nested ignored"), 0o644)

	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "demo" || list[0].Path != "demo/SKILL.md" {
		t.Fatalf("list %+v", list)
	}
	doc, err := Load("demo")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "demo" || doc.Path != "demo/SKILL.md" || doc.Body != body {
		t.Fatalf("doc %+v", doc)
	}
	if strings.Contains(doc.Body, "pwned") || strings.Contains(doc.Body, "nested") {
		t.Fatal("must not read scripts or nested files")
	}
}

func TestLoad_PathEscapeRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok", "SKILL.md"), []byte("in"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")

	for _, name := range []string{"../ok", "..", "/etc", "ok/../ok", "ok/nested", `ok\x`, ".", ""} {
		if _, err := Load(name); !errors.Is(err, ErrPathEscape) && !errors.Is(err, ErrNotFound) {
			t.Fatalf("%q err %v", name, err)
		}
	}
	if _, err := Load("../ok"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("dotdot %v", err)
	}
	if _, err := Load("/etc"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("abs %v", err)
	}
}

func TestList_SymlinkEscapeSkipped(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(root, "escape")
	if err := os.Mkdir(escape, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(escape, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(root, "ok")
	if err := os.Mkdir(ok, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ok, "SKILL.md"), []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "ok" {
		t.Fatalf("symlink escape listed %+v", list)
	}
	if _, err := Load("escape"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("load symlink %v", err)
	}
}

func TestLoad_WorkspaceJail(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "x", "SKILL.md"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_WORKSPACE", ws)
	t.Setenv("GOSO_SKILLS_DIR", other)
	if _, err := List(); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("list outside workspace %v", err)
	}
	if _, err := Load("x"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("load outside workspace %v", err)
	}

	inside := filepath.Join(ws, "skills")
	if err := os.MkdirAll(filepath.Join(inside, "in"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "in", "SKILL.md"), []byte("yes"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", inside)
	doc, err := Load("in")
	if err != nil || doc.Body != "yes" {
		t.Fatalf("%v %+v", err, doc)
	}
}

func TestLoad_Cap64KiB(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "big")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := strings.Repeat("a", MaxBody+32)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	doc, err := Load("big")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Body) != MaxBody {
		t.Fatalf("cap %d", len(doc.Body))
	}
}

func TestLoad_Missing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	if _, err := Load("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing %v", err)
	}
}
