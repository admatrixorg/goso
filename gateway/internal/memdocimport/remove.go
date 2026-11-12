// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// RemoveResult is one per-tenant gc- removal.
type RemoveResult struct {
	Tenant    string `json:"tenant"`
	Memories  int    `json:"memories"`
	Sessions  int    `json:"sessions"`
	VaultDocs int    `json:"vault_docs"`
	Files     int    `json:"files"`
}

// RemoveTenant deletes rows in tenant whose id starts with gc-, and the vault
// files for those vault rows. A session that still holds a note or a message
// is kept. Rows whose id does not start with gc-, and every other tenant, are
// left in place.
func RemoveTenant(st Store, vaultRoot, tenant string) (RemoveResult, error) {
	if st == nil {
		return RemoveResult{}, errConfig("store is required")
	}
	tenant = strings.TrimSpace(tenant)
	if !store.TenantOK(tenant) {
		return RemoveResult{}, errConfig("tenant id is empty or not a goso tenant id")
	}
	vaultRoot = strings.TrimSpace(vaultRoot)
	if vaultRoot == "" {
		return RemoveResult{}, errConfig("vault dir is required")
	}
	docs, err := st.ListImportedMemdocVault(tenant)
	if err != nil {
		return RemoveResult{}, err
	}
	for _, d := range docs {
		if _, err := checkVaultFile(vaultRoot, tenant, d.Path, false); err != nil {
			return RemoveResult{}, err
		}
	}
	removed := 0
	for _, d := range docs {
		ok, err := checkVaultFile(vaultRoot, tenant, d.Path, true)
		if err != nil {
			return RemoveResult{}, err
		}
		if ok {
			removed++
		}
	}
	counts, err := st.DeleteImportedMemdocs(tenant)
	if err != nil {
		return RemoveResult{}, err
	}
	return RemoveResult{
		Tenant:    store.NormalizeTenant(tenant),
		Memories:  counts.Memories,
		Sessions:  counts.Sessions,
		VaultDocs: counts.VaultDocs,
		Files:     removed,
	}, nil
}

// checkVaultFile checks one registry path. remove deletes a regular file.
// A missing file is not an error. A symlink is refused and is not followed.
// The bool reports whether a file was deleted.
func checkVaultFile(root, tenant, logical string, remove bool) (bool, error) {
	logical, err := vault.LogicalPath(logical)
	if err != nil {
		return false, err
	}
	tenant = store.NormalizeTenant(tenant)
	if !store.TenantOK(tenant) || strings.Contains(tenant, "..") || strings.ContainsAny(tenant, `/\`) {
		return false, errors.New("path escape")
	}
	dir := filepath.Join(root, "t", tenant)
	abs := filepath.Join(dir, filepath.FromSlash(logical))
	rel, err := filepath.Rel(dir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false, errors.New("path escape")
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("symlink")
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("not a file")
	}
	if !remove {
		return false, nil
	}
	if err := os.Remove(abs); err != nil {
		return false, err
	}
	return true, nil
}
