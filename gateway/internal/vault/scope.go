// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ErrSyncDisabled is returned when vault_scoped_sync is off, and by the
// unscoped Sync entry. Callers must not fall back to a global reconcile.
var ErrSyncDisabled = errors.New("vault sync disabled")

// ErrTargetTenant is a forged or mismatched target tenant.
var ErrTargetTenant = errors.New("target tenant denied")

// ErrScan is a directory walk that did not finish. Sync must not delete.
var ErrScan = errors.New("vault scan failed")

type diskFile struct {
	Rel  string
	Body []byte
	Hash string
	Mod  os.FileInfo
}

// InventoryEntry is one dry-run row. Action quarantine is a recommendation.
// Applying it, moving the file, or assigning an owner is out of scope.
type InventoryEntry struct {
	DocID  string `json:"doc_id,omitempty"`
	Path   string `json:"path"`
	Tenant string `json:"tenant_id,omitempty"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// Inventory is a read-only mapping for one resolved tenant.
type Inventory struct {
	DryRun  bool             `json:"dry_run"`
	Applied bool             `json:"applied"`
	Tenant  string           `json:"tenant_id"`
	Entries []InventoryEntry `json:"entries"`
}

// ScopedSyncEnabled reports the vault_scoped_sync flag (GOSO_VAULT_SCOPED_SYNC).
// Default is off. Off blocks sync; it does not restore a global scan.
func ScopedSyncEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_VAULT_SCOPED_SYNC"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// StorageDir is the server-assigned directory for one tenant. The logical
// path is appended under it and is not itself a tenant, agent, project, or
// customer grant.
func StorageDir(root, tenant string) (string, error) {
	return (&Service{Root: root}).tenantDir(tenant)
}

func (s *Service) tenantDir(tenant string) (string, error) {
	tenant = store.NormalizeTenant(tenant)
	if !store.TenantOK(tenant) || strings.Contains(tenant, "..") || strings.ContainsAny(tenant, `/\`) {
		return "", errors.New("path escape")
	}
	dir := filepath.Join(s.Root, "t", tenant)
	if !underRoot(s.Root, dir) {
		return "", errors.New("path escape")
	}
	return dir, nil
}

// LogicalPath rejects absolute paths and traversal. It does not grant scope.
func LogicalPath(rel string) (string, error) { return logicalPath(rel) }

func logicalPath(rel string) (string, error) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(strings.ToLower(rel), "file:") {
		return "", errors.New("path escape")
	}
	if len(rel) >= 2 && rel[1] == ':' {
		return "", errors.New("path escape")
	}
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.Trim(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return "", errors.New("path escape")
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", errors.New("path escape")
	}
	return cleaned, nil
}

func (s *Service) namespaced(tenant, logical string) (string, error) {
	logical, err := logicalPath(logical)
	if err != nil {
		return "", err
	}
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(dir, filepath.FromSlash(logical))
	if !underRoot(dir, abs) {
		return "", errors.New("path escape")
	}
	if err := security.Confine(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func fileInsideRoot(root, path string) bool {
	return fileInside(root, path)
}

func fileInside(root, path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	target := path
	if info.Mode()&os.ModeSymlink != 0 {
		ev, err := filepath.EvalSymlinks(path)
		if err != nil {
			return false
		}
		target = ev
	}
	return underRoot(root, target)
}

func pathNotAuthority(p string) bool {
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" || !strings.Contains(p, "/") {
		return false
	}
	seg := strings.ToLower(strings.Split(p, "/")[0])
	switch seg {
	case "agent", "agents", "project", "projects", "customer", "customers", "area", "areas":
		return true
	default:
		return false
	}
}

// legacyOwned reports a pre-namespace file that this row alone still names.
// A shared logical path never reads or deletes that file.
func (s *Service) legacyOwned(d *store.VaultDoc) bool {
	if d == nil || d.Path == "" {
		return false
	}
	if _, err := logicalPath(d.Path); err != nil {
		return false
	}
	abs := filepath.Join(s.Root, filepath.FromSlash(d.Path))
	if !fileInsideRoot(s.Root, abs) {
		return false
	}
	rel, err := filepath.Rel(s.Root, abs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "t" || strings.HasPrefix(rel, "t/") {
		return false
	}
	info, err := os.Lstat(abs)
	if err != nil || info.IsDir() {
		return false
	}
	for _, other := range s.List() {
		if other != nil && other.ID != d.ID && other.Path == d.Path && !store.SameTenant(other.TenantID, d.TenantID) {
			return false
		}
	}
	return true
}

func (s *Service) locate(d *store.VaultDoc) (string, bool) {
	if d == nil {
		return "", false
	}
	if abs, err := s.namespaced(d.TenantID, d.Path); err == nil && fileInside(filepath.Dir(abs), abs) {
		if info, err := os.Lstat(abs); err == nil && !info.IsDir() {
			return abs, true
		}
	}
	if s.legacyOwned(d) {
		abs := filepath.Join(s.Root, filepath.FromSlash(d.Path))
		if fileInsideRoot(s.Root, abs) {
			return abs, true
		}
	}
	return "", false
}

func (s *Service) collectTenant(tenant string) ([]diskFile, error) {
	if s.collectHook != nil {
		return s.collectHook(tenant)
	}
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return nil, err
	}
	if err := security.Confine(dir); err != nil {
		return nil, err
	}
	st, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []diskFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrScan
	}
	var out []diskFile
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dir {
			return nil
		}
		if d.IsDir() {
			if d.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		if !fileInside(dir, path) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, err := logicalPath(rel); err != nil {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		out = append(out, diskFile{Rel: rel, Body: body, Hash: sha256Hex(body), Mod: info})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SyncTenant reconciles one tenant's namespace. An empty, failed, or partial
// scan returns before any delete. Rows outside the tenant, and rows whose
// owner was not recorded by a scoped write, are left in place.
func (s *Service) SyncTenant(tenant string) (*SyncResult, error) {
	if !ScopedSyncEnabled() {
		return nil, ErrSyncDisabled
	}
	tenant = store.NormalizeTenant(tenant)
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.collectTenant(tenant)
	if err != nil {
		return nil, err
	}
	res := &SyncResult{}
	seen := make(map[string]struct{}, len(files))
	for _, f := range files {
		seen[f.Rel] = struct{}{}
		existing, err := s.Store.FindVaultDocByPath(tenant, f.Rel)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if existing != nil && !store.SameTenant(existing.TenantID, tenant) {
			continue
		}
		if existing != nil && existing.SHA256 == f.Hash {
			res.Skipped++
			continue
		}
		body := string(f.Body)
		title := titleFrom(f.Rel, body)
		if existing != nil && !hasH1(body) {
			title = existing.Title
		}
		doc := store.VaultDoc{
			Title:         title,
			Path:          f.Rel,
			SHA256:        f.Hash,
			Mtime:         f.Mod.ModTime().UTC(),
			Body:          body,
			TenantID:      tenant,
			OwnerExplicit: true,
		}
		if existing != nil {
			doc.ID = existing.ID
		}
		saved, err := s.Store.PutVaultDoc(doc)
		if err != nil {
			return nil, err
		}
		if err := s.refreshLinks(saved.ID, body); err != nil {
			return nil, err
		}
		res.Upserted++
	}
	if err := s.Store.ReResolveVaultLinks(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return res, nil
	}
	for _, d := range s.List() {
		if d == nil || !store.SameTenant(d.TenantID, tenant) || !d.OwnerExplicit {
			continue
		}
		if _, ok := seen[d.Path]; ok {
			continue
		}
		if s.legacyOwned(d) {
			continue
		}
		if err := s.Store.DeleteVaultDoc(d.ID); err != nil {
			return nil, err
		}
		res.Deleted++
	}
	return res, nil
}

// Inventory lists this tenant's mapping. It does not write, assign, move, or
// delete. Other tenants' paths and bodies are omitted.
func (s *Service) Inventory(tenant string) (Inventory, error) {
	tenant = store.NormalizeTenant(tenant)
	out := Inventory{DryRun: true, Applied: false, Tenant: tenant, Entries: []InventoryEntry{}}
	for _, d := range s.List() {
		if d == nil || !store.SameTenant(d.TenantID, tenant) {
			continue
		}
		e := InventoryEntry{DocID: d.ID, Path: d.Path, Tenant: store.NormalizeTenant(d.TenantID)}
		note := ""
		if pathNotAuthority(d.Path) {
			note = "path_not_authority"
		}
		if d.OwnerExplicit {
			e.Action = "mapped"
			e.Reason = "registry_explicit"
		} else {
			e.Action = "quarantine"
			e.Reason = "unclear_owner"
		}
		if note != "" {
			e.Reason += "; " + note
		}
		out.Entries = append(out.Entries, e)
	}
	files, err := s.collectTenant(tenant)
	if err != nil {
		return Inventory{}, err
	}
	have := map[string]struct{}{}
	for _, e := range out.Entries {
		have[e.Path] = struct{}{}
	}
	for _, f := range files {
		if _, ok := have[f.Rel]; ok {
			continue
		}
		reason := "unregistered_file"
		if pathNotAuthority(f.Rel) {
			reason += "; path_not_authority"
		}
		out.Entries = append(out.Entries, InventoryEntry{Path: f.Rel, Action: "quarantine", Reason: reason})
	}
	return out, nil
}
