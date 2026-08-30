// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/backup"
)

func registerBackupRoutes(mux *http.ServeMux, opt Options) {
	remote := opt.BackupS3
	if remote == nil {
		remote = backup.NewRemote()
	}
	al := opt.Audit
	aliasAPI(mux, "GET /api/system/backup/preflight", handleBackupPreflight())
	aliasAPI(mux, "GET /api/system/backup/s3", handleBackupS3Get(remote))
	aliasAPI(mux, "PUT /api/system/backup/s3", handleBackupS3Put(remote, al))
	aliasAPI(mux, "POST /api/system/backup/s3/test", handleBackupS3Test(remote, al))
	aliasAPI(mux, "POST /api/system/backup/s3/clear", handleBackupS3Clear(remote, al))
	aliasAPI(mux, "GET /api/system/backup/download", handleBackupDownload(al))
	aliasAPI(mux, "POST /api/system/backup/validate", handleBackupValidate())
	aliasAPI(mux, "POST /api/system/restore/plan", handleRestorePlan())
	aliasAPI(mux, "GET /api/system/backup", handleListBackup())
	aliasAPI(mux, "POST /api/system/backup", handleCreateBackup(remote, al))
	aliasAPI(mux, "POST /api/system/restore", handleRestoreBackup(al))
}

func handleBackupPreflight() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeBackupJSON(w, http.StatusOK, backup.Preflight())
	}
}

func handleListBackup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files, err := backup.List()
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
		if scope == backup.ScopeSystem || scope == backup.ScopeTenant {
			filtered := files[:0]
			for _, f := range files {
				if f.Scope == scope {
					filtered = append(filtered, f)
				}
			}
			files = filtered
		}
		writeBackupJSON(w, http.StatusOK, map[string]any{"files": files})
	}
}

func handleCreateBackup(remote *backup.Remote, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Scope       string `json:"scope"`
			Tenant      string `json:"tenant"`
			Destination string `json:"destination"`
		}
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
		}
		if strings.TrimSpace(body.Tenant) == "" {
			body.Tenant = requestTenant(r)
		}
		opts := backup.CreateOpts{Scope: body.Scope, Tenant: body.Tenant, Destination: body.Destination, Remote: remote}
		res, err := backup.Create(opts)
		if err != nil && res.File == "" {
			writeBackupErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "backup", Entity: "backup", EntityID: res.File,
			After: auditMeta(res.File != "", map[string]any{"scope": res.Scope, "destination": res.Destination, "bytes": res.Bytes}),
		})
		writeBackupJSON(w, http.StatusOK, res)
	}
}

func handleBackupDownload(al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.URL.Query().Get("file"))
		src, cleanup, err := backup.OpenSanitized(name)
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		defer cleanup()
		f, err := os.Open(src)
		if err != nil {
			if os.IsNotExist(err) {
				writeBackupErr(w, backup.ErrNotFound)
				return
			}
			writeBackupErr(w, err)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "download", Entity: "backup", EntityID: filepath.Base(src),
			After: auditMeta(true, map[string]any{"bytes": st.Size()}),
		})
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filepath.Base(src), `"`, "")+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
		_, _ = io.Copy(w, f)
	}
}

func handleBackupValidate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			File string `json:"file"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		man, err := backup.ValidateArchive(body.File)
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		writeBackupJSON(w, http.StatusOK, map[string]any{"valid": true, "manifest": man})
	}
}

func handleRestorePlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			File string `json:"file"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		writeBackupJSON(w, http.StatusOK, backup.PlanRestore(body.File))
	}
}

func handleRestoreBackup(al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			File    string `json:"file"`
			Apply   bool   `json:"apply"`
			Confirm string `json:"confirm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Apply {
			if err := backup.ConfirmApply(body.File, body.Confirm); err != nil {
				writeBackupErr(w, err)
				return
			}
			writeErr(w, http.StatusBadRequest, "live apply is CLI-only; stop gateway then goso-gateway restore --file --apply")
			return
		}
		dest, cleanup, err := backup.RestoreToTemp(body.File)
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		defer cleanup()
		if err := backup.IntegrityCheck(dest); err != nil {
			writeBackupErr(w, err)
			return
		}
		if err := backup.Sanitize(dest); err != nil {
			writeBackupErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "restore", Entity: "backup", EntityID: body.File,
			After: auditMeta(true, map[string]any{"applied": false, "integrity": "ok"}),
		})
		writeBackupJSON(w, http.StatusOK, map[string]any{
			"file":                 body.File,
			"integrity":            "ok",
			"applied":              false,
			"credentials_excluded": true,
			"live_apply_cli_only":  true,
			"recovery":             backup.Recovery{Strategy: "pre_restore_rename", PreRestoreSuffix: ".pre-restore", TempCleanup: true, LiveApplyCLIOnly: true},
		})
	}
}

func handleBackupS3Get(remote *backup.Remote) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeBackupJSON(w, http.StatusOK, remote.Public())
	}
}

func handleBackupS3Put(remote *backup.Remote, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body backup.S3Write
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		pub, err := remote.Put(body)
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "s3_configure", Entity: "backup_s3",
			After: auditMeta(true, map[string]any{"configured": pub.Configured, "access_key_set": pub.AccessKeySet}),
		})
		writeBackupJSON(w, http.StatusOK, pub)
	}
}

func handleBackupS3Test(remote *backup.Remote, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := remote.Test()
		recordAudit(al, r, auditlog.Record{
			Action: "s3_test", Entity: "backup_s3",
			After: auditMeta(err == nil, map[string]any{"ok": err == nil}),
		})
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		writeBackupJSON(w, http.StatusOK, map[string]any{"ok": true, "configured": remote.Public().Configured})
	}
}

func handleBackupS3Clear(remote *backup.Remote, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		pub, err := remote.Clear(body.Confirm)
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "s3_clear", Entity: "backup_s3",
			After: auditMeta(true, map[string]any{"configured": pub.Configured}),
		})
		writeBackupJSON(w, http.StatusOK, pub)
	}
}

func writeBackupJSON(w http.ResponseWriter, status int, v any) {
	pub, ok := backup.AsPublicJSON(v)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
		return
	}
	writeJSON(w, status, pub)
}

func writeBackupErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backup.ErrNoFile), errors.Is(err, backup.ErrPostgres), errors.Is(err, backup.ErrPreflight):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrCorrupt), errors.Is(err, backup.ErrInvalidArchive), errors.Is(err, backup.ErrConfirm), errors.Is(err, backup.ErrScope):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrEscape):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, backup.ErrNotConfigured):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrEnvOwned):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}
