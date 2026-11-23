package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerHookRegistryRoutes serves the hook list, the executions ledger and the
// stats row. These paths are registered before the parameterised hook routes so
// /v1/hooks/executions is not read as a hook id.
func registerHookRegistryRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/hooks/executions", handleHookExecutions(st))
	mux.HandleFunc("GET /v1/hooks/stats", handleHookStats(st))
}

func hookView(h *store.Hook) map[string]any {
	if h == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id": h.ID, "agent_id": h.AgentID, "event": h.Event, "matcher": h.Matcher,
		"handler_type": h.HandlerType, "config": h.Config, "enabled": h.Enabled,
		"created_at": h.CreatedAt, "updated_at": h.UpdatedAt,
		"secret_set": h.SecretEnc != "",
	}
}

func handleHookCreate(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Hooks(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "hook store unavailable")
			return
		}
		var body store.Hook
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.ID = ""
		body.TenantID = requestTenant(r)
		saved, err := x.CreateHook(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, hookView(saved))
	}
}

func handleHookPatch(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Hooks(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "hook store unavailable")
			return
		}
		tenant := requestTenant(r)
		cur, err := x.GetHook(tenant, r.PathValue("id"))
		if err != nil || cur == nil {
			writeErr(w, http.StatusNotFound, "hook not found")
			return
		}
		var body map[string]any
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if v, ok := body["enabled"].(bool); ok {
			cur.Enabled = v
		}
		if v, ok := body["config"].(string); ok {
			cur.Config = v
		}
		if v, ok := body["event"].(string); ok && strings.TrimSpace(v) != "" {
			cur.Event = v
		}
		if v, ok := body["matcher"].(string); ok {
			cur.Matcher = v
		}
		saved, perr := x.UpdateHook(*cur)
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, hookView(saved))
	}
}

func handleHookDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Hooks(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "hook store unavailable")
			return
		}
		if err := x.DeleteHook(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "hook not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleHookExecutions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.HookExecutions(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"executions": []any{}, "total": 0})
			return
		}
		limit, offset := 100, 0
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
				limit = n
			}
		}
		if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		rows, total := x.ListHookExecutions(requestTenant(r),
			strings.TrimSpace(r.URL.Query().Get("hook_id")),
			strings.TrimSpace(r.URL.Query().Get("event")),
			strings.TrimSpace(r.URL.Query().Get("decision")),
			limit, offset)
		writeJSON(w, http.StatusOK, map[string]any{
			"executions": rows, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleHookStats(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.HookExecutions(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"by_decision": map[string]int{}, "total": 0})
			return
		}
		stats := x.HookExecutionStats(requestTenant(r))
		total := 0
		for _, n := range stats {
			total += n
		}
		writeJSON(w, http.StatusOK, map[string]any{"by_decision": stats, "total": total})
	}
}
