// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Ingest job statuses this PR writes. Later specs (171–174) reuse the same
// table for converting, converted, naming, classifying, inbox, filed,
// wiki_updating, and ready. No new column is required for those.
const (
	IngestUploaded  = "uploaded"
	IngestDuplicate = "duplicate"
	IngestFailed    = "failed"
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
}

const ingestJobSelect = `id, tenant_id, sha256_original, original_filename, mime_detected, bytes, raw_path, status, error_class, created_at, updated_at, existing_job_id`

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
	existing_job_id TEXT NOT NULL DEFAULT ''
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
	cp := job
	s.ingestJobs[job.ID] = &cp
	out := cp
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
	cp := *job
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
			cp := *job
			found = &cp
		}
	}
	if found == nil {
		return nil, ErrNotFound
	}
	return found, nil
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
		_, err = s.db.Exec(`UPDATE vault_ingest_jobs SET tenant_id=?, sha256_original=?, original_filename=?, mime_detected=?, bytes=?, raw_path=?, status=?, error_class=?, created_at=?, updated_at=?, existing_job_id=? WHERE id=?`,
			job.TenantID, job.SHA256, job.OriginalFilename, job.MIME, job.Bytes, job.RawPath, job.Status, job.ErrorClass, formatTime(job.CreatedAt), formatTime(job.UpdatedAt), job.ExistingJobID, job.ID)
		if err != nil {
			return nil, err
		}
		return &job, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	_, err = s.db.Exec(`INSERT INTO vault_ingest_jobs(id, tenant_id, sha256_original, original_filename, mime_detected, bytes, raw_path, status, error_class, created_at, updated_at, existing_job_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.TenantID, job.SHA256, job.OriginalFilename, job.MIME, job.Bytes, job.RawPath, job.Status, job.ErrorClass, formatTime(job.CreatedAt), formatTime(job.UpdatedAt), job.ExistingJobID)
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

func normalizeIngestJob(in IngestJob) (IngestJob, error) {
	in.TenantID = NormalizeTenant(in.TenantID)
	in.SHA256 = strings.ToLower(strings.TrimSpace(in.SHA256))
	in.OriginalFilename = strings.TrimSpace(in.OriginalFilename)
	in.MIME = strings.TrimSpace(in.MIME)
	in.RawPath = strings.TrimSpace(in.RawPath)
	in.Status = strings.TrimSpace(in.Status)
	in.ErrorClass = strings.TrimSpace(in.ErrorClass)
	in.ExistingJobID = strings.TrimSpace(in.ExistingJobID)
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
	var created, updated string
	if err := sc.Scan(&job.ID, &job.TenantID, &job.SHA256, &job.OriginalFilename, &job.MIME, &job.Bytes, &job.RawPath, &job.Status, &job.ErrorClass, &created, &updated, &job.ExistingJobID); err != nil {
		return nil, err
	}
	job.TenantID = NormalizeTenant(job.TenantID)
	job.CreatedAt = parseTime(created)
	job.UpdatedAt = parseTime(updated)
	return &job, nil
}
