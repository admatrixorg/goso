// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"path"
	"strings"
)

const (
	kindMemory = "memory"
	kindRoot   = "root"
	kindSystem = "system"
)

// classify reports where a memory_documents.path sits.
// memory/ and a root file are importable. _system/ is not.
// A path that escapes, is absolute, or is some other folder is refused.
func classify(raw string) (clean, kind string, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsRune(s, 0) || strings.Contains(s, "\\") {
		return "", "", false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "%2e") || strings.Contains(s, "->") {
		return "", "", false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(lower, "file:") {
		return "", "", false
	}
	if len(s) >= 2 && s[1] == ':' {
		return "", "", false
	}
	if len(s) > 500 {
		return "", "", false
	}
	var parts []string
	for _, p := range strings.Split(s, "/") {
		if p == "" || p == "." {
			continue
		}
		if p == ".." || strings.Contains(p, "..") {
			return "", "", false
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "", "", false
	}
	clean = strings.Join(parts, "/")
	switch {
	case clean == "_system" || strings.HasPrefix(clean, "_system/"):
		if clean == "_system" {
			return "", "", false
		}
		return clean, kindSystem, true
	case clean == "memory" || strings.HasPrefix(clean, "memory/"):
		if clean == "memory" {
			return "", "", false
		}
		return clean, kindMemory, true
	case strings.Contains(clean, "/"):
		return "", "", false
	default:
		return clean, kindRoot, true
	}
}

// VaultPath is memory/goclaw/<agent>/<source path>.
// The source path is appended as itself: memory/notes.md stays one memory/
// folder under the agent, and a root file does not gain a memory/ folder.
// The agent slug is one segment. ".." is rejected so Join cannot climb out.
func VaultPath(agent, source string) (string, error) {
	if !agentSlugOK(agent) {
		return "", errConfig("agent id is not a safe slug")
	}
	return path.Join("memory/goclaw", agent, source), nil
}

func agentSlugOK(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 200 {
		return false
	}
	if strings.Contains(s, "..") || strings.ContainsAny(s, `/\:`) {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

// systemFileName is the last segment of an _system/ path. Content is not used.
func systemFileName(clean string) string {
	return path.Base(clean)
}
