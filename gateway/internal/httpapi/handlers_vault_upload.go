// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func handleVaultUpload(st store.StoreIface, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !vault.UploadEnabled() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		// Auth and tenant are decided before the body is read, including when
		// dev-mode skipped the outer Require middleware.
		if status, msg := authorizeVaultUpload(r); status != 0 {
			writeErr(w, status, msg)
			return
		}
		tenantID := requestTenant(r)
		if !vault.QuotaConfigured() {
			writeErr(w, http.StatusServiceUnavailable, "quota_required")
			return
		}
		maxBytes := vault.IngestMaxBytes()
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+vault.MultipartOverhead)
		reader, err := r.MultipartReader()
		if err != nil {
			if uploadTooLarge(err) {
				writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
				return
			}
			writeErr(w, http.StatusBadRequest, "file is required")
			return
		}
		part, err := firstFilePart(reader)
		if err != nil {
			if uploadTooLarge(err) || errors.Is(err, vault.ErrFileTooLarge) {
				writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
				return
			}
			writeErr(w, http.StatusBadRequest, "file is required")
			return
		}
		defer part.Close()
		filename := part.FileName()
		limited := http.MaxBytesReader(w, part, maxBytes)
		res, err := vaultSvc(st).SaveUpload(tenantID, filename, limited)
		if err != nil {
			writeUploadErr(w, err)
			return
		}
		if res == nil || res.Job == nil {
			writeErr(w, http.StatusInternalServerError, "upload failed")
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action:   "upload",
			Entity:   "vault_ingest",
			EntityID: res.Job.ID,
			After: auditMeta(true, map[string]any{
				"bytes":  res.Job.Bytes,
				"ext":    strings.ToLower(filepath.Ext(res.Job.RawPath)),
				"status": res.Job.Status,
			}),
		})
		view := viewIngestJob(res.Job)
		if res.Duplicate {
			writeJSON(w, http.StatusOK, map[string]any{
				"duplicate":       true,
				"job":             view,
				"existing_job_id": res.ExistingID,
			})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"job": view})
	}
}

func handleGetVaultIngest(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !vault.UploadEnabled() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if status, msg := authorizeVaultUpload(r); status != 0 {
			writeErr(w, status, msg)
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		job, err := st.GetIngestJob(id)
		if err != nil || job == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if hideWrongTenant(w, job.TenantID, requestTenant(r)) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": viewIngestJob(job)})
	}
}

type ingestJobView struct {
	ID               string   `json:"id"`
	TenantID         string   `json:"tenant_id"`
	Status           string   `json:"status"`
	SHA256           string   `json:"sha256"`
	Bytes            int64    `json:"bytes"`
	OriginalFilename string   `json:"original_filename"`
	RawPath          string   `json:"raw_path"`
	SourcePaths      []string `json:"source_paths"`
	OCREngine        string   `json:"ocr_engine"`
	OCRLang          string   `json:"ocr_lang"`
	PageCount        int      `json:"page_count"`
	ConvertedAt      string   `json:"converted_at"`
	ErrorClass       string   `json:"error_class,omitempty"`
}

func viewIngestJob(job *store.IngestJob) ingestJobView {
	if job == nil {
		return ingestJobView{SourcePaths: []string{}}
	}
	paths := job.SourcePaths
	if paths == nil {
		paths = []string{}
	}
	converted := ""
	if !job.ConvertedAt.IsZero() {
		converted = job.ConvertedAt.UTC().Format(time.RFC3339Nano)
	}
	return ingestJobView{
		ID:               job.ID,
		TenantID:         job.TenantID,
		Status:           job.Status,
		SHA256:           job.SHA256,
		Bytes:            job.Bytes,
		OriginalFilename: job.OriginalFilename,
		RawPath:          job.RawPath,
		SourcePaths:      paths,
		OCREngine:        job.OCREngine,
		OCRLang:          job.OCRLang,
		PageCount:        job.PageCount,
		ConvertedAt:      converted,
		ErrorClass:       job.ErrorClass,
	}
}

func authorizeVaultUpload(r *http.Request) (int, string) {
	tok := bearerToken(r)
	admin := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN"))
	if admin != "" && security.Equal(tok, admin) {
		return 0, ""
	}
	view := strings.TrimSpace(os.Getenv("GOSO_VIEW_TOKEN"))
	if view != "" && tok != "" && security.Equal(tok, view) {
		return http.StatusForbidden, "forbidden"
	}
	if tok != "" {
		if g, ok := apikey.Default().Accept(tok); ok {
			if g.Has("write") {
				return 0, ""
			}
			return http.StatusForbidden, "forbidden"
		}
	}
	return http.StatusUnauthorized, "unauthorized"
}

func bearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func firstFilePart(reader *multipart.Reader) (*multipart.Part, error) {
	if reader == nil {
		return nil, errors.New("file is required")
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil, errors.New("file is required")
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" {
			return part, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
		_ = part.Close()
	}
}

func writeUploadErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrTypeNotAllowed):
		writeErr(w, http.StatusBadRequest, "type not allowed")
	case errors.Is(err, vault.ErrTypeLater):
		writeErr(w, http.StatusBadRequest, "type not supported yet")
	case errors.Is(err, vault.ErrFileTooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
	case errors.Is(err, vault.ErrQuotaExceeded):
		writeErr(w, http.StatusConflict, "quota exceeded")
	case errors.Is(err, vault.ErrQuotaRequired):
		writeErr(w, http.StatusServiceUnavailable, "quota_required")
	default:
		if uploadTooLarge(err) {
			writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
			return
		}
		writeErr(w, http.StatusInternalServerError, "upload failed")
	}
}

func uploadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "request body too large")
}
