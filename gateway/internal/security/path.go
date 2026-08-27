// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var pathArgKeys = map[string]struct{}{
	"path": {}, "file": {}, "filename": {}, "filepath": {},
	"dest": {}, "dest_path": {}, "source": {}, "src": {},
	"dir": {}, "directory": {}, "target": {}, "output": {},
}

// HasDotDot reports a path-escape token in p (slash-normalized).
func HasDotDot(p string) bool {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == ".." || strings.Contains(p, "/../") || strings.HasPrefix(p, "../") || strings.HasSuffix(p, "/..") {
		return true
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isPathKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if _, ok := pathArgKeys[key]; ok {
		return true
	}
	return strings.Contains(key, "path") || strings.Contains(key, "file")
}

// ConfineArg jails a filesystem argument under GOSO_WORKSPACE when set.
func ConfineArg(p string) error {
	if HasDotDot(p) {
		return fmt.Errorf("path escape")
	}
	ws := WorkspaceRoot()
	if ws == "" {
		return nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(ws, p)
	}
	return Confine(p)
}

// RejectPathArgs rejects vault/filesystem-like args that escape the workspace.
func RejectPathArgs(args map[string]any) error {
	return walkPathArgs(args)
}

func walkPathArgs(v any) error {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if s, ok := child.(string); ok && isPathKey(k) {
				if err := ConfineArg(s); err != nil {
					return err
				}
				continue
			}
			if err := walkPathArgs(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := walkPathArgs(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// WorkspaceRoot is GOSO_WORKSPACE (optional write jail).
func WorkspaceRoot() string {
	return strings.TrimSpace(os.Getenv("GOSO_WORKSPACE"))
}

func canon(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	var missing []string
	cur := abs
	for {
		if ev, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				ev = filepath.Join(ev, missing[i])
			}
			return ev
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}

// Confine rejects writes outside GOSO_WORKSPACE when that env is set.
func Confine(path string) error {
	if HasDotDot(path) {
		return fmt.Errorf("path escape")
	}
	root := WorkspaceRoot()
	if root == "" {
		return nil
	}
	absRoot := canon(root)
	absP := canon(path)
	rel, err := filepath.Rel(absRoot, absP)
	if err != nil {
		return fmt.Errorf("outside workspace")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("outside workspace")
	}
	return nil
}
