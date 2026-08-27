// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"path/filepath"
	"testing"
)

func TestHasDotDotAndRejectPathArgs(t *testing.T) {
	if !HasDotDot("../etc/passwd") || !HasDotDot("foo/../../bar") {
		t.Fatal("expected ..")
	}
	if HasDotDot("notes/hello.md") {
		t.Fatal("clean path")
	}
	if err := RejectPathArgs(map[string]any{"path": "../secret"}); err == nil {
		t.Fatal("expected path escape")
	}
	if err := RejectPathArgs(map[string]any{"query": "ok", "file": "in.md"}); err != nil {
		t.Fatal(err)
	}
	if err := RejectPathArgs(map[string]any{"file": map[string]any{"path": "../x"}}); err == nil {
		t.Fatal("expected nested path escape")
	}
}

func TestConfine_Workspace(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", root)
	inside := filepath.Join(root, "vault", "a.md")
	if err := Confine(inside); err != nil {
		t.Fatal(err)
	}
	if err := Confine(filepath.Join(other, "nope.md")); err == nil {
		t.Fatal("expected outside workspace")
	}
	t.Setenv("GOSO_WORKSPACE", "")
	if err := Confine(filepath.Join(other, "ok.md")); err != nil {
		t.Fatal(err)
	}
}
