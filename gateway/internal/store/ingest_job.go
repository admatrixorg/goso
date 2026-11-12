// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Ingest job statuses. SPEC 170 writes uploaded, duplicate, and failed.
// SPEC 171 also writes converting and converted. Later specs reuse the same
// table for naming, classifying, inbox, filed, wiki_updating, and ready.
const (
	IngestUploaded   = "uploaded"
	IngestConverting = "converting"
	IngestConverted  = "converted"
	IngestDuplicate  = "duplicate"
	IngestFailed     = "failed"
)

// IngestJob is one vault raw upload. It is not a vault_docs row.
// SHA256 is the hash of the original bytes. RawPath is the logical path
// raw/<sha256>.<ext> inside the tenant directory.
type IngestJob struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	SHA256           string    `json:"sha256"`
	OriginalFilename string    `json:"original_filename"`
	MIME             string    `json:"mime_detected"`
	Bytes            int64     `json:"bytes"`
	RawPath          string    `json:"raw_path"`
	Status           string    `json:"status"`
	ErrorClass       string    `json:"error_class,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	ExistingJobID    string    `json:"existing_job_id,omitempty"`
	// SourcePaths are logical vault paths written by conversion. The document
	// text itself is not stored on the job.
	SourcePaths []string  `json:"source_paths,omitempty"`
	OCREngine   string    `json:"ocr_engine,omitempty"`
	OCRLang     string    `json:"ocr_lang,omitempty"`
	PageCount   int       `json:"page_count,omitempty"`
	ConvertedAt time.Time `json:"converted_at,omitempty"`
}

const ingestJobSelect = `id, tenant_id, sha256_original, original_filename, mime_detected, bytes, raw_path, status, error_class, created_at, updated_at, existing_job_id, source_paths, ocr_engine, ocr_lang, page_count, converted_at`

const ingestJobCreateTable = `CREATE TABLE IF NOT EXISTS vault_ingest_jobs (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	sha256_original TEXT NOT NULL,
	original_filename TEXT NOT NULL,
	mime_detected TEXT NOT NULL,
	bytes BIGINT NOT NULL,
	raw_path TEXT NOT NULL,
	status TEXT NOT NULL,
	error_class TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	existing_job_id TEXT NOT NULL DEFAULT '',
	source_paths TEXT NOT NULL DEFAULT '',
	ocr_engine TEXT NOT NULL DEFAULT '',
	ocr_lang TEXT NOT NULL DEFAULT '',
	page_count INTEGER NOT NULL DEFAULT 0,
	converted_at TEXT NOT NULL DEFAULT ''
)`

const ingestJobCreateIndex = `CREATE INDEX IF NOT EXISTS idx_vault_ingest_jobs_tenant_sha ON vault_ingest_jobs(tenant_id, sha256_original)`

func (s *Store) PutIngestJob(in IngestJob) (*IngestJob, error) {
	job, err := normalizeIngestJob(in)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.ID == "" {
		job.ID = s.nextID()
	}
	if existing := s.ingestJobs[job.ID]; existing != nil && !SameTenant(existing.TenantID, job.TenantID) {
		return nil, ErrConflict
	}
	cp := cloneIngestJob(job)
	s.ingestJobs[job.ID] = &cp
	out := cloneIngestJob(cp)
	return &out, nil
}

func (s *Store) GetIngestJob(id string) (*IngestJob, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	job := s.ingestJobs[id]
	if job == nil {
		return nil, ErrNotFound
	}
	cp := cloneIngestJob(*job)
	return &cp, nil
}

func (s *Store) FindUploadedIngestJob(tenant, sha string) (*IngestJob, error) {
	tenant = NormalizeTenant(tenant)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha == "" {
		return nil, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var found *IngestJob
	for _, job := range s.ingestJobs {
		if job == nil || job.Status != IngestUploaded || job.SHA256 != sha || !SameTenant(job.TenantID, tenant) {
			continue
		}
		if found == nil || job.CreatedAt.Before(found.CreatedAt) || (job.CreatedAt.Equal(found.CreatedAt) && job.ID < found.ID) {
			cp := cloneIngestJob(*job)
			found = &cp
		}
	}
	if found == nil {
		return nil, ErrNotFound
	}
	return found, nil
}

func (s *Store) FindIngestOriginal(tenant, sha string) (*IngestJob, error) {
	tenant = NormalizeTenant(tenant)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha == "" {
		return nil, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var found *IngestJob
	for _, job := range s.ingestJobs {
		if job == nil || job.SHA256 != sha || !SameTenant(job.TenantID, tenant) || !ingestOriginalStatus(job.Status) {
			continue
		}
		if found == nil || job.CreatedAt.Before(found.CreatedAt) || (job.CreatedAt.Equal(found.CreatedAt) && job.ID < found.ID) {
			cp := cloneIngestJob(*job)
			found = &cp
		}
	}
	if found == nil {
		return nil, ErrNotFound
	}
	return found, nil
}

func (s *Store) ListIngestJobsByStatus(status string) ([]*IngestJob, error) {
	status = strings.TrimSpace(status)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*IngestJob
	for _, job := range s.ingestJobs {
		if job == nil || job.Status != status {
			continue
		}
		cp := cloneIngestJob(*job)
		out = append(out, &cp)
	}
	sortIngestJobs(out)
	if out == nil {
		out = []*IngestJob{}
	}
	return out, nil
}

func (s *Store) ClaimIngestJob(id string) (*IngestJob, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.ingestJobs[id]
	if job == nil || job.Status != IngestUploaded {
		return nil, ErrConflict
	}
	job.Status = IngestConverting
	job.UpdatedAt = time.Now().UTC()
	cp := cloneIngestJob(*job)
	return &cp, nil
}

func (s *SQLiteStore) PutIngestJob(in IngestJob) (*IngestJob, error) {
	job, err := normalizeIngestJob(in)
	if err != nil {
		return nil, err
	}
	if job.ID == "" {
		job.ID = newID()
	}
	var existingTenant string
	err = s.db.QueryRow(`SELECT tenant_id FROM vault_ingest_jobs WHERE id=?`, job.ID).Scan(&existingTenant)
	if err == nil {
		if !SameTenant(existingTenant, job.TenantID) {
			return nil, ErrConflict
		}
		_, err = s.db.Exec(`UPDATE vault_ingest_jobs SET tenant_id=?, sha256_original=?, original_filename=?, mime_detected=?, bytes=?, raw_path=?, status=?, error_class=?, created_at=?, updated_at=?, existing_job_id=?, source_paths=?, ocr_engine=?, ocr_lang=?, page_count=?, converted_at=? WHERE id=?`,
			job.TenantID, job.SHA256, job.OriginalFilename, job.MIME, job.Bytes, job.RawPath, job.Status, job.ErrorClass, formatTime(job.CreatedAt), formatTime(job.UpdatedAt), job.ExistingJobID, encodeSourcePaths(job.SourcePaths), job.OCREngine, job.OCRLang, job.PageCount, formatOptionalTime(job.ConvertedAt), job.ID)
		if err != nil {
			return nil, err
		}
		return &job, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	_, err = s.db.Exec(`INSERT INTO vault_ingest_jobs(id, tenant_id, sha256_original, original_filename, mime_detected, bytes, raw_path, status, error_class, created_at, updated_at, existing_job_id, source_paths, ocr_engine, ocr_lang, page_count, converted_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.TenantID, job.SHA256, job.OriginalFilename, job.MIME, job.Bytes, job.RawPath, job.Status, job.ErrorClass, formatTime(job.CreatedAt), formatTime(job.UpdatedAt), job.ExistingJobID, encodeSourcePaths(job.SourcePaths), job.OCREngine, job.OCRLang, job.PageCount, formatOptionalTime(job.ConvertedAt))
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *SQLiteStore) GetIngestJob(id string) (*IngestJob, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT `+ingestJobSelect+` FROM vault_ingest_jobs WHERE id=?`, id)
	job, err := scanIngestJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *SQLiteStore) FindUploadedIngestJob(tenant, sha string) (*IngestJob, error) {
	tenant = NormalizeTenant(tenant)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT `+ingestJobSelect+` FROM vault_ingest_jobs WHERE tenant_id=? AND sha256_original=? AND status=? ORDER BY created_at, id LIMIT 1`,
		tenant, sha, IngestUploaded)
	job, err := scanIngestJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *SQLiteStore) FindIngestOriginal(tenant, sha string) (*IngestJob, error) {
	tenant = NormalizeTenant(tenant)
	sha = strings.ToLower(strings.TrimSpace(sha))
	if sha == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT `+ingestJobSelect+` FROM vault_ingest_jobs WHERE tenant_id=? AND sha256_original=? AND status NOT IN (?, ?) ORDER BY created_at, id LIMIT 1`,
		tenant, sha, IngestDuplicate, IngestFailed)
	job, err := scanIngestJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *SQLiteStore) ListIngestJobsByStatus(status string) ([]*IngestJob, error) {
	status = strings.TrimSpace(status)
	rows, err := s.db.Query(`SELECT `+ingestJobSelect+` FROM vault_ingest_jobs WHERE status=? ORDER BY created_at, id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*IngestJob
	for rows.Next() {
		job, err := scanIngestJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []*IngestJob{}
	}
	return out, nil
}

func (s *SQLiteStore) ClaimIngestJob(id string) (*IngestJob, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrConflict
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(`UPDATE vault_ingest_jobs SET status=?, updated_at=? WHERE id=? AND status=?`,
		IngestConverting, formatTime(now), id, IngestUploaded)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrConflict
	}
	return s.GetIngestJob(id)
}

func (s *SQLiteStore) migrateIngestConvert() error {
	stmts := []string{
		`ALTER TABLE vault_ingest_jobs ADD COLUMN source_paths TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE vault_ingest_jobs ADD COLUMN ocr_engine TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE vault_ingest_jobs ADD COLUMN ocr_lang TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE vault_ingest_jobs ADD COLUMN page_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE vault_ingest_jobs ADD COLUMN converted_at TEXT NOT NULL DEFAULT ''`,
	}
	if s.pg {
		stmts = []string{
			`ALTER TABLE vault_ingest_jobs ADD COLUMN IF NOT EXISTS source_paths TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE vault_ingest_jobs ADD COLUMN IF NOT EXISTS ocr_engine TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE vault_ingest_jobs ADD COLUMN IF NOT EXISTS ocr_lang TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE vault_ingest_jobs ADD COLUMN IF NOT EXISTS page_count INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE vault_ingest_jobs ADD COLUMN IF NOT EXISTS converted_at TEXT NOT NULL DEFAULT ''`,
		}
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			if !s.pg && strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				continue
			}
			return fmt.Errorf("migrate ingest convert: %w", err)
		}
	}
	return nil
}

func normalizeIngestJob(in IngestJob) (IngestJob, error) {
	in.TenantID = NormalizeTenant(in.TenantID)
	in.SHA256 = strings.ToLower(strings.TrimSpace(in.SHA256))
	in.OriginalFilename = strings.TrimSpace(in.OriginalFilename)
	in.MIME = strings.TrimSpace(in.MIME)
	in.RawPath = strings.TrimSpace(in.RawPath)
	in.Status = strings.TrimSpace(in.Status)
	in.ErrorClass = strings.TrimSpace(in.ErrorClass)
	in.ExistingJobID = strings.TrimSpace(in.ExistingJobID)
	in.OCREngine = strings.TrimSpace(in.OCREngine)
	in.OCRLang = strings.TrimSpace(in.OCRLang)
	if in.PageCount < 0 {
		return IngestJob{}, errors.New("ingest job is incomplete")
	}
	paths := make([]string, 0, len(in.SourcePaths))
	for _, p := range in.SourcePaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	in.SourcePaths = paths
	if in.SHA256 == "" || in.Status == "" || in.RawPath == "" {
		return IngestJob{}, errors.New("ingest job is incomplete")
	}
	if in.OriginalFilename == "" {
		in.OriginalFilename = "file"
	}
	if in.MIME == "" {
		in.MIME = "application/octet-stream"
	}
	now := time.Now().UTC()
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	} else {
		in.CreatedAt = in.CreatedAt.UTC()
	}
	if in.UpdatedAt.IsZero() {
		in.UpdatedAt = in.CreatedAt
	} else {
		in.UpdatedAt = in.UpdatedAt.UTC()
	}
	if in.Bytes < 0 {
		return IngestJob{}, errors.New("ingest job is incomplete")
	}
	return in, nil
}

func scanIngestJob(sc scanner) (*IngestJob, error) {
	var job IngestJob
	var created, updated, paths, converted string
	if err := sc.Scan(&job.ID, &job.TenantID, &job.SHA256, &job.OriginalFilename, &job.MIME, &job.Bytes, &job.RawPath, &job.Status, &job.ErrorClass, &created, &updated, &job.ExistingJobID, &paths, &job.OCREngine, &job.OCRLang, &job.PageCount, &converted); err != nil {
		return nil, err
	}
	decoded, err := decodeSourcePaths(paths)
	if err != nil {
		return nil, err
	}
	job.SourcePaths = decoded
	job.TenantID = NormalizeTenant(job.TenantID)
	job.CreatedAt = parseTime(created)
	job.UpdatedAt = parseTime(updated)
	job.ConvertedAt = parseTime(converted)
	return &job, nil
}

func ingestOriginalStatus(status string) bool {
	switch status {
	case "", IngestDuplicate, IngestFailed:
		return false
	default:
		return true
	}
}

func cloneIngestJob(in IngestJob) IngestJob {
	if in.SourcePaths != nil {
		paths := make([]string, len(in.SourcePaths))
		copy(paths, in.SourcePaths)
		in.SourcePaths = paths
	}
	return in
}

func sortIngestJobs(jobs []*IngestJob) {
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
}

func encodeSourcePaths(paths []string) string {
	if paths == nil {
		paths = []string{}
	}
	b, err := json.Marshal(paths)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeSourcePaths(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []string{}, nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}
