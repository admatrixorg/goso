package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerGrantMatrixRoutes serves the CLI and team grant rows, which complete
// the grant matrix alongside the MCP, skill and agent permissions.
func registerGrantMatrixRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/secure-cli/binaries", handleCLIBinaryList(st))
	mux.HandleFunc("GET /v1/secure-cli/grants", handleCLIGrantList(st))
	mux.HandleFunc("POST /v1/secure-cli/grants", handleCLIGrantPut(st))
	mux.HandleFunc("DELETE /v1/secure-cli/grants/{id}", handleCLIGrantDelete(st))
	mux.HandleFunc("GET /v1/teams/grants", handleTeamUserGrantList(st))
	mux.HandleFunc("POST /v1/teams/grants", handleTeamUserGrantPut(st))
	mux.HandleFunc("DELETE /v1/teams/grants/{id}", handleTeamUserGrantDelete(st))
}

func handleCLIBinaryList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.CLIGrants(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"binaries": []any{}, "total": 0})
			return
		}
		rows := x.ListCLIBinaries(requestTenant(r))
		writeJSON(w, http.StatusOK, map[string]any{"binaries": rows, "total": len(rows)})
	}
}

func handleCLIGrantList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.CLIGrants(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
			return
		}
		tenant := requestTenant(r)
		agentRef := strings.TrimSpace(r.URL.Query().Get("agent_id"))
		if agentRef != "" {
			agent, err := agentVisible(st, agentRef, tenant)
			if err != nil || agent == nil {
				writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
				return
			}
			rows := x.ListAgentCLIGrants(tenant, agent.ID)
			writeJSON(w, http.StatusOK, map[string]any{"grants": rows, "total": len(rows), "agent_id": agent.ID})
			return
		}
		rows := x.ListCLIGrants(tenant, strings.TrimSpace(r.URL.Query().Get("binary_id")))
		writeJSON(w, http.StatusOK, map[string]any{"grants": rows, "total": len(rows)})
	}
}

func handleCLIGrantPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.CLIGrants(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "cli grant store unavailable")
			return
		}
		var body struct {
			BinaryID   string   `json:"binary_id"`
			BinaryRef  string   `json:"binary_name"`
			AgentID    string   `json:"agent_id"`
			Enabled    *bool    `json:"enabled"`
			DenyArgs   []string `json:"deny_args"`
			TimeoutSec int      `json:"timeout_seconds"`
			Tips       string   `json:"tips"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		tenant := requestTenant(r)
		agent, aerr := agentVisible(st, body.AgentID, tenant)
		if aerr != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		binaryID := strings.TrimSpace(body.BinaryID)
		if binaryID == "" && strings.TrimSpace(body.BinaryRef) != "" {
			for _, b := range x.ListCLIBinaries(tenant) {
				if strings.EqualFold(b.BinaryName, strings.TrimSpace(body.BinaryRef)) {
					binaryID = b.ID
					break
				}
			}
		}
		if binaryID == "" {
			writeErr(w, http.StatusBadRequest, "binary_id or binary_name is required")
			return
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		deny, _ := json.Marshal(body.DenyArgs)
		g, perr := x.PutCLIGrant(store.CLIGrant{
			TenantID: tenant, BinaryID: binaryID, AgentID: agent.ID, Enabled: enabled,
			DenyArgs: string(deny), TimeoutSec: body.TimeoutSec, Tips: body.Tips,
		})
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"grant": g, "ok": true})
	}
}

func handleCLIGrantDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.CLIGrants(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "cli grant store unavailable")
			return
		}
		if err := x.DeleteCLIGrant(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "grant not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleTeamUserGrantList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.TeamUserGrants(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
			return
		}
		rows := x.ListTeamUserGrants(requestTenant(r), strings.TrimSpace(r.URL.Query().Get("team_id")))
		writeJSON(w, http.StatusOK, map[string]any{"grants": rows, "total": len(rows)})
	}
}

func handleTeamUserGrantPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.TeamUserGrants(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "team grant store unavailable")
			return
		}
		var body store.TeamUserGrant
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		tenant := requestTenant(r)
		teamRef := strings.TrimSpace(body.TeamID)
		if teamRef == "" {
			writeErr(w, http.StatusBadRequest, "team_id is required")
			return
		}
		body.ID = ""
		body.TenantID = tenant
		body.TeamID = teamRef
		saved, perr := x.PutTeamUserGrant(body)
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"grant": saved, "ok": true})
	}
}

func handleTeamUserGrantDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.TeamUserGrants(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "team grant store unavailable")
			return
		}
		if err := x.DeleteTeamUserGrant(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "grant not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
