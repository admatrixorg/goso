// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerToolSettingsRoutes serves the per-tenant settings of the builtin
// media tools (provider chains):
//
//	GET    /api/tools/builtin/settings          every chain tool of the tenant
//	GET    /api/tools/builtin/{name}/settings   one tool
//	PUT    /api/tools/builtin/{name}/settings   {"providers":[...]}
//	DELETE /api/tools/builtin/{name}/settings   back to the env fallback
//
// (and /v1/...). Every call is scoped to the request tenant. A provider must
// be an env provider or a saved provider of the same tenant. Settings never
// hold a key.
func registerToolSettingsRoutes(mux *http.ServeMux, opt Options) {
	st := opt.Store
	al := opt.Audit
	tss := func(w http.ResponseWriter) store.ToolSettingsStore {
		ts, _ := st.(store.ToolSettingsStore)
		if ts == nil {
			writeErr(w, http.StatusNotImplemented, "tool settings need a store")
		}
		return ts
	}
	aliasAPI(mux, "GET /api/tools/builtin/settings", func(w http.ResponseWriter, r *http.Request) {
		ts := tss(w)
		if ts == nil {
			return
		}
		tid := requestTenant(r)
		out := make([]map[string]any, 0, len(builtin.ChainTools))
		for _, tool := range builtin.ChainTools {
			out = append(out, toolSettingsView(ts, tid, tool))
		}
		writeJSON(w, http.StatusOK, map[string]any{"tools": out})
	})
	aliasAPI(mux, "GET /api/tools/builtin/{name}/settings", func(w http.ResponseWriter, r *http.Request) {
		ts := tss(w)
		if ts == nil {
			return
		}
		tool := builtin.SettingsTool(r.PathValue("name"))
		if tool == "" {
			writeErr(w, http.StatusNotFound, "tool has no settings")
			return
		}
		writeJSON(w, http.StatusOK, toolSettingsView(ts, requestTenant(r), tool))
	})
	aliasAPI(mux, "PUT /api/tools/builtin/{name}/settings", func(w http.ResponseWriter, r *http.Request) {
		ts := tss(w)
		if ts == nil {
			return
		}
		tool := builtin.SettingsTool(r.PathValue("name"))
		if tool == "" {
			writeErr(w, http.StatusNotFound, "tool has no settings")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, store.MaxToolSettingsBytes))
		if err != nil {
			writeErr(w, http.StatusRequestEntityTooLarge, "settings too large")
			return
		}
		parsed, err := builtin.ParseMediaSettings(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		tid := requestTenant(r)
		warnings := []string{}
		for i, e := range parsed.Providers {
			switch builtin.ChainProviderStatus(st, tid, e.Provider) {
			case "":
				writeErr(w, http.StatusBadRequest, "providers["+itoa(i)+"].provider "+quote(e.Provider)+" not found for this tenant")
				return
			case "disabled":
				warnings = append(warnings, "provider "+quote(e.Provider)+" is disabled; the chain skips it until it is enabled")
			}
		}
		canon, _ := json.Marshal(parsed)
		var before map[string]any
		if cur, err := ts.GetToolSettings(tid, tool); err == nil {
			before = auditToolSettings(cur.Settings)
		}
		saved, err := ts.PutToolSettings(store.ToolSettings{TenantID: tid, Tool: tool, Settings: canon})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		recordAudit(al, r, auditlog.Record{Action: "update", Entity: "builtin_tool_settings", EntityID: tool, Before: before, After: auditMeta(true, auditToolSettings(saved.Settings))})
		view := toolSettingsView(ts, tid, tool)
		view["warnings"] = warnings
		writeJSON(w, http.StatusOK, view)
	})
	aliasAPI(mux, "DELETE /api/tools/builtin/{name}/settings", func(w http.ResponseWriter, r *http.Request) {
		ts := tss(w)
		if ts == nil {
			return
		}
		tool := builtin.SettingsTool(r.PathValue("name"))
		if tool == "" {
			writeErr(w, http.StatusNotFound, "tool has no settings")
			return
		}
		tid := requestTenant(r)
		cur, err := ts.GetToolSettings(tid, tool)
		if err != nil {
			writeErr(w, http.StatusNotFound, "no settings for this tool")
			return
		}
		if err := ts.DeleteToolSettings(tid, tool); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "no settings for this tool")
				return
			}
			writeErr(w, http.StatusInternalServerError, "delete failed")
			return
		}
		recordAudit(al, r, auditlog.Record{Action: "delete", Entity: "builtin_tool_settings", EntityID: tool, Before: auditToolSettings(cur.Settings), After: auditMeta(true, nil)})
		w.WriteHeader(http.StatusNoContent)
	})
}

// toolSettingsView is one tool's settings and where its config comes from:
// "tenant" (a saved chain), "env" (GOSO_* fallback) or "none".
func toolSettingsView(ts store.ToolSettingsStore, tenant, tool string) map[string]any {
	out := map[string]any{"tool": tool, "settings": nil, "source": "none"}
	if row, err := ts.GetToolSettings(tenant, tool); err == nil {
		var v any
		if json.Unmarshal(row.Settings, &v) == nil {
			out["settings"] = v
		}
		out["source"] = "tenant"
		out["updated_at"] = row.UpdatedAt
		return out
	}
	if builtin.EnvConfigured(tool) {
		out["source"] = "env"
	}
	return out
}

// auditToolSettings keeps only provider/model names: no params.
func auditToolSettings(raw json.RawMessage) map[string]any {
	s, err := builtin.ParseMediaSettings(raw)
	if err != nil {
		return map[string]any{"providers": nil}
	}
	names := make([]string, 0, len(s.Providers))
	for _, e := range s.Providers {
		n := e.Provider
		if e.Model != "" {
			n += "/" + e.Model
		}
		names = append(names, n)
	}
	return map[string]any{"providers": names}
}

func quote(s string) string {
	b, _ := json.Marshal(strings.TrimSpace(s))
	return string(b)
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
