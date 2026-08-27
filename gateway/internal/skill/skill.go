// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skill

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// MaxBody is the SKILL.md read cap (64KiB). Larger files are truncated.
const MaxBody = 64 * 1024

const skillFile = "SKILL.md"

// Sentinel errors for the loader and HTTP/tool surfaces.
var (
	ErrNotConfigured = errors.New("not_configured")
	ErrNotFound      = errors.New("not_found")
	ErrPathEscape    = errors.New("path escape")
)

// Info is a listed skill (name = folder). Body is never included.
type Info struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Doc is one SKILL.md payload.
type Doc struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Body string `json:"body"`
}

// Dir is GOSO_SKILLS_DIR trimmed. Empty means not configured.
func Dir() string {
	return strings.TrimSpace(os.Getenv("GOSO_SKILLS_DIR"))
}

// Configured reports a non-empty skills dir env (no filesystem walk).
func Configured() bool {
	return Dir() != ""
}

// List returns one-level subdirs that contain SKILL.md.
// Empty/unset env → ErrNotConfigured and no walk.
func List() ([]Info, error) {
	root, err := resolveRoot()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Info{}, nil
		}
		return nil, err
	}
	out := make([]Info, 0)
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if err := validName(name); err != nil {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(name, skillFile))
		p := filepath.Join(root, name, skillFile)
		if err := confineFile(root, p); err != nil {
			continue
		}
		st, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil || confineFile(root, resolved) != nil {
				continue
			}
			st, err = os.Stat(resolved)
			if err != nil || st.IsDir() {
				continue
			}
		} else if st.IsDir() {
			continue
		}
		out = append(out, Info{Name: name, Path: rel})
	}
	return out, nil
}

// Load reads one SKILL.md by folder name. Never executes skill scripts.
func Load(name string) (*Doc, error) {
	root, err := resolveRoot()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	if err := validName(name); err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(filepath.Join(name, skillFile))
	p := filepath.Join(root, name, skillFile)
	if err := confineFile(root, p); err != nil {
		return nil, err
	}
	st, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil, ErrPathEscape
		}
		if err := confineFile(root, resolved); err != nil {
			return nil, err
		}
		p = resolved
		st, err = os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, ErrNotFound
			}
			return nil, err
		}
	}
	if st.IsDir() {
		return nil, ErrNotFound
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, MaxBody))
	if err != nil {
		return nil, err
	}
	return &Doc{Name: name, Path: rel, Body: string(body)}, nil
}

func resolveRoot() (string, error) {
	raw := Dir()
	if raw == "" {
		return "", ErrNotConfigured
	}
	if security.HasDotDot(raw) {
		return "", ErrPathEscape
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", ErrPathEscape
	}
	if err := security.Confine(abs); err != nil {
		return "", ErrPathEscape
	}
	return abs, nil
}

func validName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ErrPathEscape
	}
	if filepath.IsAbs(name) {
		return ErrPathEscape
	}
	if security.HasDotDot(name) {
		return ErrPathEscape
	}
	if name != filepath.Base(name) {
		return ErrPathEscape
	}
	if strings.ContainsAny(name, `/\`) {
		return ErrPathEscape
	}
	return nil
}

func confineFile(root, p string) error {
	if security.HasDotDot(p) {
		return ErrPathEscape
	}
	if !underRoot(root, p) {
		return ErrPathEscape
	}
	if err := security.Confine(p); err != nil {
		return ErrPathEscape
	}
	return nil
}

func canon(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		return ev
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

func underRoot(root, p string) bool {
	absRoot := canon(root)
	absP := canon(p)
	rel, err := filepath.Rel(absRoot, absP)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
