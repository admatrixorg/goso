// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/skill"
)

var errSymlink = errors.New("symlink")

// readSkillBytes reads one skill file as data. It never executes the file
// or any sibling. A directory path reads only SKILL.md inside that directory.
func readSkillBytes(srcDir, exportPath string) ([]byte, string, error) {
	full, reason, err := resolveSource(srcDir, exportPath)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "path-missing", nil
	}
	if err != nil {
		return nil, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, "symlink", nil
	}
	if info.IsDir() {
		full = filepath.Join(full, skillFile)
		info, err = os.Lstat(full)
		if errors.Is(err, os.ErrNotExist) {
			return nil, "path-missing", nil
		}
		if err != nil {
			return nil, "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, "symlink", nil
		}
		if !info.Mode().IsRegular() {
			return nil, "path-missing", nil
		}
	} else if !info.Mode().IsRegular() {
		return nil, "path-missing", nil
	}
	if info.Size() > skill.MaxBody {
		return nil, "too-large", nil
	}
	if info.Size() == 0 {
		return nil, "empty-body", nil
	}
	f, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "path-missing", nil
	}
	if err != nil {
		return nil, "symlink", nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		return nil, "symlink", nil
	}
	raw, err := io.ReadAll(io.LimitReader(f, skill.MaxBody+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(raw)) > skill.MaxBody {
		return nil, "too-large", nil
	}
	if len(raw) == 0 {
		return nil, "empty-body", nil
	}
	if bytes.Contains(raw, []byte{0}) || !utf8.Valid(raw) {
		return nil, "binary-skipped", nil
	}
	body := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))
	if len(body) == 0 {
		return nil, "empty-body", nil
	}
	return body, "", nil
}

func resolveSource(srcDir, exportPath string) (string, string, error) {
	rel := strings.TrimSpace(strings.ReplaceAll(exportPath, "\\", "/"))
	if rel == "" {
		return "", "path-missing", nil
	}
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(exportPath) {
		return "", "path-escape", nil
	}
	if security.HasDotDot(rel) {
		return "", "path-escape", nil
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", "path-escape", nil
	}
	root, err := filepath.Abs(strings.TrimSpace(srcDir))
	if err != nil {
		return "", "", err
	}
	full := filepath.Join(root, filepath.FromSlash(cleaned))
	back, err := filepath.Rel(root, full)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(os.PathSeparator)) {
		return "", "path-escape", nil
	}
	if err := walkNoSymlink(root, full); err != nil {
		if errors.Is(err, errSymlink) {
			return "", "symlink", nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return "", "path-missing", nil
		}
		return "", "", err
	}
	return full, "", nil
}

func walkNoSymlink(root, full string) error {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return err
	}
	cur := root
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return errSymlink
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errSymlink
		}
	}
	return nil
}
