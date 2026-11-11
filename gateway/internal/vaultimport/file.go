// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

const (
	sourcePresent = iota
	sourceMissing
	sourceSymlink
)

func checkSrcDir(srcDir string) error {
	info, err := os.Lstat(srcDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errConfig("source dir is not a directory")
	}
	return nil
}

// sourceFileState reports whether exportPath is a regular file inside srcDir.
// A missing file is sourceMissing. A symlink is not followed.
func sourceFileState(srcDir, exportPath string) (int, error) {
	rel := strings.TrimSpace(strings.ReplaceAll(exportPath, "\\", "/"))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || unsafePath(rel) != "" {
		return sourceMissing, nil
	}
	cleaned, ok := lexical(rel)
	if !ok || cleaned == "" {
		return sourceMissing, nil
	}
	root := strings.TrimSpace(srcDir)
	abs := filepath.Join(root, filepath.FromSlash(cleaned))
	relToRoot, err := filepath.Rel(root, abs)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return sourceMissing, nil
	}
	if err := walkNoSymlink(root, abs); err != nil {
		if errors.Is(err, errSymlink) {
			return sourceSymlink, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return sourceMissing, nil
		}
		return 0, err
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return sourceMissing, nil
	}
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return sourceSymlink, nil
	}
	if !info.Mode().IsRegular() {
		return sourceMissing, nil
	}
	return sourcePresent, nil
}

// publishFile writes body under the existing tenant namespace
// (GOSO_VAULT_DIR/t/<tenant>/<logical>). A matching file is left untouched.
// A symlink at the destination or on the way there is not followed and not replaced.
func publishFile(root, tenant, logical string, body []byte) (bool, error) {
	abs, err := resolveVaultFile(root, tenant, logical)
	if err != nil {
		return false, err
	}
	if err := ensureVaultRoot(root); err != nil {
		return false, err
	}
	if err := ensureDir(root, filepath.Dir(abs)); err != nil {
		return false, err
	}
	info, err := os.Lstat(abs)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false, errSymlink
		}
		got, err := readRegular(abs)
		if err != nil {
			return false, err
		}
		if bytes.Equal(got, body) {
			return false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := writeReplace(abs, body); err != nil {
		return false, err
	}
	return true, nil
}

// fileMatches reports whether the published file already equals body.
// A missing file is not a match. A symlink is errSymlink.
func fileMatches(root, tenant, logical string, body []byte) (bool, error) {
	if strings.TrimSpace(root) == "" {
		return true, nil
	}
	abs, err := resolveVaultFile(root, tenant, logical)
	if err != nil {
		return false, err
	}
	if err := walkNoSymlink(root, abs); err != nil {
		return false, err
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, errSymlink
	}
	got, err := readRegular(abs)
	if err != nil {
		return false, err
	}
	return bytes.Equal(got, body), nil
}

// resolveVaultFile is the destination path. It does not create directories and
// does not follow a symlink.
func resolveVaultFile(root, tenant, logical string) (string, error) {
	logical, err := vault.LogicalPath(logical)
	if err != nil {
		return "", err
	}
	dir, err := vault.StorageDir(root, tenant)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(dir, filepath.FromSlash(logical))
	rel, err := filepath.Rel(dir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errSymlink
	}
	if err := security.Confine(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// walkNoSymlink Lstats each existing component. A symlink is refused. A missing
// component stops the walk; the caller may create it.
func walkNoSymlink(root, abs string) error {
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errSymlink
	}
	cur := root
	info, err := os.Lstat(cur)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errSymlink
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
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errSymlink
		}
	}
	return nil
}

func ensureVaultRoot(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		parent := filepath.Dir(root)
		pinfo, perr := os.Lstat(parent)
		if perr != nil {
			return perr
		}
		if pinfo.Mode()&os.ModeSymlink != 0 || !pinfo.IsDir() {
			return errSymlink
		}
		return os.Mkdir(root, 0o755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errSymlink
	}
	return nil
}

func ensureDir(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errSymlink
	}
	cur := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return errSymlink
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(cur, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errSymlink
		}
	}
	return nil
}

func readRegular(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errSymlink
	}
	return io.ReadAll(f)
}

func writeReplace(path string, body []byte) error {
	info, err := os.Lstat(path)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return errSymlink
	}
	tmp := path + ".import-partial"
	if err := removeRegular(tmp); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = removeRegular(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = removeRegular(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = removeRegular(tmp)
		return err
	}
	return nil
}

func removeRegular(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errSymlink
	}
	return os.Remove(path)
}
