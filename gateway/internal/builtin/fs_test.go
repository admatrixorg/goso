// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
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

func TestInvoke_ListFilesHappyAndJail(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.Mkdir(filepath.Join(ws, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "notes", "a.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolListFiles, map[string]any{"path": "notes"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("list %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	ents, _ := m["entries"].([]map[string]any)
	if len(ents) != 1 || ents[0]["name"] != "a.md" || ents[0]["type"] != "file" {
		t.Fatalf("entries %+v", m["entries"])
	}
	res, err = Invoke(context.Background(), ToolListFiles, map[string]any{}, false)
	if err != nil || res.Status != "ok" {
		t.Fatalf("root %v %+v", err, res)
	}
	other := t.TempDir()
	res, err = Invoke(context.Background(), ToolListFiles, map[string]any{"path": filepath.Join(other, "x")}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("abs outside %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("abs outside msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolListFiles, map[string]any{"path": ".."}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("dotdot %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("dotdot msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolListFiles, map[string]any{"path": "notes/a.md"}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("file as dir %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "not a directory" {
		t.Fatalf("file as dir msg %+v", m)
	}
}

func TestInvoke_ListFilesEmptyDir(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	res, err := Invoke(context.Background(), ToolListFiles, map[string]any{}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("empty root %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	ents, _ := m["entries"].([]map[string]any)
	if len(ents) != 0 {
		t.Fatalf("want empty entries %+v", m)
	}
	if err := os.Mkdir(filepath.Join(ws, "blank"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err = Invoke(context.Background(), ToolListFiles, map[string]any{"path": "blank"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("empty subdir %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	ents, _ = m["entries"].([]map[string]any)
	if len(ents) != 0 {
		t.Fatalf("want empty subdir %+v", m)
	}
}

func TestInvoke_ListEditSendSymlinkEscape(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	secret := filepath.Join(other, "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(ws, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSendFile, map[string]any{"path": "link.txt"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("send symlink %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolEdit, map[string]any{"path": "link.txt", "old": "nope", "new": "x"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("edit symlink %v %+v", err, res)
	}
	b, err := os.ReadFile(secret)
	if err != nil || string(b) != "nope" {
		t.Fatalf("symlink target mutated %q %v", b, err)
	}
	res, err = Invoke(context.Background(), ToolListFiles, map[string]any{"path": "out"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("list dir symlink %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("list dir symlink msg %+v", m)
	}
}

func TestInvoke_ListFilesEmptyEnvNoTouch(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	dir := t.TempDir()
	res, err := Invoke(context.Background(), ToolListFiles, map[string]any{"path": dir}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("empty %v %+v", err, res)
	}
}

func TestInvoke_EditOneReplaceAndJail(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.WriteFile(filepath.Join(ws, "t.md"), []byte("one two one"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolEdit, map[string]any{"path": "t.md", "old": "one", "new": "ONE"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("edit %v %+v", err, res)
	}
	b, err := os.ReadFile(filepath.Join(ws, "t.md"))
	if err != nil || string(b) != "ONE two one" {
		t.Fatalf("disk %q %v", b, err)
	}
	res, err = Invoke(context.Background(), ToolEdit, map[string]any{"path": "t.md", "old": "missing", "new": "x"}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("missing old %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "old not found" {
		t.Fatalf("missing old msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolEdit, map[string]any{"path": "../t.md", "old": "ONE", "new": "x"}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("escape %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("escape msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolEdit, map[string]any{"path": "t.md", "old": "", "new": "x"}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("empty old %v %+v", err, res)
	}
}

func TestInvoke_EditEmptyEnvNoTouch(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "t.md")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolEdit, map[string]any{"path": target, "old": "keep", "new": "x"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("empty %v %+v", err, res)
	}
	b, _ := os.ReadFile(target)
	if string(b) != "keep" {
		t.Fatalf("mutated %q", b)
	}
}

func TestInvoke_SendFileMetadataOnlyAndJail(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	body := []byte("hello png")
	if err := os.WriteFile(filepath.Join(ws, "pic.png"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSendFile, map[string]any{"path": "pic.png"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("send %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["path"] != "pic.png" {
		t.Fatalf("path %+v", m)
	}
	if m["bytes"] != int64(len(body)) {
		t.Fatalf("bytes %+v", m)
	}
	mime, _ := m["mime"].(string)
	if mime != "image/png" && mime != "application/octet-stream" {
		t.Fatalf("mime %q", mime)
	}
	if _, ok := m["content"]; ok {
		t.Fatal("must not return file content")
	}
	res, err = Invoke(context.Background(), ToolSendFile, map[string]any{"path": "../pic.png"}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("escape %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("escape msg %+v", m)
	}
}

func TestInvoke_SendFileEmptyEnvNoTouch(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSendFile, map[string]any{"path": target}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("empty %v %+v", err, res)
	}
}

func TestInvoke_SearchHappyAndJail(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.Mkdir(filepath.Join(ws, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "notes", "a.md"), []byte("Hello GOSO\nsecond\nHELLO again"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "root.txt"), []byte("no match here"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSearch, map[string]any{"q": "hello"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("search %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["hits"].([]map[string]any)
	if len(hits) != 2 {
		t.Fatalf("hits %+v", m["hits"])
	}
	if hits[0]["path"] != "notes/a.md" || hits[0]["line"] != 1 || hits[0]["snippet"] != "Hello GOSO" {
		t.Fatalf("hit0 %+v", hits[0])
	}
	if hits[1]["line"] != 3 {
		t.Fatalf("hit1 %+v", hits[1])
	}
	res, err = Invoke(context.Background(), ToolSearch, map[string]any{"q": "hello", "path": "notes"}, false)
	if err != nil || res.Status != "ok" {
		t.Fatalf("scoped %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	hits, _ = m["hits"].([]map[string]any)
	if len(hits) != 2 {
		t.Fatalf("scoped hits %+v", m["hits"])
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Invoke(context.Background(), ToolSearch, map[string]any{"q": "hello", "path": ".."}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("dotdot %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("dotdot msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolSearch, map[string]any{"q": "hello", "path": filepath.Join(other, "secret.txt")}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("abs outside %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("abs outside msg %+v", m)
	}
}

func TestInvoke_SearchEmptyQNoWalk(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSearch, map[string]any{"q": "  "}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("empty q %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "q is required" {
		t.Fatalf("empty q msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolSearch, map[string]any{"q": "", "path": ".."}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("empty q escape %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "q is required" {
		t.Fatalf("must not walk path %+v", m)
	}
	res, err = Invoke(context.Background(), ToolSearch, map[string]any{}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("missing q %v %+v", err, res)
	}
}

func TestInvoke_SearchEmptyEnvNoTouch(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	dir := t.TempDir()
	target := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSearch, map[string]any{"q": "hello", "path": target}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("empty %v %+v", err, res)
	}
}

func TestInvoke_SearchCapBinaryAndLargeSkip(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString("needle line\n")
	}
	if err := os.WriteFile(filepath.Join(ws, "many.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "bin.dat"), []byte("needle\x00hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("needle ", (MaxReadBytes/7)+8)
	if err := os.WriteFile(filepath.Join(ws, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSearch, map[string]any{"q": "needle"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("search %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["hits"].([]map[string]any)
	if len(hits) != maxFSHits {
		t.Fatalf("cap got %d", len(hits))
	}
	if m["truncated"] != true {
		t.Fatalf("truncated %+v", m)
	}
	for _, h := range hits {
		if h["path"] != "many.txt" {
			t.Fatalf("binary or large leaked %+v", h)
		}
	}
}

func TestInvoke_SearchSymlinkEscape(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	secret := filepath.Join(other, "secret.txt")
	if err := os.WriteFile(secret, []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(ws, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolSearch, map[string]any{"q": "needle"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("search %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["hits"].([]map[string]any)
	if len(hits) != 0 {
		t.Fatalf("symlink leaked %+v", hits)
	}
	res, err = Invoke(context.Background(), ToolSearch, map[string]any{"q": "needle", "path": "out"}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("dir symlink %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("dir symlink msg %+v", m)
	}
}

func TestInvoke_GlobHappyAndJail(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.Mkdir(filepath.Join(ws, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "notes", "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "notes", "b.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "c.md"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "*.md"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("glob %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["hits"].([]map[string]any)
	got := map[string]bool{}
	for _, h := range hits {
		p, _ := h["path"].(string)
		got[p] = true
	}
	if !got["c.md"] || !got["notes/a.md"] {
		t.Fatalf("basename+rel %+v", hits)
	}
	if got["notes/b.txt"] {
		t.Fatalf("txt matched %+v", hits)
	}
	res, err = Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "notes/*.md"}, false)
	if err != nil || res.Status != "ok" {
		t.Fatalf("path pattern %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	hits, _ = m["hits"].([]map[string]any)
	if len(hits) != 1 || hits[0]["path"] != "notes/a.md" {
		t.Fatalf("path pattern hits %+v", hits)
	}
	res, err = Invoke(context.Background(), ToolGlob, map[string]any{"pattern": ".."}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("dotdot %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("dotdot msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "../secret"}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("dotdot prefix %v %+v", err, res)
	}
	m, _ = res.Content.(map[string]any)
	if m["error"] != "path escape" {
		t.Fatalf("dotdot prefix msg %+v", m)
	}
}

func TestInvoke_GlobEmptyPatternNoWalk(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.WriteFile(filepath.Join(ws, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "  "}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("empty %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "pattern is required" {
		t.Fatalf("empty msg %+v", m)
	}
	res, err = Invoke(context.Background(), ToolGlob, map[string]any{}, true)
	if err != nil || res.Status != "error" {
		t.Fatalf("missing %v %+v", err, res)
	}
}

func TestInvoke_GlobEmptyEnvNoTouch(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "*.md"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("empty %v %+v", err, res)
	}
}

func TestInvoke_GlobCap(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	for i := 0; i < maxListEnts+10; i++ {
		name := filepath.Join(ws, fmt.Sprintf("f%03d.txt", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "*.txt"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("glob %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["hits"].([]map[string]any)
	if len(hits) != maxListEnts {
		t.Fatalf("cap got %d", len(hits))
	}
	if m["truncated"] != true {
		t.Fatalf("truncated %+v", m)
	}
}

func TestInvoke_GlobSymlinkNoEscape(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.WriteFile(filepath.Join(other, "secret.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(other, "secret.md"), filepath.Join(ws, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	res, err := Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "*.md"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("glob %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["hits"].([]map[string]any)
	for _, h := range hits {
		p, _ := h["path"].(string)
		if strings.Contains(p, "secret") || strings.HasPrefix(p, "out/") {
			t.Fatalf("escaped %+v", hits)
		}
	}
}

func TestInvoke_SearchMissingPathNotFound(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	res, err := Invoke(context.Background(), ToolSearch, map[string]any{"q": "hello", "path": "nope"}, true)
	if err != nil || res == nil || res.Status != "not_found" {
		t.Fatalf("missing %v %+v", err, res)
	}
}

func TestInvoke_SearchSkipsFifo(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "pipe"), 0o644); err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	type ret struct {
		res *connector.InvokeResult
		err error
	}
	ch := make(chan ret, 1)
	go func() {
		r, e := Invoke(ctx, ToolSearch, map[string]any{"q": "needle"}, false)
		ch <- ret{r, e}
	}()
	select {
	case <-ctx.Done():
		t.Fatal("search hung on fifo")
	case got := <-ch:
		if got.err != nil || got.res == nil || got.res.Status != "ok" {
			t.Fatalf("search %v %+v", got.err, got.res)
		}
		m, _ := got.res.Content.(map[string]any)
		hits, _ := m["hits"].([]map[string]any)
		if len(hits) != 1 || hits[0]["path"] != "a.txt" {
			t.Fatalf("hits %+v", hits)
		}
	}
}

func TestInvoke_GlobInvalidPattern(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	res, err := Invoke(context.Background(), ToolGlob, map[string]any{"pattern": "["}, true)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("invalid %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "invalid pattern" {
		t.Fatalf("msg %+v", m)
	}
}

func TestConfigured_SearchGlobWorkspace(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	if Configured(ToolSearch) || Configured(ToolGlob) {
		t.Fatal("empty workspace must not configure search/glob")
	}
	t.Setenv("GOSO_WORKSPACE", t.TempDir())
	if !Configured(ToolSearch) || !Configured(ToolGlob) {
		t.Fatal("workspace must configure search/glob")
	}
}
