// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// MaxContextFile is the per-file read cap for bootstrap markdown (32KiB).
const MaxContextFile = 32 * 1024

// bootstrapNames is the allowlist of direct children read from GOSO_CONTEXT_DIR.
// USER.md is optional; the others are also skipped when missing.
var bootstrapNames = []string{"SOUL.md", "IDENTITY.md", "AGENTS.md", "USER.md"}

// ContextDir is GOSO_CONTEXT_DIR trimmed. Empty means no bootstrap inject.
func ContextDir() string {
	return strings.TrimSpace(os.Getenv("GOSO_CONTEXT_DIR"))
}

// BootstrapText loads labeled bootstrap markdown from GOSO_CONTEXT_DIR.
// Empty/unset env, missing dir, or a path-escape token is a no-op (not an error).
// Files must be direct children of the dir. Each body is capped at 32KiB.
// Identity fields (display_name, agent_key) are never rewritten here.
func BootstrapText() string {
	root := ContextDir()
	if root == "" {
		return ""
	}
	if security.HasDotDot(root) {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return ""
	}
	var parts []string
	for _, name := range bootstrapNames {
		body := readBootstrapFile(abs, name)
		if body == "" {
			continue
		}
		parts = append(parts, name+":\n"+body)
	}
	return strings.Join(parts, "\n\n")
}

func readBootstrapFile(root, name string) string {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || security.HasDotDot(name) {
		return ""
	}
	p := filepath.Join(root, name)
	if security.HasDotDot(p) || !directChild(root, p) {
		return ""
	}
	st, err := os.Lstat(p)
	if err != nil {
		return ""
	}
	if st.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil || !directChild(root, resolved) {
			return ""
		}
		p = resolved
		st, err = os.Stat(p)
		if err != nil {
			return ""
		}
	}
	if st.IsDir() {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxContextFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func directChild(root, p string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	if ev, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = ev
	}
	if ev, err := filepath.EvalSymlinks(absP); err == nil {
		absP = ev
	}
	if security.HasDotDot(absP) {
		return false
	}
	return filepath.Dir(absP) == absRoot
}
