// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func normalizeVaultPath(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return "", errors.New("path is required")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(strings.ToLower(p), "file:") {
		return "", errors.New("path escape")
	}
	if len(p) >= 2 && p[1] == ':' {
		return "", errors.New("path escape")
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.Trim(p, "/")
	if p == "" || strings.Contains(p, "..") {
		return "", errors.New("path escape")
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", errors.New("path escape")
	}
	return cleaned, nil
}

func vaultRaw(inner string) string {
	inner = strings.TrimSpace(inner)
	return "[[" + inner + "]]"
}

func vaultInner(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[[")
	s = strings.TrimSuffix(s, "]]")
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "|"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func copyVaultDoc(d *VaultDoc) *VaultDoc {
	if d == nil {
		return nil
	}
	cp := *d
	return &cp
}

func (s *Store) resolveVaultLocked(tenant, inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	tenant = NormalizeTenant(tenant)
	var matches []*VaultDoc
	for _, d := range s.vaultDocs {
		if d != nil && SameTenant(d.TenantID, tenant) && strings.EqualFold(d.Title, inner) {
			matches = append(matches, d)
		}
	}
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].Path < matches[j].Path })
		return matches[0].ID
	}
	if d, ok := s.vaultDocs[inner]; ok && d != nil && SameTenant(d.TenantID, tenant) {
		return d.ID
	}
	return ""
}

func (s *Store) reResolveVaultLocked() {
	for from, list := range s.vaultLinks {
		tenant := DefaultTenant
		if d := s.vaultDocs[from]; d != nil {
			tenant = d.TenantID
		}
		for i := range list {
			inner := vaultInner(list[i].Raw)
			list[i].ToID = s.resolveVaultLocked(tenant, inner)
			list[i].FromID = from
		}
		s.vaultLinks[from] = list
	}
}

func (s *Store) PutVaultDoc(d VaultDoc) (*VaultDoc, error) {
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		return nil, errors.New("title is required")
	}
	path, err := normalizeVaultPath(d.Path)
	if err != nil {
		return nil, err
	}
	d.Path = path
	d.TenantID = NormalizeTenant(d.TenantID)
	if d.Mtime.IsZero() {
		d.Mtime = time.Now().UTC()
	} else {
		d.Mtime = d.Mtime.UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var existing *VaultDoc
	if d.ID != "" {
		existing = s.vaultDocs[d.ID]
		if existing != nil && !SameTenant(existing.TenantID, d.TenantID) {
			return nil, ErrConflict
		}
	}
	if existing == nil {
		for _, v := range s.vaultDocs {
			if v != nil && v.Path == d.Path && SameTenant(v.TenantID, d.TenantID) {
				existing = v
				break
			}
		}
	}
	if existing != nil {
		if !existing.OwnerExplicit {
			return nil, ErrConflict
		}
		existing.Title = d.Title
		existing.Path = d.Path
		existing.SHA256 = d.SHA256
		existing.Mtime = d.Mtime
		existing.Body = d.Body
		if d.OwnerExplicit {
			existing.OwnerExplicit = true
		}
		s.reResolveVaultLocked()
		return copyVaultDoc(existing), nil
	}
	d.ID = s.nextID()
	cp := d
	s.vaultDocs[cp.ID] = &cp
	s.reResolveVaultLocked()
	return copyVaultDoc(&cp), nil
}

func (s *Store) GetVaultDoc(id string) (*VaultDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.vaultDocs[id]
	if !ok || d == nil {
		return nil, ErrNotFound
	}
	return copyVaultDoc(d), nil
}

func (s *Store) ListVaultDocs() []*VaultDoc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*VaultDoc, 0, len(s.vaultDocs))
	for _, d := range s.vaultDocs {
		out = append(out, copyVaultDoc(d))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Title != out[j].Title {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].Path < out[j].Path
	})
	if out == nil {
		out = []*VaultDoc{}
	}
	return out
}

func (s *Store) FindVaultDocByPath(tenant, path string) (*VaultDoc, error) {
	path, err := normalizeVaultPath(path)
	if err != nil {
		return nil, err
	}
	tenant = NormalizeTenant(tenant)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.vaultDocs {
		if d != nil && d.Path == path && SameTenant(d.TenantID, tenant) {
			return copyVaultDoc(d), nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) FindVaultDocByTitle(tenant, title string) (*VaultDoc, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	tenant = NormalizeTenant(tenant)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []*VaultDoc
	for _, d := range s.vaultDocs {
		if d != nil && SameTenant(d.TenantID, tenant) && strings.EqualFold(d.Title, title) {
			matches = append(matches, d)
		}
	}
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Path < matches[j].Path })
	return copyVaultDoc(matches[0]), nil
}

func (s *Store) DeleteVaultDoc(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.vaultDocs[id]; !ok {
		return ErrNotFound
	}
	delete(s.vaultDocs, id)
	delete(s.vaultLinks, id)
	for from, list := range s.vaultLinks {
		for i := range list {
			if list[i].ToID == id {
				list[i].ToID = ""
			}
		}
		s.vaultLinks[from] = list
	}
	return nil
}

func (s *Store) ReplaceVaultLinks(fromID string, raws []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.vaultDocs[fromID]; !ok {
		return ErrNotFound
	}
	seen := make(map[string]struct{})
	out := make([]VaultLink, 0, len(raws))
	for _, inner := range raws {
		inner = strings.TrimSpace(inner)
		if inner == "" {
			continue
		}
		raw := vaultRaw(inner)
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		tenant := DefaultTenant
		if doc := s.vaultDocs[fromID]; doc != nil {
			tenant = doc.TenantID
		}
		out = append(out, VaultLink{
			FromID: fromID,
			ToID:   s.resolveVaultLocked(tenant, inner),
			Raw:    raw,
		})
	}
	s.vaultLinks[fromID] = out
	return nil
}

func (s *Store) ListVaultDocLinks(id string) (outbound, inbound []VaultLink, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.vaultDocs[id]
	if !ok || doc == nil {
		return nil, nil, ErrNotFound
	}
	ob := s.vaultLinks[id]
	outbound = make([]VaultLink, 0, len(ob))
	for _, l := range ob {
		if l.ToID != "" {
			t := s.vaultDocs[l.ToID]
			if t == nil || !SameTenant(t.TenantID, doc.TenantID) || !t.OwnerExplicit {
				l.ToID = ""
			}
		}
		outbound = append(outbound, l)
	}
	inbound = []VaultLink{}
	for from, list := range s.vaultLinks {
		src := s.vaultDocs[from]
		if src == nil || !SameTenant(src.TenantID, doc.TenantID) || !src.OwnerExplicit {
			continue
		}
		for _, l := range list {
			if l.ToID == id {
				inbound = append(inbound, l)
			}
		}
	}
	if outbound == nil {
		outbound = []VaultLink{}
	}
	return outbound, inbound, nil
}

func (s *Store) SearchVault(tenant, q string) ([]VaultSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []VaultSearchHit{}, nil
	}
	tenant = NormalizeTenant(tenant)
	needle := strings.ToLower(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []VaultSearchHit
	for _, d := range s.vaultDocs {
		if d == nil || !SameTenant(d.TenantID, tenant) || !d.OwnerExplicit {
			continue
		}
		inTitle := strings.Contains(strings.ToLower(d.Title), needle)
		inBody := strings.Contains(strings.ToLower(d.Body), needle)
		if !inTitle && !inBody {
			continue
		}
		hay := d.Body
		if !inBody {
			hay = d.Title
		}
		out = append(out, VaultSearchHit{
			ID: d.ID, Title: d.Title, Path: d.Path,
			Snippet: SnippetAround(hay, q, 80),
			SHA256:  d.SHA256,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	if len(out) > VaultSearchLimit {
		out = out[:VaultSearchLimit]
	}
	if out == nil {
		out = []VaultSearchHit{}
	}
	return out, nil
}

func (s *Store) ReResolveVaultLinks() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reResolveVaultLocked()
	return nil
}
