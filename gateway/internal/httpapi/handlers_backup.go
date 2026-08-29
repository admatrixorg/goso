// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/backup"
)

func registerBackupRoutes(mux *http.ServeMux) {
	aliasAPI(mux, "GET /api/system/backup", handleListBackup())
	aliasAPI(mux, "POST /api/system/backup", handleCreateBackup())
	aliasAPI(mux, "POST /api/system/restore", handleRestoreBackup())
}

func handleCreateBackup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := backup.Snapshot()
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleListBackup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files, err := backup.List()
		if err != nil {
			writeBackupErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files})
	}
}

func handleRestoreBackup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			File  string `json:"file"`
			Apply bool   `json:"apply"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Apply {
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
		writeJSON(w, http.StatusOK, map[string]any{
			"file":      body.File,
			"integrity": "ok",
			"applied":   false,
		})
	}
}

func writeBackupErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backup.ErrNoFile), errors.Is(err, backup.ErrPostgres):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrCorrupt):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrEscape):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, backup.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}
