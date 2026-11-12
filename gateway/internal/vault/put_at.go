// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// PutTenantAt writes one logical path and its vault_docs row in one step.
// path is inside the tenant directory. It must already be a logical path:
// escape and symlinks are refused, and raw/ is refused. The registry row is
// committed before the file is published. A database error leaves no file.
// If publishing the file fails, the previous row is restored, or the new row
// is deleted, and the temp file is removed. A link refresh error after the
// file and row are both published is logged and is not a write failure.
func (s *Service) PutTenantAt(tenant, path string, body []byte) (*store.VaultDoc, error) {
	if body == nil {
		body = []byte{}
	}
	tenant = store.NormalizeTenant(tenant)
	logical, err := logicalPath(path)
	if err != nil {
		return nil, err
	}
	if UnderRaw(logical) {
		return nil, errors.New("raw path denied")
	}
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return nil, err
	}
	if err := security.Confine(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	unlock := lockNamespace(s.Root)
	defer unlock()
	abs, err := s.namespaced(tenant, logical)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	text := string(body)
	title := titleFrom(logical, text)
	existing, err := s.Store.FindVaultDocByPath(tenant, logical)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if existing != nil && !store.SameTenant(existing.TenantID, tenant) {
		return nil, ErrTargetTenant
	}
	tmp, err := writeExclusive(filepath.Dir(abs), filepath.Base(abs)+".partial", body)
	if err != nil {
		return nil, err
	}
	doc := store.VaultDoc{
		Title:         title,
		Path:          filepath.ToSlash(logical),
		SHA256:        sha256Hex(body),
		Mtime:         time.Now().UTC(),
		Body:          text,
		TenantID:      tenant,
		OwnerExplicit: true,
	}
	if existing != nil {
		if !existing.OwnerExplicit {
			removeRegular(tmp)
			return nil, store.ErrConflict
		}
		doc.ID = existing.ID
	}
	saved, err := s.Store.PutVaultDoc(doc)
	if err != nil {
		removeRegular(tmp)
		return nil, err
	}
	if barrierAfterCommit != nil {
		barrierAfterCommit()
	}
	if err := publishRename(tmp, abs); err != nil {
		removeRegular(tmp)
		if rbErr := s.rollbackPut(existing, saved.ID); rbErr != nil {
			// Keep the rename error. A rollback failure must not replace it.
			return nil, errors.Join(err, rbErr)
		}
		return nil, err
	}
	if err := s.refreshLinks(saved.ID, text); err != nil {
		// The file and the row are already the published pair. A stale
		// wikilink index is not a failed write.
		log.Printf("vault: links not refreshed id=%s", saved.ID)
		return saved, nil
	}
	_ = s.Store.ReResolveVaultLinks()
	return saved, nil
}

// rollbackPut undoes a registry commit that never reached disk.
// previous is the row that occupied the path, or nil when the path was new.
func (s *Service) rollbackPut(previous *store.VaultDoc, newID string) error {
	if previous != nil {
		_, err := s.Store.PutVaultDoc(*previous)
		return err
	}
	if newID == "" {
		return nil
	}
	err := s.Store.DeleteVaultDoc(newID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}
