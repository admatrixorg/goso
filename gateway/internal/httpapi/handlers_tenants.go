// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

func registerTenantRoutes(mux *http.ServeMux, opt Options) {
	reg := opt.Tenants
	if reg == nil {
		reg = tenant.DefaultRegistry()
	}
	al := opt.Audit
	aliasAPI(mux, "GET /api/tenant", handleTenantContext(reg))
	aliasAPI(mux, "GET /api/tenants", handleListTenants(reg))
	aliasAPI(mux, "POST /api/tenants", handleCreateTenant(reg, al))
	aliasAPI(mux, "GET /api/tenants/{id}", handleGetTenant(reg))
	aliasAPI(mux, "POST /api/tenants/{id}/status", handleTenantStatus(reg, al))
	aliasAPI(mux, "POST /api/tenants/{id}/members", handleAddTenantMember(reg, al))
	aliasAPI(mux, "PATCH /api/tenants/{id}/members/{mid}", handlePatchTenantMember(reg, al))
	aliasAPI(mux, "DELETE /api/tenants/{id}/members/{mid}", handleRemoveTenantMember(reg, al))
}

func handleTenantContext(reg *tenant.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := requestTenant(r)
		cur := reg.Context(tid)
		master := reg.Master()
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant":       cur.ID,
			"name":         cur.Name,
			"status":       cur.Status,
			"master":       cur.Master,
			"master_id":    master.ID,
			"master_name":  master.Name,
			"multi_tenant": tenant.Enabled(),
		})
	}
}

func handleListTenants(reg *tenant.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		rows := reg.List(q)
		if rows == nil {
			rows = []tenant.Public{}
		}
		tid := requestTenant(r)
		cur := reg.Context(tid)
		master := reg.Master()
		writeJSON(w, http.StatusOK, map[string]any{
			"tenants": rows,
			"current": map[string]any{
				"id":     cur.ID,
				"name":   cur.Name,
				"status": cur.Status,
				"master": cur.Master,
			},
			"master": map[string]any{
				"id":     master.ID,
				"name":   master.Name,
				"status": master.Status,
			},
			"multi_tenant": tenant.Enabled(),
		})
	}
}

func handleGetTenant(reg *tenant.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		row, err := reg.Get(id)
		if writeTenantErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, row)
	}
}

func handleCreateTenant(reg *tenant.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		}
		if !decodeTenantBody(w, r, &body) {
			return
		}
		row, err := reg.Create(body.Slug, body.Name)
		if writeTenantErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "create", Entity: "tenant", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"status": row.Status, "name": row.Name}),
		})
		writeJSON(w, http.StatusCreated, row)
	}
}

func handleTenantStatus(reg *tenant.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		var body struct {
			Status  string `json:"status"`
			Confirm string `json:"confirm"`
		}
		if !decodeTenantBody(w, r, &body) {
			return
		}
		before, _ := reg.Get(id)
		row, err := reg.SetStatus(id, body.Status, body.Confirm)
		if writeTenantErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "status", Entity: "tenant", EntityID: row.ID,
			Before: auditMeta(true, map[string]any{"status": before.Status}),
			After:  auditMeta(true, map[string]any{"status": row.Status}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func handleAddTenantMember(reg *tenant.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		var body struct {
			Subject string `json:"subject"`
			Role    string `json:"role"`
		}
		if !decodeTenantBody(w, r, &body) {
			return
		}
		row, mem, err := reg.AddMember(id, body.Subject, body.Role)
		if writeTenantErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "access", Entity: "tenant", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"member_id": mem.ID, "role": mem.Role, "subject": mem.Subject}),
		})
		writeJSON(w, http.StatusCreated, row)
	}
}

func handlePatchTenantMember(reg *tenant.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		mid := strings.TrimSpace(r.PathValue("mid"))
		var body struct {
			Role string `json:"role"`
		}
		if !decodeTenantBody(w, r, &body) {
			return
		}
		row, mem, err := reg.SetMemberRole(id, mid, body.Role)
		if writeTenantErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "access", Entity: "tenant", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"member_id": mem.ID, "role": mem.Role, "subject": mem.Subject}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func handleRemoveTenantMember(reg *tenant.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		mid := strings.TrimSpace(r.PathValue("mid"))
		var body struct {
			Confirm string `json:"confirm"`
		}
		if !decodeTenantBody(w, r, &body) {
			return
		}
		row, mem, err := reg.RemoveMember(id, mid, body.Confirm)
		if writeTenantErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "access", Entity: "tenant", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"member_id": mem.ID, "removed": true, "subject": mem.Subject}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func decodeTenantBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	if err := dec.Decode(dst); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func writeTenantErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, tenant.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, tenant.ErrExists), errors.Is(err, tenant.ErrMemberExists):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, tenant.ErrMaster):
		writeErr(w, http.StatusConflict, "cannot deactivate master tenant")
	case errors.Is(err, tenant.ErrCap):
		writeErr(w, http.StatusConflict, "too many tenants")
	case errors.Is(err, tenant.ErrMemberCap):
		writeErr(w, http.StatusConflict, "too many members")
	case errors.Is(err, tenant.ErrConfirmRequired):
		writeErr(w, http.StatusBadRequest, "confirm is required")
	case errors.Is(err, tenant.ErrConfirm):
		writeErr(w, http.StatusBadRequest, "confirm does not match")
	case errors.Is(err, tenant.ErrSlug):
		writeErr(w, http.StatusBadRequest, "slug is required")
	case errors.Is(err, tenant.ErrName):
		writeErr(w, http.StatusBadRequest, "name is required")
	case errors.Is(err, tenant.ErrStatus):
		writeErr(w, http.StatusBadRequest, "status must be active or deactivated")
	case errors.Is(err, tenant.ErrSubject):
		writeErr(w, http.StatusBadRequest, "subject is required")
	case errors.Is(err, tenant.ErrRole):
		writeErr(w, http.StatusBadRequest, "role must be owner, admin, member, or viewer")
	case errors.Is(err, tenant.ErrSecret):
		writeErr(w, http.StatusBadRequest, "secret-shaped value is not allowed")
	default:
		writeErr(w, http.StatusBadRequest, "invalid request")
	}
	return true
}

// GuardDeactivatedTenant rejects writes when the request tenant is registered and deactivated.
func GuardDeactivatedTenant(reg *tenant.Registry, next http.Handler) http.Handler {
	if reg == nil {
		reg = tenant.DefaultRegistry()
	}
	return guardDeactivatedTenant(reg, next)
}

func guardDeactivatedTenant(reg *tenant.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if next == nil {
			return
		}
		if r == nil {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/tenants") || strings.HasPrefix(path, "/v1/tenants") {
			next.ServeHTTP(w, r)
			return
		}
		tid := requestTenant(r)
		if !reg.Writable(tid) {
			writeErr(w, http.StatusConflict, "tenant deactivated")
			return
		}
		next.ServeHTTP(w, r)
	})
}
