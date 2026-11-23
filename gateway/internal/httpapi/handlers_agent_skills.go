package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/skill"
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
		infos, _ := skill.List(requestTenant(r))
		out := make([]map[string]any, 0, len(infos))
		for _, info := range infos {
			name := strings.TrimSpace(info.Name)
			if name == "" {
				continue
			}
			out = append(out, map[string]any{
				"id": name, "name": name, "slug": name, "description": "",
				"visibility": "tenant", "version": 1, "granted": false,
				"can_manage": true, "is_system": false,
			})
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
