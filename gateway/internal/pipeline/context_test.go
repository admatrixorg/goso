// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestBootstrapText_EmptyEnvNoOp(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_DIR", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("should-not-appear"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := BootstrapText(); got != "" {
		t.Fatalf("empty env want empty, got %q", got)
	}
}

func TestBootstrapText_MissingDirNoOp(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_DIR", filepath.Join(t.TempDir(), "missing-context"))
	if got := BootstrapText(); got != "" {
		t.Fatalf("missing dir want empty, got %q", got)
	}
}

func TestBootstrapText_PathEscapeIgnored(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_DIR", filepath.Join(t.TempDir(), "ctx")+string(os.PathSeparator)+".."+string(os.PathSeparator)+"escape")
	if got := BootstrapText(); got != "" {
		t.Fatalf("path escape want empty, got %q", got)
	}
}

func TestBootstrapText_TempdirAllowlist(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("SOUL.md", "soul-body")
	write("IDENTITY.md", "identity-body")
	write("AGENTS.md", "agents-body")
	write("USER.md", "user-body")
	write("README.md", "ignored-extra")
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SOUL.md"), []byte("nested-soul"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_DIR", dir)
	got := BootstrapText()
	for _, want := range []string{"SOUL.md:", "soul-body", "IDENTITY.md:", "identity-body", "AGENTS.md:", "agents-body", "USER.md:", "user-body"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "ignored-extra") || strings.Contains(got, "nested-soul") {
		t.Fatalf("leaked extra/nested file: %q", got)
	}
}

func TestBootstrapText_Cap32KiB(t *testing.T) {
	dir := t.TempDir()
	raw := strings.Repeat("a", MaxContextFile+64)
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_DIR", dir)
	got := BootstrapText()
	body := strings.TrimPrefix(got, "SOUL.md:\n")
	if len(body) != MaxContextFile {
		t.Fatalf("cap %d want %d", len(body), MaxContextFile)
	}
}

func TestBootstrapText_SymlinkEscapeIgnored(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("outside-secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(dir, "SOUL.md")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_DIR", dir)
	if got := BootstrapText(); strings.Contains(got, "outside-secret") {
		t.Fatalf("symlink escape leaked: %q", got)
	}
}

func TestBootstrapText_EmptyEnvIgnoresCwdAgents(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("AGENTS.md", []byte("cwd-agents-marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_DIR", "")
	if got := BootstrapText(); got != "" {
		t.Fatalf("empty env want empty, got %q", got)
	}
	if sys := SystemPrompt(ModeFull, "A"); strings.Contains(sys, "cwd-agents-marker") {
		t.Fatalf("cwd AGENTS.md leaked into SystemPrompt: %q", sys)
	}
}

func TestRun_BootstrapSoulInSystem(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul-marker-xyz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("rewrite-name"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_DIR", dir)

	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "Agent One"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	r := NewRunner(st, nil, scripted, nil)
	if _, err := r.Run(context.Background(), sess.ID, "hi", ModeFull); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sys := firstSystem(scripted)
	if !strings.Contains(sys, "SOUL.md:") || !strings.Contains(sys, "soul-marker-xyz") {
		t.Fatalf("SOUL.md missing from system: %q", sys)
	}
	if !strings.Contains(sys, "IDENTITY.md:") || !strings.Contains(sys, "rewrite-name") {
		t.Fatalf("IDENTITY.md missing from system: %q", sys)
	}
	if !strings.Contains(sys, "You are Agent One") {
		t.Fatalf("display identity missing from system: %q", sys)
	}
	got, err := st.GetAgent(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Agent One" || got.AgentKey != "k1" {
		t.Fatalf("identity mutated: %+v", got)
	}
}

func TestRun_BootstrapMissingDirNoOp(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_DIR", filepath.Join(t.TempDir(), "no-such-dir"))
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "Agent One"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	r := NewRunner(st, nil, scripted, nil)
	if _, err := r.Run(context.Background(), sess.ID, "hi", ModeFull); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sys := firstSystem(scripted)
	if strings.Contains(sys, "SOUL.md:") {
		t.Fatalf("missing dir injected: %q", sys)
	}
}

func TestRun_BootstrapNoneSkipsInject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul-none-marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_DIR", dir)
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "Agent One"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	r := NewRunner(st, nil, scripted, nil)
	if _, err := r.Run(context.Background(), sess.ID, "hi", ModeNone); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sys := firstSystem(scripted); strings.Contains(sys, "soul-none-marker") || strings.Contains(sys, "SOUL.md:") {
		t.Fatalf("mode none injected bootstrap: %q", sys)
	}
}

func TestRun_BootstrapPathEscapeIgnored(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_DIR", "../outside-context")
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "Agent One"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	r := NewRunner(st, nil, scripted, nil)
	if _, err := r.Run(context.Background(), sess.ID, "hi", ModeFull); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sys := firstSystem(scripted)
	if strings.Contains(sys, "SOUL.md:") {
		t.Fatalf("path escape injected: %q", sys)
	}
}

func firstSystem(s *llm.Scripted) string {
	if s == nil || len(s.Recorded) == 0 {
		return ""
	}
	for _, m := range s.Recorded[0] {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}
