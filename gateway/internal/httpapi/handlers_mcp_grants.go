package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerMCPGrantRoutes serves per agent MCP access, the shape the MCP grants
// dialog reads.
func registerMCPGrantRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/mcp/servers/{id}/grants", handleMCPGrantsForServer(st))
	mux.HandleFunc("POST /v1/mcp/servers/{id}/grants/agent", handleMCPGrantPut(st))
	mux.HandleFunc("DELETE /v1/mcp/servers/{id}/grants/agent/{agentId}", handleMCPGrantDelete(st))
	mux.HandleFunc("GET /v1/mcp/grants/agent/{agentId}", handleMCPGrantsForAgent(st))
}

func mcpServerIDByRef(st store.StoreIface, tenant, idOrName string) string {
	x := store.Extras(st)
	if x == nil {
		return strings.TrimSpace(idOrName)
	}
	want := strings.TrimSpace(idOrName)
	for _, m := range x.ListMCPServers(tenant) {
		if m != nil && (m.ID == want || m.Name == want) {
			return m.ID
		}
	}
	return want
}

func handleMCPGrantsForServer(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.MCPGrants(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
			return
		}
		tenant := requestTenant(r)
		serverID := mcpServerIDByRef(st, tenant, r.PathValue("id"))
		rows := x.ListMCPGrants(tenant, serverID)
		writeJSON(w, http.StatusOK, map[string]any{"grants": rows, "total": len(rows)})
	}
}

func handleMCPGrantsForAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.MCPGrants(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
			return
		}
		tenant := requestTenant(r)
		agent, err := agentVisible(st, r.PathValue("agentId"), tenant)
		if err != nil || agent == nil {
			writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
			return
		}
		rows := x.ListAgentMCPGrants(tenant, agent.ID)
		writeJSON(w, http.StatusOK, map[string]any{"grants": rows, "total": len(rows)})
	}
}

func handleMCPGrantPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.MCPGrants(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "MCP grant store unavailable")
			return
		}
		tenant := requestTenant(r)
		serverID := mcpServerIDByRef(st, tenant, r.PathValue("id"))
		var body struct {
			AgentID   string   `json:"agent_id"`
			AgentID2  string   `json:"agentId"`
			Enabled   *bool    `json:"enabled"`
			ToolAllow []string `json:"tool_allow"`
			ToolDeny  []string `json:"tool_deny"`
			GrantedBy string   `json:"granted_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		agentRef := body.AgentID
		if agentRef == "" {
			agentRef = body.AgentID2
		}
		agent, aerr := agentVisible(st, agentRef, tenant)
		if aerr != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		allow, _ := json.Marshal(body.ToolAllow)
		deny, _ := json.Marshal(body.ToolDeny)
		g, perr := x.PutMCPGrant(store.MCPGrant{
			TenantID: tenant, ServerID: serverID, AgentID: agent.ID, Enabled: enabled,
			ToolAllow: string(allow), ToolDeny: string(deny),
			GrantedBy: firstNonEmpty(body.GrantedBy, "api"),
		})
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"grant": g, "ok": true})
	}
}

func handleMCPGrantDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.MCPGrants(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "MCP grant store unavailable")
			return
		}
		tenant := requestTenant(r)
		serverID := mcpServerIDByRef(st, tenant, r.PathValue("id"))
		agent, aerr := agentVisible(st, r.PathValue("agentId"), tenant)
		if aerr != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		if err := x.DeleteMCPGrant(tenant, serverID, agent.ID); err != nil {
			writeErr(w, http.StatusNotFound, "grant not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
