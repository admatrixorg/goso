// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/team"
)

func parseOrchMode(s string) (string, error) {
	return team.ParseMode(s)
}

func registerTeamRoutes(mux *http.ServeMux, opt Options) {
	st := opt.Store
	ev := opt.Events
	mux.HandleFunc("POST /api/teams", handleCreateTeam(st, ev))
	aliasAPI(mux, "GET /api/teams", handleListTeams(st))
	mux.HandleFunc("GET /api/teams/{id}/members", handleListMembers(st))
	mux.HandleFunc("POST /api/teams/{id}/members", handleAddMember(st, ev))
	mux.HandleFunc("DELETE /api/teams/{id}/members/{agent_id}", handleRemoveMember(st, ev))
	mux.HandleFunc("GET /api/teams/{id}/tasks", handleListTasks(st))
	mux.HandleFunc("POST /api/teams/{id}/tasks", handleCreateTask(st, ev))
	mux.HandleFunc("PATCH /api/teams/{id}/tasks/{tid}", handleUpdateTask(st, ev))
	mux.HandleFunc("GET /api/teams/{id}/messages", handleListMessagesTeam(st))
	mux.HandleFunc("POST /api/teams/{id}/messages", handleCreateMessageTeam(st, ev))
	mux.HandleFunc("GET /api/teams/{id}", handleGetTeam(st))
	mux.HandleFunc("PUT /api/teams/{id}", handleUpdateTeam(st, ev))
	mux.HandleFunc("DELETE /api/teams/{id}", handleDeleteTeam(st, ev))

	aliasAPI(mux, "GET /api/agents/{id}/links", handleListLinks(st))
	aliasAPI(mux, "POST /api/agents/{id}/links", handleAddLink(st, ev))
	aliasAPI(mux, "DELETE /api/agents/{id}/links/{to_id}", handleRemoveLink(st, ev))
	mux.HandleFunc("GET /api/agents/{id}/evolution", handleEvolution(st))
	mux.HandleFunc("PATCH /api/agents/{id}/evolution", handleEvolutionGuardrails(st))
	mux.HandleFunc("POST /api/agents/{id}/evolution/tick", handleEvolutionTick(st))
	mux.HandleFunc("POST /api/agents/{id}/evolution/{sid}/apply", handleEvolutionApply(st))
}

func handleCreateTeam(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name        string `json:"name"`
			LeadAgentID string `json:"lead_agent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		tid := requestTenant(r)
		lead := strings.TrimSpace(body.LeadAgentID)
		if lead != "" {
			if _, err := agentVisible(st, lead, tid); err != nil {
				writeErr(w, http.StatusBadRequest, "lead agent not found")
				return
			}
		}
		t, err := st.CreateTeam(store.Team{TenantID: tid, Name: body.Name, LeadAgentID: lead})
		if err != nil {
			if errors.Is(err, store.ErrLiteCap) {
				writeErr(w, http.StatusBadRequest, "lite cap: max 1 team")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTeam, Kind: eventstore.KindSuccess, Action: "create",
			Actor: operatorActor(r), TeamID: t.ID, AgentID: t.LeadAgentID, Entity: t.ID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "create", "team_id": t.ID, "name": t.Name}),
		})
		writeJSON(w, http.StatusCreated, t)
	}
}

func handleListTeams(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := teamsInTenant(st.ListTeams(), requestTenant(r))
		if list == nil {
			list = []*store.Team{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"teams": list})
	}
}

func handleGetTeam(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := st.GetTeam(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		if hideWrongTenant(w, t.TenantID, requestTenant(r)) {
			return
		}
		members, _ := st.ListTeamMembers(t.ID)
		if members == nil {
			members = []*store.TeamMember{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"team": t, "members": members})
	}
}

func handleUpdateTeam(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := teamVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		var body struct {
			Name        string `json:"name"`
			LeadAgentID string `json:"lead_agent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		t, err := st.UpdateTeam(store.Team{ID: id, Name: body.Name, LeadAgentID: body.LeadAgentID})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "team not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTeam, Kind: eventstore.KindSuccess, Action: "update",
			Actor: operatorActor(r), TeamID: t.ID, AgentID: t.LeadAgentID, Entity: t.ID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "update", "team_id": t.ID, "name": t.Name}),
		})
		writeJSON(w, http.StatusOK, t)
	}
}

func handleDeleteTeam(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := teamVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		if err := st.DeleteTeam(id); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTeam, Kind: eventstore.KindSuccess, Action: "delete",
			Actor: operatorActor(r), TeamID: id, Entity: id,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "delete", "team_id": id}),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleListMembers(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := teamVisible(st, strings.TrimSpace(r.PathValue("id")), requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		list, err := st.ListTeamMembers(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"members": list})
	}
}

func handleAddMember(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID := strings.TrimSpace(r.PathValue("id"))
		if _, err := teamVisible(st, teamID, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		var body struct {
			AgentID string `json:"agent_id"`
			Role    string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if _, err := agentVisible(st, strings.TrimSpace(body.AgentID), requestTenant(r)); err != nil {
			writeErr(w, http.StatusBadRequest, "agent not found")
			return
		}
		m, err := st.AddTeamMember(store.TeamMember{TeamID: teamID, AgentID: body.AgentID, Role: body.Role})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTeam, Kind: eventstore.KindSuccess, Action: "add_member",
			Actor: operatorActor(r), TeamID: teamID, AgentID: m.AgentID, Entity: m.AgentID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "add_member", "team_id": teamID, "agent_id": m.AgentID, "role": m.Role}),
		})
		writeJSON(w, http.StatusCreated, m)
	}
}

func handleRemoveMember(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := teamVisible(st, strings.TrimSpace(r.PathValue("id")), requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "member not found")
			return
		}
		teamID := strings.TrimSpace(r.PathValue("id"))
		agentID := strings.TrimSpace(r.PathValue("agent_id"))
		err := st.RemoveTeamMember(teamID, agentID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "member not found")
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTeam, Kind: eventstore.KindSuccess, Action: "remove_member",
			Actor: operatorActor(r), TeamID: teamID, AgentID: agentID, Entity: agentID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "remove_member", "team_id": teamID, "agent_id": agentID}),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleListTasks(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := teamVisible(st, strings.TrimSpace(r.PathValue("id")), requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		list, err := st.ListTeamTasks(strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": list})
	}
}

func handleCreateTask(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID := strings.TrimSpace(r.PathValue("id"))
		if _, err := teamVisible(st, teamID, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		var body struct {
			Title           string `json:"title"`
			Status          string `json:"status"`
			AssigneeAgentID string `json:"assignee_agent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if aid := strings.TrimSpace(body.AssigneeAgentID); aid != "" {
			if _, err := agentVisible(st, aid, requestTenant(r)); err != nil {
				writeErr(w, http.StatusBadRequest, "agent not found")
				return
			}
		}
		task, err := st.CreateTeamTask(store.TeamTask{
			TeamID: teamID, Title: body.Title, Status: body.Status, AssigneeAgentID: body.AssigneeAgentID,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTask, Kind: eventstore.KindSuccess, Action: "create",
			Actor: operatorActor(r), TeamID: teamID, AgentID: task.AssigneeAgentID, Entity: task.ID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "create", "task_id": task.ID, "team_id": teamID, "status": task.Status}),
		})
		writeJSON(w, http.StatusCreated, task)
	}
}

func handleUpdateTask(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := strings.TrimSpace(r.PathValue("tid"))
		var body struct {
			Title           string `json:"title"`
			Status          string `json:"status"`
			AssigneeAgentID string `json:"assignee_agent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		task, err := st.UpdateTeamTask(store.TeamTask{
			ID: tid, Title: body.Title, Status: body.Status, AssigneeAgentID: body.AssigneeAgentID,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "task not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeTask, Kind: eventstore.KindSuccess, Action: "update",
			Actor: operatorActor(r), TeamID: task.TeamID, AgentID: task.AssigneeAgentID, Entity: task.ID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "update", "task_id": task.ID, "team_id": task.TeamID, "status": task.Status}),
		})
		writeJSON(w, http.StatusOK, task)
	}
}

func handleListMessagesTeam(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := teamVisible(st, strings.TrimSpace(r.PathValue("id")), requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		list, err := st.ListTeamMessages(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": list})
	}
}

func handleCreateMessageTeam(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID := strings.TrimSpace(r.PathValue("id"))
		if _, err := teamVisible(st, teamID, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "team not found")
			return
		}
		var body struct {
			FromAgentID string `json:"from_agent_id"`
			Body        string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		m, err := st.CreateTeamMessage(store.TeamMessage{
			TeamID: teamID, FromAgentID: body.FromAgentID, Body: body.Body,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		actor := strings.TrimSpace(m.FromAgentID)
		if actor == "" {
			actor = operatorActor(r)
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeMessage, Kind: eventstore.KindSuccess, Action: "create",
			Actor: actor, TeamID: teamID, AgentID: m.FromAgentID, Entity: m.ID,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "create", "message_id": m.ID, "team_id": teamID, "from_agent_id": m.FromAgentID, "bytes": len(m.Body)}),
		})
		writeJSON(w, http.StatusCreated, m)
	}
}

func handleListLinks(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := agentVisible(st, strings.TrimSpace(r.PathValue("id")), requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		from := strings.TrimSpace(r.PathValue("id"))
		if _, err := st.ListAgentLinks(from); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"links": publicAgentLinks(st, from)})
	}
}

func handleAddLink(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from := strings.TrimSpace(r.PathValue("id"))
		tid := requestTenant(r)
		if _, err := agentVisible(st, from, tid); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var body struct {
			ToAgentID     string `json:"to_agent_id"`
			Bidirectional bool   `json:"bidirectional"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		to := strings.TrimSpace(body.ToAgentID)
		if _, err := agentVisible(st, to, tid); err != nil {
			writeErr(w, http.StatusBadRequest, "agent not found")
			return
		}
		if err := st.AddAgentLink(from, to); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Bidirectional {
			_ = st.AddAgentLink(strings.TrimSpace(body.ToAgentID), from)
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeAgentLink, Kind: eventstore.KindSuccess, Action: "add",
			Actor: operatorActor(r), AgentID: from, Entity: to,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "add", "from_agent_id": from, "to_agent_id": to, "bidirectional": body.Bidirectional}),
		})
		writeJSON(w, http.StatusCreated, map[string]any{"links": publicAgentLinks(st, from)})
	}
}

func publicAgentLinks(st store.StoreIface, from string) []map[string]any {
	list, err := st.ListAgentLinks(from)
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(list))
	for _, l := range list {
		out = append(out, map[string]any{
			"from_agent_id": l.FromAgentID,
			"to_agent_id":   l.ToAgentID,
			"bidirectional": st.HasAgentLink(l.ToAgentID, from),
		})
	}
	return out
}

func handleRemoveLink(st store.StoreIface, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from := strings.TrimSpace(r.PathValue("id"))
		to := strings.TrimSpace(r.PathValue("to_id"))
		if _, err := agentVisible(st, from, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		if err := st.RemoveAgentLink(from, to); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "link not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		pair := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("pair")), "1") ||
			strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("pair")), "true")
		if pair {
			_ = st.RemoveAgentLink(to, from)
		}
		recordEvent(ev, eventstore.Event{
			Type: eventstore.TypeAgentLink, Kind: eventstore.KindSuccess, Action: "remove",
			Actor: operatorActor(r), AgentID: from, Entity: to,
			Summary: eventstore.SummarizeArgs(map[string]any{"action": "remove", "from_agent_id": from, "to_agent_id": to, "pair": pair}),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "links": publicAgentLinks(st, from)})
	}
}

func handleEvolution(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := agentVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		sugs := team.Suggestions(st, id)
		g := store.PublicGuardrails(st.GetEvolutionGuardrails(id))
		writeJSON(w, http.StatusOK, map[string]any{"suggestions": sugs, "guardrails": g})
	}
}

func handleEvolutionGuardrails(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := agentVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var body struct {
			AutoAdapt *bool    `json:"auto_adapt"`
			MinRuns   *int     `json:"min_runs"`
			Locked    []string `json:"locked"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		cur := st.GetEvolutionGuardrails(id)
		if body.AutoAdapt != nil {
			cur.AutoAdapt = *body.AutoAdapt
		}
		if body.MinRuns != nil {
			if *body.MinRuns <= 0 {
				writeErr(w, http.StatusBadRequest, "min_runs must be > 0")
				return
			}
			cur.MinRuns = *body.MinRuns
		}
		if body.Locked != nil {
			cur.Locked = store.MergeLocked(body.Locked)
		} else {
			cur.Locked = store.MergeLocked(cur.Locked)
		}
		if err := st.PutEvolutionGuardrails(id, cur); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"guardrails": store.PublicGuardrails(st.GetEvolutionGuardrails(id))})
	}
}

func handleEvolutionTick(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := agentVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		res, err := team.Tick(st, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleEvolutionApply(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := agentVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		sid := strings.TrimSpace(r.PathValue("sid"))
		var raw map[string]any
		_ = json.NewDecoder(r.Body).Decode(&raw)
		for k := range raw {
			if team.ForbiddenWrite(k) {
				writeErr(w, http.StatusBadRequest, "apply rejected: cannot change protected fields")
				return
			}
		}
		if team.ForbiddenWrite(sid) {
			writeErr(w, http.StatusBadRequest, "apply rejected: cannot change protected fields")
			return
		}
		a, err := team.Apply(st, id, sid)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}
