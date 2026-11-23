package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerAgentSkillsExtraRoutes serves the agent skill-grant tab.
func registerAgentSkillsExtraRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/agents/{id}/skills", handleAgentSkillsList(st))
	mux.HandleFunc("POST /v1/agents/{id}/skills/{skill}", handleAgentSkillGrant(st, true))
	mux.HandleFunc("DELETE /v1/agents/{id}/skills/{skill}", handleAgentSkillGrant(st, false))
}

func handleAgentSkillsList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent, err := agentVisible(st, r.PathValue("id"), requestTenant(r))
		if err != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		tenant := requestTenant(r)
		granted := map[string]bool{}
		if x := store.Skills(st); x != nil {
			for _, g := range x.ListAgentSkillGrants(tenant, agent.ID) {
				granted[g.SkillID] = true
			}
		}
		rows := skillRowsForPage(st, tenant)
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			id, _ := row["id"].(string)
			row["granted"] = granted[id]
			row["can_manage"] = true
			out = append(out, row)
		}
		writeJSON(w, http.StatusOK, map[string]any{"skills": out, "agent_id": agent.ID})
	}
}

func handleAgentSkillGrant(st store.StoreIface, granted bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent, err := agentVisible(st, r.PathValue("id"), requestTenant(r))
		if err != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "agent_id": agent.ID, "skill": r.PathValue("skill"), "granted": granted,
			"stored": false,
		})
	}
}
