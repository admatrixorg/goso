// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/storage"
)

func registerStorageRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/storage", handleStorageList())
	aliasAPI(mux, "GET /api/storage/preview", handleStoragePreview())
	aliasAPI(mux, "GET /api/storage/download", handleStorageDownload())
	aliasAPI(mux, "POST /api/storage/upload", handleStorageUpload(opt.Events, opt.Audit))
	aliasAPI(mux, "POST /api/storage/delete", handleStorageDelete(opt.Events, opt.Audit))
}

func storagePath(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("path"))
}

func handleStorageList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		showHidden := r.URL.Query().Get("show_hidden") == "1"
		lst, err := storage.List(storagePath(r), showHidden)
		if errors.Is(err, storage.ErrNotConfigured) {
			writeJSON(w, http.StatusOK, storage.EmptyListing())
			return
		}
		if writeStorageErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, lst)
	}
}

func handleStoragePreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := storage.PreviewFile(storagePath(r))
		if writeStorageErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func handleStorageDownload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, st, rel, typ, err := storage.OpenFile(storagePath(r))
		if writeStorageErr(w, err) {
			return
		}
		defer f.Close()
		if err := storage.GuardContent(f); writeStorageErr(w, err) {
			return
		}
		name := filepath.Base(rel)
		if typ == "" {
			typ = "application/octet-stream"
		}
		w.Header().Set("Content-Type", typ)
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
		_, _ = io.Copy(w, io.LimitReader(f, storage.MaxFileBytes))
	}
}

func handleStorageUpload(ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(storage.MaxFileBytes); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid multipart")
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()
		dest := strings.TrimSpace(r.FormValue("path"))
		size := hdr.Size
		ent, err := storage.Upload(dest, hdr.Filename, file, size)
		if writeStorageErr(w, err) {
			auditStorage(ev, "upload", hdr.Filename, false)
			return
		}
		auditStorage(ev, "upload", ent.Path, true)
		recordAudit(al, r, auditlog.Record{
			Action: "upload", Entity: "storage", EntityID: ent.Path,
			After: auditMeta(true, map[string]any{"path": ent.Path, "size": ent.Size}),
		})
		writeJSON(w, http.StatusCreated, ent)
	}
}

func handleStorageDelete(ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path    string `json:"path"`
			Confirm string `json:"confirm"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		ent, err := storage.Delete(strings.TrimSpace(body.Path), strings.TrimSpace(body.Confirm))
		if writeStorageErr(w, err) {
			auditStorage(ev, "delete", body.Path, false)
			return
		}
		auditStorage(ev, "delete", ent.Path, true)
		recordAudit(al, r, auditlog.Record{
			Action: "delete", Entity: "storage", EntityID: ent.Path,
			After: auditMeta(true, map[string]any{"path": ent.Path}),
		})
		writeJSON(w, http.StatusOK, ent)
	}
}

func writeStorageErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, storage.ErrNotConfigured):
		writeErr(w, http.StatusBadRequest, "not_configured")
	case errors.Is(err, storage.ErrPathEscape):
		writeErr(w, http.StatusBadRequest, "path escape")
	case errors.Is(err, storage.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, storage.ErrNotFile):
		writeErr(w, http.StatusBadRequest, "not a file")
	case errors.Is(err, storage.ErrNotDir):
		writeErr(w, http.StatusBadRequest, "not a directory")
	case errors.Is(err, storage.ErrTooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, "too large")
	case errors.Is(err, storage.ErrType):
		writeErr(w, http.StatusBadRequest, "type not allowed")
	case errors.Is(err, storage.ErrHidden), errors.Is(err, storage.ErrSecret):
		writeErr(w, http.StatusForbidden, "path not listed")
	case errors.Is(err, storage.ErrConfirmRequired):
		writeErr(w, http.StatusBadRequest, "confirm is required")
	case errors.Is(err, storage.ErrConfirm):
		writeErr(w, http.StatusBadRequest, "confirm does not match")
	case errors.Is(err, storage.ErrQuota):
		writeErr(w, http.StatusConflict, "quota exceeded")
	case errors.Is(err, storage.ErrName):
		writeErr(w, http.StatusBadRequest, "name is invalid")
	case errors.Is(err, storage.ErrNotEmpty):
		writeErr(w, http.StatusConflict, "directory not empty")
	case errors.Is(err, storage.ErrRoot):
		writeErr(w, http.StatusBadRequest, "cannot delete workspace root")
	default:
		writeErr(w, http.StatusBadRequest, "storage error")
	}
	return true
}

func auditStorage(ev *eventstore.Store, tool, path string, ok bool) {
	if ev == nil {
		return
	}
	kind := eventstore.KindSuccess
	if !ok {
		kind = eventstore.KindError
	}
	summary := tool + " storage"
	p := strings.TrimSpace(path)
	if p != "" && !storage.SecretName(filepath.Base(p)) {
		summary += " " + filepath.Base(p)
	}
	ev.Append(eventstore.Event{
		Connector: "storage",
		Tool:      tool,
		Kind:      kind,
		Summary:   summary,
	})
}
