// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"errors"
	"io"
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
	// MaxReadBytes is the read_file cap. Larger files are rejected, not truncated.
	MaxReadBytes = 1 << 20
)

var (
	errNotConfigured = errors.New("not_configured")
	errPathEscape    = errors.New("path escape")
	errPathRequired  = errors.New("path is required")
	errNotFile       = errors.New("not a file")
	errTooLarge      = errors.New("too large")
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
	if errors.Is(err, errTooLarge) {
		return toolErr(name, "error", "too large")
	}
	if os.IsNotExist(err) {
		return toolErr(name, "not_found", "not_found")
	}
	if name == ToolWriteFile {
		return toolErr(name, "error", "write failed")
	}
	return toolErr(name, "error", "read failed")
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
