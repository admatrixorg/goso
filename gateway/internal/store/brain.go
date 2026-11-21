package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type BrainPage struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	SourceIDs string    `json:"source_ids"`
	Rev       int       `json:"rev"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BrainHit struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

const (
	BRAIN_CREATE = `CREATE TABLE IF NOT EXISTS brain_pages (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, slug TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL DEFAULT '', source_ids TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, UNIQUE(tenant_id, slug))`
	BRAIN_INDEX  = `CREATE INDEX IF NOT EXISTS idx_brain_pages_tenant ON brain_pages(tenant_id)`
	BRAIN_GET_ID = `SELECT id FROM brain_pages WHERE tenant_id=? AND slug=?`
	BRAIN_UPSERT = `INSERT INTO brain_pages(id, tenant_id, slug, title, body, source_ids, updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(tenant_id, slug) DO UPDATE SET title=excluded.title, body=excluded.body, source_ids=excluded.source_ids, updated_at=excluded.updated_at`
	BRAIN_GET    = `SELECT id, tenant_id, slug, title, body, source_ids, updated_at FROM brain_pages WHERE tenant_id=? AND (id=? OR slug=?) LIMIT 1`
	BRAIN_LIST   = `SELECT id, tenant_id, slug, title, body, source_ids, updated_at FROM brain_pages WHERE tenant_id=? ORDER BY title`
	BRAIN_SEARCH = `SELECT id, slug, title, body FROM brain_pages WHERE tenant_id=? AND (instr(lower(title), lower(?)) > 0 OR instr(lower(body), lower(?)) > 0) ORDER BY title LIMIT ?`
)

func (s *SQLiteStore) MigrateBrain() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(BRAIN_CREATE)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(BRAIN_INDEX)
	_ = s.migrateBrainExtra()
	return nil
}

func (s *SQLiteStore) UpsertBrainPage(p BrainPage) (*BrainPage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store required")
	}
	p.TenantID = NormalizeTenant(p.TenantID)
	p.Slug = strings.ToValidUTF8(strings.TrimSpace(p.Slug), "")
	p.Title = strings.ToValidUTF8(strings.TrimSpace(p.Title), "")
	p.Body = strings.ToValidUTF8(p.Body, "")
	if !utf8.ValidString(p.Body) {
		p.Body = strings.ToValidUTF8(p.Body, "")
	}
	if p.Slug == "" || p.Title == "" {
		return nil, errors.New("slug and title are required")
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	ts := p.UpdatedAt.UTC().Format(time.RFC3339Nano)
	if p.ID == "" {
		var existing string
		err := s.db.QueryRow(BRAIN_GET_ID, p.TenantID, p.Slug).Scan(&existing)
		if err == nil && existing != "" {
			p.ID = existing
		} else {
			p.ID = newID()
		}
	}
	_, err := s.db.Exec(BRAIN_UPSERT, p.ID, p.TenantID, p.Slug, p.Title, p.Body, p.SourceIDs, ts)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLiteStore) GetBrainPage(tenant, idOrSlug string) (*BrainPage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store required")
	}
	tenant = NormalizeTenant(tenant)
	idOrSlug = strings.TrimSpace(idOrSlug)
	if idOrSlug == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(BRAIN_GET, tenant, idOrSlug, idOrSlug)
	return scanBrainPage(row)
}

func (s *SQLiteStore) ListBrainPages(tenant string) ([]BrainPage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store required")
	}
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query(BRAIN_LIST, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainPage{}
	for rows.Next() {
		p, err := scanBrainPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SearchBrainPages(tenant, q string, limit int) ([]BrainHit, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store required")
	}
	tenant = NormalizeTenant(tenant)
	q = strings.TrimSpace(q)
	if q == "" {
		return []BrainHit{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.Query(BRAIN_SEARCH, tenant, q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrainHit{}
	for rows.Next() {
		var h BrainHit
		var body string
		if err := rows.Scan(&h.ID, &h.Slug, &h.Title, &body); err != nil {
			return nil, err
		}
		h.Snippet = brainSnippet(body)
		out = append(out, h)
	}
	return out, rows.Err()
}

func scanBrainPage(sc scanner) (*BrainPage, error) {
	var p BrainPage
	var ts string
	if err := sc.Scan(&p.ID, &p.TenantID, &p.Slug, &p.Title, &p.Body, &p.SourceIDs, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.UpdatedAt = parseTime(ts)
	return &p, nil
}

func brainSnippet(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= 240 {
		return body
	}
	return strings.TrimSpace(body[:240]) + "..."
}

func (s *SQLiteStore) ListVaultTenants() []string {
	if s == nil || s.db == nil {
		return nil
	}
	rows, err := s.db.Query("SELECT DISTINCT tenant_id FROM vault_docs ORDER BY tenant_id")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return out
		}
		out = append(out, NormalizeTenant(id))
	}
	return out
}

func (s *SQLiteStore) ListVaultDocsByTenant(tenant string) []*VaultDoc {
	if s == nil || s.db == nil {
		return nil
	}
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query("SELECT "+vaultDocSelect+" FROM vault_docs WHERE tenant_id=? ORDER BY path", tenant)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*VaultDoc
	for rows.Next() {
		d, err := scanVaultDoc(rows)
		if err != nil {
			continue
		}
		if d != nil {
			d.Title = strings.ToValidUTF8(d.Title, "")
			d.Body = strings.ToValidUTF8(d.Body, "")
			out = append(out, d)
		}
	}
	return out
}

func validUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}
