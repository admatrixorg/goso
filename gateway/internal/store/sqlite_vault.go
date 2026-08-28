// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *SQLiteStore) resolveVaultInner(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	var id string
	err := s.db.QueryRow(`SELECT id FROM vault_docs WHERE lower(title)=lower(?) ORDER BY path LIMIT 1`, inner).Scan(&id)
	if err == nil && id != "" {
		return id
	}
	err = s.db.QueryRow(`SELECT id FROM vault_docs WHERE id=?`, inner).Scan(&id)
	if err == nil && id != "" {
		return id
	}
	return ""
}

func scanVaultDoc(sc scanner) (*VaultDoc, error) {
	var d VaultDoc
	var ts string
	var body, tenant sql.NullString
	if err := sc.Scan(&d.ID, &d.Title, &d.Path, &d.SHA256, &ts, &body, &tenant); err != nil {
		return nil, err
	}
	d.Mtime = parseTime(ts)
	d.TenantID = NormalizeTenant(tenant.String)
	if body.Valid {
		d.Body = body.String
	}
	return &d, nil
}

func (s *SQLiteStore) PutVaultDoc(d VaultDoc) (*VaultDoc, error) {
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

	var existing *VaultDoc
	if d.ID != "" {
		row := s.db.QueryRow(`SELECT id, title, path, sha256, mtime, body, tenant_id FROM vault_docs WHERE id=?`, d.ID)
		existing, _ = scanVaultDoc(row)
	}
	if existing == nil {
		row := s.db.QueryRow(`SELECT id, title, path, sha256, mtime, body, tenant_id FROM vault_docs WHERE path=?`, d.Path)
		existing, _ = scanVaultDoc(row)
	}
	if existing != nil {
		d.ID = existing.ID
		d.TenantID = NormalizeTenant(existing.TenantID)
		_, err = s.db.Exec(`UPDATE vault_docs SET title=?, path=?, sha256=?, mtime=?, body=? WHERE id=?`,
			d.Title, d.Path, d.SHA256, formatTime(d.Mtime), d.Body, d.ID)
		if err != nil {
			return nil, err
		}
		_ = s.ReResolveVaultLinks()
		cp := d
		return &cp, nil
	}
	d.ID = newID()
	d.TenantID = NormalizeTenant(d.TenantID)
	_, err = s.db.Exec(`INSERT INTO vault_docs(id, title, path, sha256, mtime, body, tenant_id) VALUES(?,?,?,?,?,?,?)`,
		d.ID, d.Title, d.Path, d.SHA256, formatTime(d.Mtime), d.Body, d.TenantID)
	if err != nil {
		return nil, err
	}
	_ = s.ReResolveVaultLinks()
	cp := d
	return &cp, nil
}

func (s *SQLiteStore) GetVaultDoc(id string) (*VaultDoc, error) {
	row := s.db.QueryRow(`SELECT id, title, path, sha256, mtime, body, tenant_id FROM vault_docs WHERE id=?`, id)
	d, err := scanVaultDoc(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *SQLiteStore) ListVaultDocs() []*VaultDoc {
	rows, err := s.db.Query(`SELECT id, title, path, sha256, mtime, body, tenant_id FROM vault_docs ORDER BY lower(title), path`)
	if err != nil {
		return []*VaultDoc{}
	}
	defer rows.Close()
	var out []*VaultDoc
	for rows.Next() {
		d, err := scanVaultDoc(rows)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	if out == nil {
		out = []*VaultDoc{}
	}
	return out
}

func (s *SQLiteStore) FindVaultDocByPath(path string) (*VaultDoc, error) {
	path, err := normalizeVaultPath(path)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`SELECT id, title, path, sha256, mtime, body, tenant_id FROM vault_docs WHERE path=?`, path)
	d, err := scanVaultDoc(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *SQLiteStore) FindVaultDocByTitle(title string) (*VaultDoc, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	row := s.db.QueryRow(`SELECT id, title, path, sha256, mtime, body, tenant_id FROM vault_docs WHERE lower(title)=lower(?) ORDER BY path LIMIT 1`, title)
	d, err := scanVaultDoc(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *SQLiteStore) DeleteVaultDoc(id string) error {
	res, err := s.db.Exec(`DELETE FROM vault_docs WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM vault_links WHERE from_id=?`, id)
	_, _ = s.db.Exec(`UPDATE vault_links SET to_id='' WHERE to_id=?`, id)
	if s.vaultFTS {
		_, _ = s.db.Exec(`DELETE FROM vault_fts WHERE id=?`, id)
	}
	return nil
}

func (s *SQLiteStore) ReplaceVaultLinks(fromID string, raws []string) error {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vault_docs WHERE id=?`, fromID).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return ErrNotFound
	}
	if _, err := s.db.Exec(`DELETE FROM vault_links WHERE from_id=?`, fromID); err != nil {
		return err
	}
	seen := make(map[string]struct{})
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
		toID := s.resolveVaultInner(inner)
		if _, err := s.db.Exec(`INSERT INTO vault_links(from_id, to_id, raw) VALUES(?,?,?)`, fromID, toID, raw); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ListVaultDocLinks(id string) (outbound, inbound []VaultLink, err error) {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vault_docs WHERE id=?`, id).Scan(&cnt); err != nil {
		return nil, nil, err
	}
	if cnt == 0 {
		return nil, nil, ErrNotFound
	}
	rows, err := s.db.Query(`SELECT from_id, to_id, raw FROM vault_links WHERE from_id=? ORDER BY raw`, id)
	if err != nil {
		return nil, nil, err
	}
	outbound = []VaultLink{}
	for rows.Next() {
		var l VaultLink
		if err := rows.Scan(&l.FromID, &l.ToID, &l.Raw); err != nil {
			continue
		}
		outbound = append(outbound, l)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT from_id, to_id, raw FROM vault_links WHERE to_id=? ORDER BY raw`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	inbound = []VaultLink{}
	for rows.Next() {
		var l VaultLink
		if err := rows.Scan(&l.FromID, &l.ToID, &l.Raw); err != nil {
			continue
		}
		inbound = append(inbound, l)
	}
	return outbound, inbound, nil
}

func (s *SQLiteStore) SearchVault(q string) ([]VaultSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []VaultSearchHit{}, nil
	}
	if s.vaultFTS {
		if hits, err := s.searchVaultFTS(q); err == nil {
			return hits, nil
		}
	}
	return s.searchVaultInstr(q)
}

func (s *SQLiteStore) searchVaultFTS(q string) ([]VaultSearchHit, error) {
	phrase := strings.ReplaceAll(q, `"`, `""`)
	rows, err := s.db.Query(`SELECT id, title, path, snippet(vault_fts, 2, '', '', '…', 16)
		FROM vault_fts WHERE vault_fts MATCH ? ORDER BY rank LIMIT 50`, `"`+phrase+`"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VaultSearchHit
	for rows.Next() {
		var h VaultSearchHit
		if err := rows.Scan(&h.ID, &h.Title, &h.Path, &h.Snippet); err != nil {
			continue
		}
		out = append(out, h)
	}
	if out == nil {
		out = []VaultSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchVaultInstr(q string) ([]VaultSearchHit, error) {
	rows, err := s.db.Query(`SELECT id, title, path, body FROM vault_docs
		WHERE instr(lower(title), lower(?)) > 0 OR instr(lower(body), lower(?)) > 0
		ORDER BY lower(title) LIMIT 50`, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VaultSearchHit
	for rows.Next() {
		var id, title, path, body string
		if err := rows.Scan(&id, &title, &path, &body); err != nil {
			continue
		}
		hay := body
		if !strings.Contains(strings.ToLower(body), strings.ToLower(q)) {
			hay = title
		}
		out = append(out, VaultSearchHit{ID: id, Title: title, Path: path, Snippet: SnippetAround(hay, q, 80)})
	}
	if out == nil {
		out = []VaultSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) ReResolveVaultLinks() error {
	rows, err := s.db.Query(`SELECT from_id, raw FROM vault_links`)
	if err != nil {
		return err
	}
	type pair struct{ from, raw string }
	var all []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.from, &p.raw); err != nil {
			continue
		}
		all = append(all, p)
	}
	rows.Close()
	for _, p := range all {
		toID := s.resolveVaultInner(vaultInner(p.raw))
		_, _ = s.db.Exec(`UPDATE vault_links SET to_id=? WHERE from_id=? AND raw=?`, toID, p.from, p.raw)
	}
	return nil
}
