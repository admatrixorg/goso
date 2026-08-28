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
	p = strings.TrimPrefix(p, "./")
	p = strings.Trim(p, "/")
	if p == "" {
		return "", errors.New("path is required")
	}
	if strings.Contains(p, "..") {
		return "", errors.New("path escape")
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
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

func (s *Store) resolveVaultLocked(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	for _, d := range s.vaultDocs {
		if d != nil && strings.EqualFold(d.Title, inner) {
			return d.ID
		}
	}
	if d, ok := s.vaultDocs[inner]; ok && d != nil {
		return d.ID
	}
	return ""
}

func (s *Store) reResolveVaultLocked() {
	for from, list := range s.vaultLinks {
		for i := range list {
			inner := vaultInner(list[i].Raw)
			list[i].ToID = s.resolveVaultLocked(inner)
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
	}
	if existing == nil {
		for _, v := range s.vaultDocs {
			if v != nil && v.Path == d.Path {
				existing = v
				break
			}
		}
	}
	if existing != nil {
		existing.Title = d.Title
		existing.Path = d.Path
		existing.SHA256 = d.SHA256
		existing.Mtime = d.Mtime
		existing.Body = d.Body
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

func (s *Store) FindVaultDocByPath(path string) (*VaultDoc, error) {
	path, err := normalizeVaultPath(path)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.vaultDocs {
		if d != nil && d.Path == path {
			return copyVaultDoc(d), nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) FindVaultDocByTitle(title string) (*VaultDoc, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matches []*VaultDoc
	for _, d := range s.vaultDocs {
		if d != nil && strings.EqualFold(d.Title, title) {
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
		out = append(out, VaultLink{
			FromID: fromID,
			ToID:   s.resolveVaultLocked(inner),
			Raw:    raw,
		})
	}
	s.vaultLinks[fromID] = out
	return nil
}

func (s *Store) ListVaultDocLinks(id string) (outbound, inbound []VaultLink, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.vaultDocs[id]; !ok {
		return nil, nil, ErrNotFound
	}
	ob := s.vaultLinks[id]
	outbound = make([]VaultLink, len(ob))
	copy(outbound, ob)
	inbound = []VaultLink{}
	for _, list := range s.vaultLinks {
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

func (s *Store) SearchVault(q string) ([]VaultSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []VaultSearchHit{}, nil
	}
	needle := strings.ToLower(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []VaultSearchHit
	for _, d := range s.vaultDocs {
		if d == nil {
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
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
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
