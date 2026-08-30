// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/impexp"
)

const maxArchiveBytes = 1 << 20

func registerImportExportRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/import-export", handleImportExportCatalog(opt))
	aliasAPI(mux, "GET /api/import-export/{id}", handleImportExportJob(opt))
	aliasAPI(mux, "POST /api/import-export/export", handleImportExportExport(opt))
	aliasAPI(mux, "POST /api/import-export/preview", handleImportExportPreview(opt))
	aliasAPI(mux, "POST /api/import-export/import", handleImportExportImport(opt))
	aliasAPI(mux, "POST /api/import-export/{id}/rollback", handleImportExportRollback(opt))
}

func handleImportExportCatalog(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cat := opt.Portable.Catalog(requestTenant(r))
		if impexp.ContainsSecrets(cat) {
			writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
			return
		}
		writeJSON(w, http.StatusOK, cat)
	}
}

func handleImportExportJob(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := opt.Portable.Get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		if impexp.ContainsSecrets(job) {
			writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func handleImportExportExport(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sel impexp.Selection
		if err := json.NewDecoder(io.LimitReader(r.Body, maxArchiveBytes)).Decode(&sel); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		job, err := opt.Portable.Export(requestTenant(r), sel)
		if err != nil {
			status := http.StatusBadRequest
			if job != nil && job.Status == impexp.StatusFailed && job.Error != "" {
				writeJSON(w, status, job)
				return
			}
			writeErr(w, status, err.Error())
			return
		}
		if impexp.ContainsSecrets(job) {
			writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
			return
		}
		meta := map[string]any{}
		if job.Archive != nil {
			meta["teams"] = len(job.Archive.Teams)
			meta["agents"] = len(job.Archive.Agents)
			meta["skills"] = len(job.Archive.Skills)
			meta["mcp"] = len(job.Archive.MCP)
		}
		recordAudit(opt.Audit, r, auditlog.Record{
			Action:   "export",
			Entity:   "portable",
			EntityID: job.ID,
			After:    auditMeta(true, meta),
		})
		writeJSON(w, http.StatusOK, job)
	}
}

func handleImportExportPreview(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := readArchiveBody(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		prev, err := opt.Portable.Preview(requestTenant(r), raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if impexp.ContainsSecrets(prev) {
			writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
			return
		}
		writeJSON(w, http.StatusOK, prev)
	}
}

func handleImportExportImport(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Archive  json.RawMessage `json:"archive"`
			Conflict string          `json:"conflict"`
			DryRun   bool            `json:"dry_run"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxArchiveBytes)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		job, err := opt.Portable.Import(requestTenant(r), body.Archive, body.Conflict, body.DryRun)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, impexp.ErrSchema) || errors.Is(err, impexp.ErrVersion) || errors.Is(err, impexp.ErrInvalidArchive) {
				status = http.StatusUnprocessableEntity
			}
			if job != nil {
				writeJSON(w, status, job)
				return
			}
			writeErr(w, status, err.Error())
			return
		}
		if impexp.ContainsSecrets(job) {
			writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
			return
		}
		recordAudit(opt.Audit, r, auditlog.Record{
			Action:   "import",
			Entity:   "portable",
			EntityID: job.ID,
			After: auditMeta(true, map[string]any{
				"dry_run":     job.DryRun,
				"conflict":    job.Conflict,
				"created":     len(job.Report.Created),
				"skipped":     len(job.Report.Skipped),
				"renamed":     len(job.Report.Renamed),
				"overwritten": len(job.Report.Overwritten),
			}),
		})
		writeJSON(w, http.StatusOK, job)
	}
}

func handleImportExportRollback(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := opt.Portable.Rollback(r.PathValue("id"))
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, impexp.ErrNotFound):
				status = http.StatusNotFound
			case errors.Is(err, impexp.ErrAlreadyRolled), errors.Is(err, impexp.ErrImportNotDone):
				status = http.StatusConflict
			}
			if job != nil {
				writeJSON(w, status, job)
				return
			}
			writeErr(w, status, err.Error())
			return
		}
		recordAudit(opt.Audit, r, auditlog.Record{
			Action:   "rollback",
			Entity:   "portable",
			EntityID: job.ID,
			After:    auditMeta(true, map[string]any{"status": job.Status}),
		})
		writeJSON(w, http.StatusOK, job)
	}
}

func readArchiveBody(r *http.Request) ([]byte, error) {
	var body struct {
		Archive json.RawMessage `json:"archive"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxArchiveBytes)).Decode(&body); err != nil {
		return nil, errors.New("invalid json")
	}
	if len(strings.TrimSpace(string(body.Archive))) == 0 {
		return nil, errors.New("archive is required")
	}
	return body.Archive, nil
}
