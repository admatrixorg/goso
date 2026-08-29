// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/security"
)

const (
	ToolReadFile  = "read_file"
	ToolWriteFile = "write_file"
	ToolListFiles = "list_files"
	ToolEdit      = "edit"
	ToolSendFile  = "send_file"
	ToolSearch    = "search"
	ToolGlob      = "glob"
	// MaxReadBytes is the read_file/edit cap. Larger files are rejected, not truncated.
	MaxReadBytes = 1 << 20
	maxListEnts  = 256
	maxFSHits    = 50
	binarySniff  = 512
	maxSnippet   = 256
)

var (
	errNotConfigured   = errors.New("not_configured")
	errPathEscape      = errors.New("path escape")
	errPathRequired    = errors.New("path is required")
	errNotFile         = errors.New("not a file")
	errNotDir          = errors.New("not a directory")
	errTooLarge        = errors.New("too large")
	errOldRequired     = errors.New("old is required")
	errOldNotFound     = errors.New("old not found")
	errNewRequired     = errors.New("new is required")
	errQRequired       = errors.New("q is required")
	errPatternRequired = errors.New("pattern is required")
	errInvalidPattern  = errors.New("invalid pattern")
	errHitCap          = errors.New("hit cap")
)

func workspaceConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOSO_WORKSPACE")) != ""
}

func pathArg(args map[string]any) string {
	if args == nil {
		return ""
	}
	v, ok := args["path"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func contentArg(args map[string]any) (string, bool) {
	if args == nil {
		return "", false
	}
	v, ok := args["content"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func toolErr(name, status, msg string) *connector.InvokeResult {
	return &connector.InvokeResult{
		Tool:      name,
		Connector: ConnectorName,
		Status:    status,
		Content:   map[string]any{"error": msg},
	}
}

func isLoop(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ELOOP) {
		return true
	}
	var pe *os.PathError
	if errors.As(err, &pe) && (errors.Is(pe.Err, syscall.ELOOP) || pe.Err == syscall.ELOOP) {
		return true
	}
	return false
}

func mapFSErr(name string, err error) *connector.InvokeResult {
	if err == nil {
		return notConfigured(name)
	}
	if errors.Is(err, errNotConfigured) {
		return notConfigured(name)
	}
	if errors.Is(err, errPathRequired) {
		return toolErr(name, "error", "path is required")
	}
	if errors.Is(err, errPathEscape) || isLoop(err) {
		return toolErr(name, "error", "path escape")
	}
	if errors.Is(err, errNotFile) {
		return toolErr(name, "error", "not a file")
	}
	if errors.Is(err, errNotDir) {
		return toolErr(name, "error", "not a directory")
	}
	if errors.Is(err, errTooLarge) {
		return toolErr(name, "error", "too large")
	}
	if errors.Is(err, errOldRequired) {
		return toolErr(name, "error", "old is required")
	}
	if errors.Is(err, errOldNotFound) {
		return toolErr(name, "error", "old not found")
	}
	if errors.Is(err, errNewRequired) {
		return toolErr(name, "error", "new is required")
	}
	if errors.Is(err, errQRequired) {
		return toolErr(name, "error", "q is required")
	}
	if errors.Is(err, errPatternRequired) {
		return toolErr(name, "error", "pattern is required")
	}
	if errors.Is(err, errInvalidPattern) {
		return toolErr(name, "error", "invalid pattern")
	}
	if os.IsNotExist(err) {
		return toolErr(name, "not_found", "not_found")
	}
	switch name {
	case ToolWriteFile, ToolEdit:
		return toolErr(name, "error", "write failed")
	case ToolListFiles, ToolGlob:
		return toolErr(name, "error", "list failed")
	case ToolSendFile:
		return toolErr(name, "error", "stat failed")
	case ToolSearch:
		return toolErr(name, "error", "search failed")
	default:
		return toolErr(name, "error", "read failed")
	}
}

func workspaceAbs() (string, error) {
	raw := strings.TrimSpace(os.Getenv("GOSO_WORKSPACE"))
	if raw == "" {
		return "", errNotConfigured
	}
	if security.HasDotDot(raw) {
		return "", errPathEscape
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", errPathEscape
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		abs = ev
	}
	return abs, nil
}

func confineUnder(root, p string) error {
	if security.HasDotDot(p) {
		return errPathEscape
	}
	if err := security.Confine(p); err != nil {
		return errPathEscape
	}
	absRoot := canonPath(root)
	absP := canonPath(p)
	rel, err := filepath.Rel(absRoot, absP)
	if err != nil {
		return errPathEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errPathEscape
	}
	return nil
}

func canonPath(p string) string {
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

func jailPath(arg string) (abs string, rel string, err error) {
	root, err := workspaceAbs()
	if err != nil {
		return "", "", err
	}
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", "", errPathRequired
	}
	if strings.IndexByte(arg, 0) >= 0 {
		return "", "", errPathEscape
	}
	if security.HasDotDot(arg) {
		return "", "", errPathEscape
	}
	var cand string
	if filepath.IsAbs(arg) {
		cand = arg
	} else {
		cand = filepath.Join(root, arg)
	}
	cand, err = filepath.Abs(cand)
	if err != nil {
		return "", "", errPathEscape
	}
	if err := confineUnder(root, cand); err != nil {
		return "", "", err
	}
	canonCand := canonPath(cand)
	if err := confineUnder(root, canonCand); err != nil {
		return "", "", err
	}
	rel, err = filepath.Rel(canonPath(root), canonCand)
	if err != nil {
		return "", "", errPathEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", errPathEscape
	}
	return canonCand, filepath.ToSlash(rel), nil
}

func readFile(args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolReadFile), nil
	}
	abs, rel, err := jailPath(pathArg(args))
	if err != nil {
		return mapFSErr(ToolReadFile, err), nil
	}
	f, st, err := openRegular(abs, os.O_RDONLY)
	if err != nil {
		return mapFSErr(ToolReadFile, err), nil
	}
	defer f.Close()
	if st.Size() > MaxReadBytes {
		return toolErr(ToolReadFile, "error", "too large"), nil
	}
	body, err := io.ReadAll(io.LimitReader(f, MaxReadBytes+1))
	if err != nil {
		return toolErr(ToolReadFile, "error", "read failed"), nil
	}
	if len(body) > MaxReadBytes {
		return toolErr(ToolReadFile, "error", "too large"), nil
	}
	return &connector.InvokeResult{
		Tool:      ToolReadFile,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"path":    rel,
			"content": string(body),
		},
	}, nil
}

func writeFile(args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolWriteFile), nil
	}
	content, ok := contentArg(args)
	if !ok {
		return toolErr(ToolWriteFile, "error", "content is required"), nil
	}
	abs, rel, err := jailPath(pathArg(args))
	if err != nil {
		return mapFSErr(ToolWriteFile, err), nil
	}
	root, err := workspaceAbs()
	if err != nil {
		return mapFSErr(ToolWriteFile, err), nil
	}
	dir := filepath.Dir(abs)
	if err := confineUnder(root, dir); err != nil {
		return mapFSErr(ToolWriteFile, err), nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return toolErr(ToolWriteFile, "error", "mkdir failed"), nil
	}
	if ev, err := filepath.EvalSymlinks(dir); err != nil {
		return mapFSErr(ToolWriteFile, errPathEscape), nil
	} else if err := confineUnder(root, ev); err != nil {
		return mapFSErr(ToolWriteFile, err), nil
	}
	if err := confineUnder(root, abs); err != nil {
		return mapFSErr(ToolWriteFile, err), nil
	}
	if err := writeRegular(abs, content); err != nil {
		return mapFSErr(ToolWriteFile, err), nil
	}
	return &connector.InvokeResult{
		Tool:      ToolWriteFile,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"path":  rel,
			"bytes": len(content),
		},
	}, nil
}

func openRegular(path string, flag int) (*os.File, os.FileInfo, error) {
	f, err := os.OpenFile(path, flag|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if isLoop(err) {
			return nil, nil, errPathEscape
		}
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, nil, errNotFile
	}
	return f, st, nil
}

func writeRegular(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		if isLoop(err) {
			return errPathEscape
		}
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errNotFile
	}
	_, err = f.Write([]byte(content))
	return err
}

func stringArg(args map[string]any, keys ...string) (string, bool) {
	if args == nil {
		return "", false
	}
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return "", false
		}
		return s, true
	}
	return "", false
}

func listPathArg(args map[string]any) string {
	p := pathArg(args)
	if p == "" {
		return "."
	}
	return p
}

func openDir(path string) (*os.File, os.FileInfo, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if isLoop(err) {
			return nil, nil, errPathEscape
		}
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.IsDir() {
		f.Close()
		return nil, nil, errNotDir
	}
	return f, st, nil
}

func fileMIME(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "application/octet-stream"
	}
	if m := mime.TypeByExtension(ext); m != "" {
		return m
	}
	return "application/octet-stream"
}

func listFiles(args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolListFiles), nil
	}
	abs, rel, err := jailPath(listPathArg(args))
	if err != nil {
		return mapFSErr(ToolListFiles, err), nil
	}
	f, _, err := openDir(abs)
	if err != nil {
		return mapFSErr(ToolListFiles, err), nil
	}
	defer f.Close()
	ents, err := f.ReadDir(maxListEnts + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return toolErr(ToolListFiles, "error", "list failed"), nil
	}
	truncated := false
	if len(ents) > maxListEnts {
		ents = ents[:maxListEnts]
		truncated = true
	}
	out := make([]map[string]any, 0, len(ents))
	for _, e := range ents {
		kind := "file"
		switch {
		case e.Type()&os.ModeSymlink != 0:
			kind = "symlink"
		case e.IsDir():
			kind = "dir"
		case !e.Type().IsRegular() && e.Type() != 0:
			kind = "other"
		}
		item := map[string]any{
			"name": e.Name(),
			"path": filepath.ToSlash(filepath.Join(rel, e.Name())),
			"type": kind,
		}
		if kind == "file" {
			if info, err := e.Info(); err == nil {
				item["bytes"] = info.Size()
			}
		}
		out = append(out, item)
	}
	content := map[string]any{
		"path":    rel,
		"entries": out,
	}
	if truncated {
		content["truncated"] = true
	}
	return &connector.InvokeResult{
		Tool:      ToolListFiles,
		Connector: ConnectorName,
		Status:    "ok",
		Content:   content,
	}, nil
}

func editFile(args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolEdit), nil
	}
	old, ok := stringArg(args, "old", "old_string")
	if !ok {
		return mapFSErr(ToolEdit, errOldRequired), nil
	}
	if old == "" {
		return mapFSErr(ToolEdit, errOldRequired), nil
	}
	newVal, ok := stringArg(args, "new", "new_string")
	if !ok {
		return mapFSErr(ToolEdit, errNewRequired), nil
	}
	abs, rel, err := jailPath(pathArg(args))
	if err != nil {
		return mapFSErr(ToolEdit, err), nil
	}
	f, st, err := openRegular(abs, os.O_RDONLY)
	if err != nil {
		return mapFSErr(ToolEdit, err), nil
	}
	if st.Size() > MaxReadBytes {
		f.Close()
		return toolErr(ToolEdit, "error", "too large"), nil
	}
	body, err := io.ReadAll(io.LimitReader(f, MaxReadBytes+1))
	f.Close()
	if err != nil {
		return toolErr(ToolEdit, "error", "read failed"), nil
	}
	if len(body) > MaxReadBytes {
		return toolErr(ToolEdit, "error", "too large"), nil
	}
	src := string(body)
	if !strings.Contains(src, old) {
		return mapFSErr(ToolEdit, errOldNotFound), nil
	}
	next := strings.Replace(src, old, newVal, 1)
	if err := writeRegular(abs, next); err != nil {
		return mapFSErr(ToolEdit, err), nil
	}
	return &connector.InvokeResult{
		Tool:      ToolEdit,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"path":      rel,
			"replaced":  1,
			"bytes":     len(next),
			"old_bytes": len(src),
		},
	}, nil
}

func sendFile(args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolSendFile), nil
	}
	abs, rel, err := jailPath(pathArg(args))
	if err != nil {
		return mapFSErr(ToolSendFile, err), nil
	}
	f, st, err := openRegular(abs, os.O_RDONLY)
	if err != nil {
		return mapFSErr(ToolSendFile, err), nil
	}
	_ = f.Close()
	return &connector.InvokeResult{
		Tool:      ToolSendFile,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"path":  rel,
			"bytes": st.Size(),
			"mime":  fileMIME(rel),
		},
	}, nil
}

func qArg(args map[string]any) string {
	s, ok := stringArg(args, "q")
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func patternArg(args map[string]any) string {
	s, ok := stringArg(args, "pattern")
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func searchFile(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolSearch), nil
	}
	q := qArg(args)
	if q == "" {
		return mapFSErr(ToolSearch, errQRequired), nil
	}
	abs, _, err := jailPath(listPathArg(args))
	if err != nil {
		return mapFSErr(ToolSearch, err), nil
	}
	root, err := workspaceAbs()
	if err != nil {
		return mapFSErr(ToolSearch, err), nil
	}
	qLower := strings.ToLower(q)
	hits := make([]map[string]any, 0)
	walkErr := filepath.WalkDir(abs, func(p string, d os.DirEntry, err error) error {
		if ctx != nil {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
		}
		if err != nil {
			if p == abs {
				return err
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if d.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			if confineUnder(root, p) != nil {
				return filepath.SkipDir
			}
			return nil
		}
		if len(hits) >= maxFSHits {
			return errHitCap
		}
		if !walkEntryRegular(d) {
			return nil
		}
		if confineUnder(root, p) != nil {
			return nil
		}
		fileHits, ok := searchRegularFile(p, root, qLower)
		if !ok {
			return nil
		}
		for _, h := range fileHits {
			hits = append(hits, h)
			if len(hits) >= maxFSHits {
				return errHitCap
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errHitCap) {
		return mapFSErr(ToolSearch, walkErr), nil
	}
	content := map[string]any{"hits": hits}
	if len(hits) >= maxFSHits {
		content["truncated"] = true
	}
	return &connector.InvokeResult{
		Tool:      ToolSearch,
		Connector: ConnectorName,
		Status:    "ok",
		Content:   content,
	}, nil
}

func searchRegularFile(abs, root, qLower string) ([]map[string]any, bool) {
	f, st, err := openRegular(abs, os.O_RDONLY)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	if st.Size() > MaxReadBytes {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(f, MaxReadBytes+1))
	if err != nil || len(body) > MaxReadBytes {
		return nil, false
	}
	sniff := body
	if len(sniff) > binarySniff {
		sniff = sniff[:binarySniff]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return nil, false
	}
	rel, err := filepath.Rel(canonPath(root), canonPath(abs))
	if err != nil {
		return nil, false
	}
	relSlash := filepath.ToSlash(rel)
	lines := strings.Split(string(body), "\n")
	out := make([]map[string]any, 0)
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if !strings.Contains(strings.ToLower(line), qLower) {
			continue
		}
		out = append(out, map[string]any{
			"path":    relSlash,
			"line":    i + 1,
			"snippet": clipSnippet(line),
		})
		if len(out) >= maxFSHits {
			break
		}
	}
	return out, true
}

func clipSnippet(s string) string {
	if len(s) <= maxSnippet {
		return s
	}
	return s[:maxSnippet]
}

func globFiles(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolGlob), nil
	}
	pattern := patternArg(args)
	if pattern == "" {
		return mapFSErr(ToolGlob, errPatternRequired), nil
	}
	if strings.IndexByte(pattern, 0) >= 0 || security.HasDotDot(pattern) {
		return mapFSErr(ToolGlob, errPathEscape), nil
	}
	if _, err := filepath.Match(pattern, "x"); err != nil {
		return mapFSErr(ToolGlob, errInvalidPattern), nil
	}
	root, err := workspaceAbs()
	if err != nil {
		return mapFSErr(ToolGlob, err), nil
	}
	abs, _, err := jailPath(".")
	if err != nil {
		return mapFSErr(ToolGlob, err), nil
	}
	hits := make([]map[string]any, 0)
	walkErr := filepath.WalkDir(abs, func(p string, d os.DirEntry, err error) error {
		if ctx != nil {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
		}
		if err != nil {
			if p == abs {
				return err
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if confineUnder(root, p) != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && d.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(canonPath(root), canonPath(p))
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." {
			return nil
		}
		ok, matchErr := globMatch(pattern, relSlash)
		if matchErr != nil {
			return errInvalidPattern
		}
		if !ok {
			return nil
		}
		hits = append(hits, map[string]any{"path": relSlash})
		if len(hits) >= maxListEnts {
			return errHitCap
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errHitCap) {
		return mapFSErr(ToolGlob, walkErr), nil
	}
	content := map[string]any{"hits": hits}
	if len(hits) >= maxListEnts {
		content["truncated"] = true
	}
	return &connector.InvokeResult{
		Tool:      ToolGlob,
		Connector: ConnectorName,
		Status:    "ok",
		Content:   content,
	}, nil
}

func walkEntryRegular(d os.DirEntry) bool {
	if d == nil {
		return false
	}
	t := d.Type()
	if t&os.ModeSymlink != 0 {
		return false
	}
	if t != 0 {
		return t.IsRegular()
	}
	info, err := d.Info()
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func globMatch(pattern, relSlash string) (bool, error) {
	ok, err := filepath.Match(pattern, relSlash)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	base := relSlash
	if i := strings.LastIndex(relSlash, "/"); i >= 0 {
		base = relSlash[i+1:]
	}
	return filepath.Match(pattern, base)
}
