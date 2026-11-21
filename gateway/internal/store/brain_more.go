package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type BrainSource struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	VaultDocID string `json:"vault_doc_id"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Strategy   string `json:"strategy"`
	UpdatedAt  string `json:"updated_at"`
}

type BrainPlan struct {
	ID        string `json:"id"`
	SourceID  string `json:"source_id"`
	TenantID  string `json:"tenant_id"`
	Status    string `json:"status"`
	JSON      string `json:"json"`
	UpdatedAt string `json:"updated_at"`
}

type BrainVersion struct {
	ID        string `json:"id"`
	PageID    string `json:"page_id"`
	TenantID  string `json:"tenant_id"`
	Rev       int    `json:"rev"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type BrainLink struct {
	FromID   string `json:"from_id"`
	ToSlug   string `json:"to_slug"`
	TenantID string `json:"tenant_id"`
	Raw      string `json:"raw"`
}

type BrainDraft struct {
	ID        string `json:"id"`
	PageID    string `json:"page_id"`
	TenantID  string `json:"tenant_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (s *SQLiteStore) migrateBrainExtra() error {
	if s == nil || s.db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS brain_sources (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, vault_doc_id TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '', path TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'ready', strategy TEXT NOT NULL DEFAULT 'extractive', updated_at TEXT NOT NULL, UNIQUE(tenant_id, vault_doc_id))`,
		`CREATE INDEX IF NOT EXISTS idx_brain_sources_tenant ON brain_sources(tenant_id)`,
		`CREATE TABLE IF NOT EXISTS brain_extracts (id TEXT PRIMARY KEY, source_id TEXT NOT NULL, tenant_id TEXT NOT NULL, chunk_index INTEGER NOT NULL DEFAULT 0, payload TEXT NOT NULL DEFAULT '{}', updated_at TEXT NOT NULL, UNIQUE(source_id, chunk_index))`,
		`CREATE TABLE IF NOT EXISTS brain_plans (id TEXT PRIMARY KEY, source_id TEXT NOT NULL, tenant_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'approved', payload TEXT NOT NULL DEFAULT '{}', updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_brain_plans_tenant ON brain_plans(tenant_id)`,
		`CREATE TABLE IF NOT EXISTS brain_versions (id TEXT PRIMARY KEY, page_id TEXT NOT NULL, tenant_id TEXT NOT NULL, rev INTEGER NOT NULL, title TEXT NOT NULL DEFAULT '', body TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, UNIQUE(page_id, rev))`,
		`CREATE TABLE IF NOT EXISTS brain_links (from_id TEXT NOT NULL, to_slug TEXT NOT NULL, tenant_id TEXT NOT NULL, raw TEXT NOT NULL DEFAULT '', PRIMARY KEY(from_id, to_slug))`,
		`CREATE INDEX IF NOT EXISTS idx_brain_links_tenant ON brain_links(tenant_id)`,
		`CREATE TABLE IF NOT EXISTS brain_drafts (id TEXT PRIMARY KEY, page_id TEXT NOT NULL DEFAULT '', tenant_id TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', body TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_brain_drafts_tenant ON brain_drafts(tenant_id)`,
		`ALTER TABLE brain_pages ADD COLUMN IF NOT EXISTS rev INTEGER NOT NULL DEFAULT 1`,
	}
	for _, stmt := range stmts {
		_, _ = s.db.Exec(stmt)
	}
	return nil
}

func brainNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *SQLiteStore) UpsertBrainSource(src BrainSource) (*BrainSource, error) {
	src.TenantID = NormalizeTenant(src.TenantID)
	src.Title = validUTF8(src.Title)
	src.Path = validUTF8(src.Path)
	if src.Status == "" {
		src.Status = "ready"
	}
	if src.Strategy == "" {
		src.Strategy = "extractive"
	}
	ts := brainNow()
	if src.ID == "" {
		var existing string
		err := s.db.QueryRow(`SELECT id FROM brain_sources WHERE tenant_id=? AND vault_doc_id=?`, src.TenantID, src.VaultDocID).Scan(&existing)
		if err == nil && existing != "" {
			src.ID = existing
		} else {
			src.ID = newID()
		}
	}
	_, err := s.db.Exec(`INSERT INTO brain_sources(id, tenant_id, vault_doc_id, title, path, status, strategy, updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(tenant_id, vault_doc_id) DO UPDATE SET title=excluded.title, path=excluded.path, status=excluded.status, strategy=excluded.strategy, updated_at=excluded.updated_at`, src.ID, src.TenantID, src.VaultDocID, src.Title, src.Path, src.Status, src.Strategy, ts)
	if err != nil {
		return nil, err
	}
	src.UpdatedAt = ts
	return &src, nil
}

func (s *SQLiteStore) ListBrainSources(tenant string) ([]BrainSource, error) {
	rows, err := s.db.Query(`SELECT id, tenant_id, vault_doc_id, title, path, status, strategy, updated_at FROM brain_sources WHERE tenant_id=? ORDER BY title`, NormalizeTenant(tenant))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainSource{}
	for rows.Next() {
		var src BrainSource
		if err := rows.Scan(&src.ID, &src.TenantID, &src.VaultDocID, &src.Title, &src.Path, &src.Status, &src.Strategy, &src.UpdatedAt); err == nil {
			out = append(out, src)
		}
	}
	return out, nil
}

func (s *SQLiteStore) PutBrainPlan(p BrainPlan) (*BrainPlan, error) {
	p.TenantID = NormalizeTenant(p.TenantID)
	if p.ID == "" {
		p.ID = newID()
	}
	if p.Status == "" {
		p.Status = "approved"
	}
	p.UpdatedAt = brainNow()
	_, err := s.db.Exec(`INSERT INTO brain_plans(id, source_id, tenant_id, status, payload, updated_at) VALUES(?,?,?,?,?,?)`, p.ID, p.SourceID, p.TenantID, p.Status, p.JSON, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLiteStore) ListBrainPlans(tenant string) ([]BrainPlan, error) {
	rows, err := s.db.Query(`SELECT id, source_id, tenant_id, status, payload, updated_at FROM brain_plans WHERE tenant_id=? ORDER BY updated_at DESC LIMIT 100`, NormalizeTenant(tenant))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainPlan{}
	for rows.Next() {
		var p BrainPlan
		if err := rows.Scan(&p.ID, &p.SourceID, &p.TenantID, &p.Status, &p.JSON, &p.UpdatedAt); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *SQLiteStore) SnapshotBrainVersion(page BrainPage) {
	_, _ = s.db.Exec(`INSERT INTO brain_versions(id, page_id, tenant_id, rev, title, body, created_at) VALUES(?,?,?,?,?,?,?)`, newID(), page.ID, page.TenantID, page.Rev, page.Title, page.Body, brainNow())
}

func (s *SQLiteStore) ListBrainVersions(tenant, pageID string) ([]BrainVersion, error) {
	rows, err := s.db.Query(`SELECT id, page_id, tenant_id, rev, title, body, created_at FROM brain_versions WHERE tenant_id=? AND page_id=? ORDER BY rev DESC LIMIT 50`, NormalizeTenant(tenant), pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainVersion{}
	for rows.Next() {
		var v BrainVersion
		if err := rows.Scan(&v.ID, &v.PageID, &v.TenantID, &v.Rev, &v.Title, &v.Body, &v.CreatedAt); err == nil {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *SQLiteStore) RollbackBrainPage(tenant, pageID string, rev int) (*BrainPage, error) {
	var title, body string
	err := s.db.QueryRow(`SELECT title, body FROM brain_versions WHERE tenant_id=? AND page_id=? AND rev=?`, NormalizeTenant(tenant), pageID, rev).Scan(&title, &body)
	if err != nil {
		return nil, ErrNotFound
	}
	cur, err := s.GetBrainPage(tenant, pageID)
	if err != nil {
		return nil, err
	}
	cur.Title, cur.Body = title, body
	return s.UpsertBrainPage(*cur)
}

func (s *SQLiteStore) ReplaceBrainLinks(tenant, fromID string, slugs []string) error {
	tenant = NormalizeTenant(tenant)
	_, _ = s.db.Exec(`DELETE FROM brain_links WHERE tenant_id=? AND from_id=?`, tenant, fromID)
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}
		_, _ = s.db.Exec(`INSERT INTO brain_links(from_id, to_slug, tenant_id, raw) VALUES(?,?,?,?) ON CONFLICT(from_id, to_slug) DO NOTHING`, fromID, slug, tenant, slug)
	}
	return nil
}

func (s *SQLiteStore) ListBrainGraph(tenant string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT from_id, to_slug FROM brain_links WHERE tenant_id=? LIMIT ?`, NormalizeTenant(tenant), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges := []map[string]any{}
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err == nil {
			edges = append(edges, map[string]any{"from": from, "to": to})
		}
	}
	return edges, nil
}

func (s *SQLiteStore) ListBrainLinks(tenant, pageID string) (outbound []BrainLink, inbound []BrainLink, err error) {
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query(`SELECT from_id, to_slug, tenant_id, raw FROM brain_links WHERE tenant_id=? AND from_id=?`, tenant, pageID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var l BrainLink
		if err := rows.Scan(&l.FromID, &l.ToSlug, &l.TenantID, &l.Raw); err == nil {
			outbound = append(outbound, l)
		}
	}
	rows.Close()
	page, _ := s.GetBrainPage(tenant, pageID)
	slug := ""
	if page != nil {
		slug = page.Slug
	}
	rows, err = s.db.Query(`SELECT from_id, to_slug, tenant_id, raw FROM brain_links WHERE tenant_id=? AND to_slug=?`, tenant, slug)
	if err != nil {
		return outbound, []BrainLink{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var l BrainLink
		if err := rows.Scan(&l.FromID, &l.ToSlug, &l.TenantID, &l.Raw); err == nil {
			inbound = append(inbound, l)
		}
	}
	if outbound == nil {
		outbound = []BrainLink{}
	}
	if inbound == nil {
		inbound = []BrainLink{}
	}
	return outbound, inbound, nil
}

func (s *SQLiteStore) PutBrainDraft(d BrainDraft) (*BrainDraft, error) {
	d.TenantID = NormalizeTenant(d.TenantID)
	d.Title, d.Body = validUTF8(d.Title), validUTF8(d.Body)
	if d.ID == "" {
		d.ID = newID()
	}
	if d.Status == "" {
		d.Status = "pending"
	}
	d.CreatedAt = brainNow()
	_, err := s.db.Exec(`INSERT INTO brain_drafts(id, page_id, tenant_id, title, body, status, created_at) VALUES(?,?,?,?,?,?,?)`, d.ID, d.PageID, d.TenantID, d.Title, d.Body, d.Status, d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *SQLiteStore) ListBrainDrafts(tenant, status string) ([]BrainDraft, error) {
	tenant = NormalizeTenant(tenant)
	q := `SELECT id, page_id, tenant_id, title, body, status, created_at FROM brain_drafts WHERE tenant_id=?`
	args := []any{tenant}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT 100`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainDraft{}
	for rows.Next() {
		var d BrainDraft
		if err := rows.Scan(&d.ID, &d.PageID, &d.TenantID, &d.Title, &d.Body, &d.Status, &d.CreatedAt); err == nil {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *SQLiteStore) SetBrainDraftStatus(tenant, id, status string) (*BrainDraft, error) {
	_, err := s.db.Exec(`UPDATE brain_drafts SET status=? WHERE tenant_id=? AND id=?`, status, NormalizeTenant(tenant), id)
	if err != nil {
		return nil, err
	}
	var d BrainDraft
	err = s.db.QueryRow(`SELECT id, page_id, tenant_id, title, body, status, created_at FROM brain_drafts WHERE tenant_id=? AND id=?`, NormalizeTenant(tenant), id).Scan(&d.ID, &d.PageID, &d.TenantID, &d.Title, &d.Body, &d.Status, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (s *SQLiteStore) PutBrainExtract(sourceID, tenant string, idx int, payload any) error {
	b, _ := json.Marshal(payload)
	_, err := s.db.Exec(`INSERT INTO brain_extracts(id, source_id, tenant_id, chunk_index, payload, updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id, chunk_index) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`, newID(), sourceID, NormalizeTenant(tenant), idx, string(b), brainNow())
	return err
}
