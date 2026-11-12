// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Upload errors are classes. They do not include the file name, hash, or path.
var (
	ErrTypeNotAllowed = errors.New("type not allowed")
	ErrTypeLater      = errors.New("type not supported yet")
	ErrFileTooLarge   = errors.New("file too large")
	ErrQuotaExceeded  = errors.New("quota exceeded")
	ErrQuotaRequired  = errors.New("quota_required")
	ErrUploadFailed   = errors.New("upload failed")
)

// UploadResult is a stored ingest job. Duplicate is true when this tenant
// already had the same original SHA-256. ExistingID is that first job.
type UploadResult struct {
	Job        *store.IngestJob
	Duplicate  bool
	ExistingID string
}

// SaveUpload streams r to a temp file in the tenant directory, checks magic
// bytes, enforces the per-tenant disk quota, and either publishes raw/<sha>.<ext>
// or records a duplicate. The caller must already have checked auth, tenant,
// and that a quota is configured. r must already be limited to IngestMaxBytes.
func (s *Service) SaveUpload(tenant, filename string, r io.Reader) (*UploadResult, error) {
	if !QuotaConfigured() {
		return nil, ErrQuotaRequired
	}
	tenant = store.NormalizeTenant(tenant)
	if unsupportedExt(clientExt(filename)) {
		return nil, ErrTypeLater
	}
	if err := s.EnsureIngestTree(tenant); err != nil {
		return nil, err
	}
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return nil, err
	}
	if err := security.Confine(dir); err != nil {
		return nil, err
	}
	rawDir := filepath.Join(dir, "raw")
	if err := rejectSymlinks(dir, rawDir); err != nil {
		return nil, err
	}
	tmp, sum, err := s.streamTemp(rawDir, r)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			removeRegular(tmp)
		}
	}()
	info, err := os.Lstat(tmp)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("path escape")
	}
	if info.Size() == 0 {
		return nil, ErrTypeNotAllowed
	}
	ext := clientExt(filename)
	kind, err := sniffFile(tmp)
	if err != nil {
		return nil, err
	}
	storedExt, ok := extAgrees(ext, kind)
	if !ok {
		return nil, ErrTypeNotAllowed
	}
	logical := "raw/" + sum + storedExt
	name := safeOriginalName(filename)
	mime := mimeForExt(storedExt, kind)

	unlock := lockNamespace(s.Root)
	defer unlock()

	existing, err := s.Store.FindIngestOriginal(tenant, sum)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	dest, err := s.namespaced(tenant, logical)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		removeRegular(tmp)
		committed = true
		dup, err := s.Store.PutIngestJob(store.IngestJob{
			TenantID:         tenant,
			SHA256:           sum,
			OriginalFilename: name,
			MIME:             mime,
			Bytes:            info.Size(),
			RawPath:          logical,
			Status:           store.IngestDuplicate,
			ExistingJobID:    existing.ID,
		})
		if err != nil {
			return nil, err
		}
		return &UploadResult{Job: dup, Duplicate: true, ExistingID: existing.ID}, nil
	}
	if _, err := os.Lstat(dest); err == nil {
		removeRegular(tmp)
		committed = true
		job, err := s.Store.PutIngestJob(store.IngestJob{
			TenantID:         tenant,
			SHA256:           sum,
			OriginalFilename: name,
			MIME:             mime,
			Bytes:            info.Size(),
			RawPath:          logical,
			Status:           store.IngestUploaded,
		})
		if err != nil {
			return nil, err
		}
		return &UploadResult{Job: job}, nil
	}
	quota, ok := QuotaBytes()
	if !ok {
		return nil, ErrQuotaRequired
	}
	used, err := tenantDiskUsage(dir, tmp)
	if err != nil {
		return nil, err
	}
	if used > quota {
		return nil, ErrQuotaExceeded
	}
	job, err := s.Store.PutIngestJob(store.IngestJob{
		TenantID:         tenant,
		SHA256:           sum,
		OriginalFilename: name,
		MIME:             mime,
		Bytes:            info.Size(),
		RawPath:          logical,
		Status:           store.IngestUploaded,
	})
	if err != nil {
		return nil, err
	}
	if err := publishRename(tmp, dest); err != nil {
		removeRegular(tmp)
		committed = true
		now := time.Now().UTC()
		job.Status = store.IngestFailed
		job.ErrorClass = "publish"
		job.UpdatedAt = now
		if _, uerr := s.Store.PutIngestJob(*job); uerr != nil {
			return nil, uerr
		}
		return nil, ErrUploadFailed
	}
	committed = true
	return &UploadResult{Job: job}, nil
}

func (s *Service) streamTemp(dir string, r io.Reader) (string, string, error) {
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", "", err
	}
	name := ".partial-" + hex.EncodeToString(rnd[:])
	path := filepath.Join(dir, name)
	if err := rejectSymlinks(dir, path); err != nil {
		return "", "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return "", "", err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			removeRegular(path)
		}
	}()
	buf := make([]byte, uploadChunk)
	h := sha256.New()
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if UploadChunkHook != nil {
				UploadChunkHook(n)
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return "", "", werr
			}
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", "", werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if isTooLarge(err) {
				return "", "", ErrFileTooLarge
			}
			return "", "", err
		}
	}
	if err := f.Close(); err != nil {
		return "", "", err
	}
	ok = true
	return path, hex.EncodeToString(h.Sum(nil)), nil
}

func isTooLarge(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrFileTooLarge) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "request body too large") || strings.Contains(msg, "http: request body too large")
}

func safeOriginalName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 200 {
			break
		}
	}
	if b.Len() == 0 {
		return "file"
	}
	return b.String()
}

func tenantDiskUsage(dir, keep string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".partial-") && path != keep {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}
