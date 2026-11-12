// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ingestDirs is the folder tree later specs fill in. This PR only creates it.
var ingestDirs = []string{
	"raw",
	"sources/inbox",
	"sources/projects",
	"sources/areas",
	"sources/resources",
	"sources/archive",
	"wiki/topics",
}

// wikiStubs are empty pages the code owns until SPEC 174 writes them.
var wikiStubs = []string{
	"wiki/schema.md",
	"wiki/index.md",
	"wiki/log.md",
}

// EnsureIngestTree creates the ingest folders for one tenant and the empty
// wiki pages when they are not already there. It does not overwrite a page.
func (s *Service) EnsureIngestTree(tenant string) error {
	tenant = store.NormalizeTenant(tenant)
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return err
	}
	if err := security.Confine(dir); err != nil {
		return err
	}
	for _, rel := range ingestDirs {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := rejectSymlinks(dir, abs); err != nil {
			return err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return err
		}
	}
	for _, rel := range wikiStubs {
		abs, err := s.namespaced(tenant, rel)
		if err != nil {
			return err
		}
		info, err := os.Lstat(abs)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return errors.New("path escape")
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := s.PutTenantAt(tenant, rel, nil); err != nil {
			return err
		}
	}
	return nil
}
