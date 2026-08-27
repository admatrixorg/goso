// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvoke_FSEmptyEnvNoTouch(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolReadFile, map[string]any{"path": target}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("read empty %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": target, "content": "overwrite"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("write empty %v %+v", err, res)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "keep" {
		t.Fatalf("must not write when unconfigured %q", b)
	}
	ghost := filepath.Join(dir, "ghost.txt")
	_, _ = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": ghost, "content": "x"}, true)
	if _, err := os.Stat(ghost); err == nil {
		t.Fatal("must not create file when unconfigured")
	}
}

func TestInvoke_FSRoundTrip(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	res, err := Invoke(context.Background(), ToolWriteFile, map[string]any{"path": "notes/hello.md", "content": "hi"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("write %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["path"] != "notes/hello.md" || m["bytes"] != 2 {
		t.Fatalf("write content %+v", m)
	}
	onDisk, err := os.ReadFile(filepath.Join(ws, "notes", "hello.md"))
	if err != nil || string(onDisk) != "hi" {
		t.Fatalf("disk %q %v", onDisk, err)
	}
	res, err = Invoke(context.Background(), ToolReadFile, map[string]any{"path": "notes/hello.md"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("read %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["path"] != "notes/hello.md" || m["content"] != "hi" {
		t.Fatalf("read content %+v", m)
	}
}

func TestInvoke_FSEscapeRejected(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret.txt", filepath.Join(other, "secret.txt"), "notes/../../x"} {
		res, err := Invoke(context.Background(), ToolReadFile, map[string]any{"path": path}, true)
		if err != nil || res == nil || res.Status != "error" {
			t.Fatalf("read escape %s %v %+v", path, err, res)
		}
		m, _ := res.Content.(map[string]any)
		if m["error"] != "path escape" {
			t.Fatalf("read escape msg %s %+v", path, m)
		}
		res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": path, "content": "x"}, true)
		if err != nil || res == nil || res.Status != "error" {
			t.Fatalf("write escape %s %v %+v", path, err, res)
		}
		m, _ = res.Content.(map[string]any)
		if m["error"] != "path escape" {
			t.Fatalf("write escape msg %s %+v", path, m)
		}
	}
	ents, err := os.ReadDir(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "secret.txt" {
		t.Fatalf("outside dir mutated %+v", ents)
	}
}

func TestInvoke_FSSymlinkEscape(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	secret := filepath.Join(other, "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ws, "link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolReadFile, map[string]any{"path": "link.txt"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("read symlink %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": "link.txt", "content": "overwrite"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("write symlink %v %+v", err, res)
	}
	b, err := os.ReadFile(secret)
	if err != nil || string(b) != "nope" {
		t.Fatalf("symlink target mutated %q %v", b, err)
	}
	if err := os.Symlink(other, filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": "out/nested.txt", "content": "x"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("dir symlink %v %+v", err, res)
	}
	if _, err := os.Stat(filepath.Join(other, "nested.txt")); err == nil {
		t.Fatal("must not write through directory symlink")
	}
}

func TestInvoke_FSReadCap(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	big := strings.Repeat("a", MaxReadBytes+1)
	if err := os.WriteFile(filepath.Join(ws, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolReadFile, map[string]any{"path": "big.txt"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("too large %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "too large" {
		t.Fatalf("msg %+v", m)
	}
	ok := strings.Repeat("b", MaxReadBytes)
	res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": "ok.txt", "content": ok}, true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("write cap %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolReadFile, map[string]any{"path": "ok.txt"}, true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("read cap %v %+v", err, res)
	}
}

func TestInvoke_FSArgsAndDirectory(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	res, err := Invoke(context.Background(), ToolReadFile, map[string]any{"path": ""}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("empty path %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "path is required" {
		t.Fatalf("empty path msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": "a.md"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("missing content %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "content is required" {
		t.Fatalf("content msg %+v", m)
	}
	if err := os.Mkdir(filepath.Join(ws, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err = Invoke(context.Background(), ToolReadFile, map[string]any{"path": "subdir"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("dir %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "not a file" {
		t.Fatalf("dir msg %+v", m)
	}
}

func TestInvoke_FSMissingAndAbsInside(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	res, err := Invoke(context.Background(), ToolReadFile, map[string]any{"path": "missing.md"}, true)
	if err != nil || res == nil || res.Status != "not_found" {
		t.Fatalf("missing %v %+v", err, res)
	}
	inside := filepath.Join(ws, "abs.md")
	res, err = Invoke(context.Background(), ToolWriteFile, map[string]any{"path": inside, "content": "abs"}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("abs write %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolReadFile, map[string]any{"path": inside}, true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("abs read %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["content"] != "abs" || m["path"] != "abs.md" {
		t.Fatalf("abs content %+v", m)
	}
}
