// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

const (
	skillFile     = "SKILL.md"
	sourceMapFile = ".goso-skill-import.json"
	mapVersion    = 1
)

type sourceMap struct {
	Version int           `json:"version"`
	Entries []sourceEntry `json:"entries"`
}

type sourceEntry struct {
	TenantID string `json:"tenant_id"`
	SourceID string `json:"source_id"`
	Old      string `json:"old"`
	Name     string `json:"name"`
	Sum      string `json:"sum"`
}

func sumBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func loadSourceMap(skillsDir string) (sourceMap, error) {
	var m sourceMap
	p := filepath.Join(skillsDir, sourceMapFile)
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		m.Version = mapVersion
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return m, errConfig("source map is not a regular file")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, errConfig("source map is unreadable")
	}
	if m.Version != mapVersion {
		return m, errConfig("source map version is not supported")
	}
	return m, nil
}

func saveSourceMap(skillsDir string, m sourceMap) error {
	m.Version = mapVersion
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeAtomic(filepath.Join(skillsDir, sourceMapFile), raw)
}

func writeAtomic(path string, raw []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func existingNames(skillsDir string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	info, err := os.Lstat(skillsDir)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errConfig("skills dir is not a directory")
	}
	ents, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		name := e.Name()
		if name == sourceMapFile || name == sourceMapFile+".tmp" {
			continue
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func writeNewSkill(skillsDir, name string, body []byte) error {
	dir := filepath.Join(skillsDir, name)
	file := filepath.Join(dir, skillFile)
	root, err := filepath.Abs(skillsDir)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, absDir)
	if err != nil || rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator) || rel != name {
		return errConfig("skill path escaped the skills dir")
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func skillFileBytes(skillsDir, name string) ([]byte, error) {
	p := filepath.Join(skillsDir, name, skillFile)
	info, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errSymlink
	}
	return os.ReadFile(p)
}
