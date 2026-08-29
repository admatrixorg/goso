// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
)

func registerAPIKeyRoutes(mux *http.ServeMux, opt Options) {
	reg := opt.APIKeys
	if reg == nil {
		reg = apikey.Default()
	}
	al := opt.Audit
	aliasAPI(mux, "GET /api/api-keys", handleListAPIKeys(reg))
	aliasAPI(mux, "POST /api/api-keys", handleCreateAPIKey(reg, al))
	aliasAPI(mux, "GET /api/api-keys/{id}", handleGetAPIKey(reg))
	aliasAPI(mux, "POST /api/api-keys/{id}/revoke", handleRevokeAPIKey(reg, al))
}

func handleListAPIKeys(reg *apikey.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		rows := reg.List(q)
		if rows == nil {
			rows = []apikey.Public{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": rows})
	}
}

func handleGetAPIKey(reg *apikey.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, err := reg.Get(strings.TrimSpace(r.PathValue("id")))
		if writeAPIKeyErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, row)
	}
}

func handleCreateAPIKey(reg *apikey.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name      string   `json:"name"`
			TenantID  string   `json:"tenant_id"`
			Scopes    []string `json:"scopes"`
			ExpiresAt string   `json:"expires_at"`
		}
		if !decodeAPIKeyBody(w, r, &body) {
			return
		}
		in := apikey.Input{Name: body.Name, TenantID: body.TenantID, Scopes: body.Scopes}
		if strings.TrimSpace(body.ExpiresAt) != "" {
			exp, err := parseAPIKeyExpiry(body.ExpiresAt)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "expires_at must be RFC3339")
				return
			}
			in.ExpiresAt = &exp
		}
		row, err := reg.Create(in)
		if writeAPIKeyErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "create", Entity: "api_key", EntityID: row.ID,
			After: auditMeta(true, map[string]any{
				"name": row.Name, "prefix": row.Prefix, "scopes": row.Scopes, "tenant_id": row.TenantID,
			}),
		})
		writeJSON(w, http.StatusCreated, row)
	}
}

func handleRevokeAPIKey(reg *apikey.Registry, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		var body struct {
			Confirm string `json:"confirm"`
		}
		if !decodeAPIKeyBody(w, r, &body) {
			return
		}
		before, _ := reg.Get(id)
		row, err := reg.Revoke(id, body.Confirm)
		if writeAPIKeyErr(w, err) {
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "revoke", Entity: "api_key", EntityID: row.ID,
			Before: auditMeta(true, map[string]any{"status": before.Status, "prefix": before.Prefix}),
			After:  auditMeta(true, map[string]any{"status": row.Status, "prefix": row.Prefix}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func decodeAPIKeyBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	if err := dec.Decode(dst); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func parseAPIKeyExpiry(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func writeAPIKeyErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, apikey.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, apikey.ErrCap), errors.Is(err, apikey.ErrRevoked):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, apikey.ErrConfirmRequired):
		writeErr(w, http.StatusBadRequest, "confirm is required")
	case errors.Is(err, apikey.ErrConfirm):
		writeErr(w, http.StatusBadRequest, "confirm does not match")
	case errors.Is(err, apikey.ErrName):
		writeErr(w, http.StatusBadRequest, "name is required")
	case errors.Is(err, apikey.ErrScope):
		writeErr(w, http.StatusBadRequest, "scope is required")
	case errors.Is(err, apikey.ErrUnknownScope):
		writeErr(w, http.StatusBadRequest, "unknown scope")
	case errors.Is(err, apikey.ErrExpiry):
		writeErr(w, http.StatusBadRequest, "expiry is in the past")
	case errors.Is(err, apikey.ErrSecret):
		writeErr(w, http.StatusBadRequest, "secret-shaped value is not allowed")
	default:
		writeErr(w, http.StatusBadRequest, "invalid request")
	}
	return true
}
