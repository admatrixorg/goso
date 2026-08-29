// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/pkgmgr"
)

func registerPackageRoutes(mux *http.ServeMux, opt Options) {
	m := opt.Packages
	if m == nil {
		m = pkgmgr.Default()
	}
	al := opt.Audit
	aliasAPI(mux, "GET /api/packages", handlePackageSnapshot(m))
	aliasAPI(mux, "GET /api/packages/{id}", handleGetPackage(m))
	aliasAPI(mux, "POST /api/packages/allow", handlePackageAllow(m, al))
	aliasAPI(mux, "POST /api/packages/unpin", handlePackageUnpin(m, al))
	aliasAPI(mux, "POST /api/packages/install", handlePackageInstall(m, al))
	aliasAPI(mux, "POST /api/packages/{id}/uninstall", handlePackageUninstall(m, al))
	aliasAPI(mux, "POST /api/packages/{id}/recover", handlePackageRecover(m, al))
	aliasAPI(mux, "POST /api/packages/cli", handlePackageSetCLI(m, al))
	aliasAPI(mux, "POST /api/packages/uncli", handlePackageClearCLI(m, al))
}

func handlePackageSnapshot(m *pkgmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := m.Snapshot()
		writeJSON(w, http.StatusOK, snap)
	}
}

func handleGetPackage(m *pkgmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, err := m.Get(strings.TrimSpace(r.PathValue("id")))
		if writePackageErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, row)
	}
}

func handlePackageAllow(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
			Pin       string `json:"pin"`
		}
		if !decodePackageBody(w, r, &body, nil) {
			return
		}
		row, err := m.Allow(body.Ecosystem, body.Name, body.Pin)
		if writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "allow", Entity: "package_allow", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"ecosystem": row.Ecosystem, "name": row.Name, "pin": row.Pin}),
		})
		writeJSON(w, http.StatusCreated, row)
	}
}

func handlePackageUnpin(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID      string `json:"id"`
			Confirm string `json:"confirm"`
		}
		if !decodePackageBody(w, r, &body, nil) {
			return
		}
		row, err := m.Unpin(body.ID, body.Confirm)
		if writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "unpin", Entity: "package_allow", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"ecosystem": row.Ecosystem, "name": row.Name}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func handlePackageInstall(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
			Version   string `json:"version"`
			Confirm   string `json:"confirm"`
		}
		if !decodePackageBody(w, r, &body, nil) {
			return
		}
		pkg, job, err := m.Install(body.Ecosystem, body.Name, body.Version, body.Confirm)
		if job.ID == "" && writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "install", Entity: "package", EntityID: pkg.ID,
			After: auditMeta(err == nil, map[string]any{
				"ecosystem": pkg.Ecosystem, "name": pkg.Name, "version": pkg.Version,
				"status": pkg.Status, "job_id": job.ID, "job_status": job.Status,
			}),
		})
		code := http.StatusCreated
		if job.Status != pkgmgr.JobSucceeded {
			code = http.StatusOK
		}
		writeJSON(w, code, map[string]any{"package": pkg, "job": job})
	}
}

func handlePackageUninstall(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if !decodePackageBody(w, r, &body, nil) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		pkg, job, err := m.Uninstall(id, body.Confirm)
		if job.ID == "" && writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "uninstall", Entity: "package", EntityID: id,
			After: auditMeta(err == nil, map[string]any{"name": pkg.Name, "job_id": job.ID, "job_status": job.Status}),
		})
		writeJSON(w, http.StatusOK, map[string]any{"package": pkg, "job": job})
	}
}

func handlePackageRecover(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if !decodePackageBody(w, r, &body, nil) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		pkg, job, err := m.Recover(id, body.Confirm)
		if job.ID == "" && writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "recover", Entity: "package", EntityID: id,
			After: auditMeta(err == nil, map[string]any{"name": pkg.Name, "status": pkg.Status, "job_id": job.ID}),
		})
		writeJSON(w, http.StatusOK, map[string]any{"package": pkg, "job": job})
	}
}

func handlePackageSetCLI(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Kind  string `json:"kind"`
			Token string `json:"token"`
		}
		if !decodePackageBody(w, r, &body, []string{"token"}) {
			return
		}
		row, err := m.SetCLI(body.Kind, body.Token)
		if writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "set_cli", Entity: "package_cli", EntityID: row.Kind,
			After: auditMeta(true, map[string]any{"kind": row.Kind, "set": true}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func handlePackageClearCLI(m *pkgmgr.Manager, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Kind    string `json:"kind"`
			Confirm string `json:"confirm"`
		}
		if !decodePackageBody(w, r, &body, nil) {
			return
		}
		row, err := m.ClearCLI(body.Kind, body.Confirm)
		if writePackageErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "clear_cli", Entity: "package_cli", EntityID: row.Kind,
			After: auditMeta(true, map[string]any{"kind": row.Kind, "set": false}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func decodePackageBody(w http.ResponseWriter, r *http.Request, dst any, allowSecret []string) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	allow := map[string]struct{}{}
	for _, k := range allowSecret {
		allow[strings.ToLower(k)] = struct{}{}
	}
	for k := range probe {
		if pkgmgr.SecretField(k) {
			if _, ok := allow[strings.ToLower(k)]; !ok {
				writeErr(w, http.StatusBadRequest, "secret-shaped field is not allowed")
				return false
			}
		}
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func writePackageErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, pkgmgr.ErrNotFound), errors.Is(err, pkgmgr.ErrNotSet):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, pkgmgr.ErrExists), errors.Is(err, pkgmgr.ErrBusy), errors.Is(err, pkgmgr.ErrCap),
		errors.Is(err, pkgmgr.ErrUseRecover), errors.Is(err, pkgmgr.ErrNotPartial):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, pkgmgr.ErrConfirmRequired):
		writeErr(w, http.StatusBadRequest, "confirm is required")
	case errors.Is(err, pkgmgr.ErrConfirm):
		writeErr(w, http.StatusBadRequest, "confirm does not match")
	case errors.Is(err, pkgmgr.ErrSecret):
		writeErr(w, http.StatusBadRequest, "secret-shaped value is not allowed")
	case errors.Is(err, pkgmgr.ErrAllow), errors.Is(err, pkgmgr.ErrPinMismatch), errors.Is(err, pkgmgr.ErrRuntime):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, pkgmgr.ErrEcosystem), errors.Is(err, pkgmgr.ErrName), errors.Is(err, pkgmgr.ErrNameInvalid),
		errors.Is(err, pkgmgr.ErrPin), errors.Is(err, pkgmgr.ErrPinInvalid), errors.Is(err, pkgmgr.ErrKind),
		errors.Is(err, pkgmgr.ErrToken):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeErr(w, http.StatusBadRequest, "invalid request")
	}
	return true
}
