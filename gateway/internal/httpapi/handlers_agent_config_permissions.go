package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerAgentConfigPermissionRoutes serves the config write permissions an
// agent holds, which the agent settings page lists.
func registerAgentConfigPermissionRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/agent-config-permissions", handleAgentConfigPermList(st))
	mux.HandleFunc("POST /v1/agent-config-permissions", handleAgentConfigPermPut(st))
	mux.HandleFunc("DELETE /v1/agent-config-permissions/{id}", handleAgentConfigPermDelete(st))
}

func handleAgentConfigPermList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.AgentConfigPermissions(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"permissions": []any{}, "total": 0})
			return
		}
		tenant := requestTenant(r)
		agentRef := strings.TrimSpace(r.URL.Query().Get("agent_id"))
		agentID := ""
		if agentRef != "" {
			if agent, err := agentVisible(st, agentRef, tenant); err == nil && agent != nil {
				agentID = agent.ID
			} else {
				agentID = agentRef
			}
		}
		rows := x.ListAgentConfigPermissions(tenant, agentID)
		writeJSON(w, http.StatusOK, map[string]any{"permissions": rows, "total": len(rows)})
	}
}

func handleAgentConfigPermPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.AgentConfigPermissions(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "permission store unavailable")
			return
		}
		var body store.AgentConfigPermission
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		tenant := requestTenant(r)
		agent, err := agentVisible(st, body.AgentID, tenant)
		if err != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		body.ID = ""
		body.TenantID = tenant
		body.AgentID = agent.ID
		saved, perr := x.PutAgentConfigPermission(body)
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"permission": saved, "ok": true})
	}
}

func handleAgentConfigPermDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.AgentConfigPermissions(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "permission store unavailable")
			return
		}
		if err := x.DeleteAgentConfigPermission(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "permission not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
