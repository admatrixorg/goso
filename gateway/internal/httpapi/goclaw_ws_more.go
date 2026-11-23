package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/logstore"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

// wsDeps carries the optional runtime pieces the WS control plane needs.
// registerGoClawExtraDeps fills it from NewRouter once per process.
type wsDeps struct {
	Store    store.StoreIface
	Gate     *approval.Gate
	Runtime  *agent.Runtime
	Provider llm.Provider
	Meter    *billing.Store
	Logs     *logstore.Store
	Audit    *auditlog.Store
}

var wsD wsDeps

func registerGoClawExtraDeps(opt Options) {
	wsD = wsDeps{
		Store: opt.Store, Gate: opt.Gate, Runtime: opt.Runtime,
		Provider: opt.Provider, Meter: opt.Meter, Logs: opt.Logs, Audit: opt.Audit,
	}
}

func handleGoClawMore(conn *websocket.Conn, st store.StoreIface, req goclawReq, who *auth.Identity) error {
	tnt := wsTenantFilter(who)
	if st == nil {
		st = wsD.Store
	}
	switch req.Method {
	case "teams.list":
		return wsTeamsList(conn, st, req, tnt)
	case "teams.get":
		return wsTeamGet(conn, st, req, tnt)
	case "teams.create":
		return wsTeamCreate(conn, st, req, tnt)
	case "teams.update":
		return wsTeamUpdate(conn, st, req, tnt)
	case "teams.delete":
		return wsTeamDelete(conn, st, req, tnt)
	case "teams.members.add":
		return wsTeamMemberAdd(conn, st, req, tnt)
	case "teams.members.remove":
		return wsTeamMemberRemove(conn, st, req, tnt)
	case "teams.tasks.list":
		return wsTeamTasksList(conn, st, req, tnt)
	case "teams.tasks.create":
		return wsTeamTaskCreate(conn, st, req, tnt)
	case "teams.tasks.update":
		return wsTeamTaskUpdate(conn, st, req, tnt)
	case "config.get":
		return wsConfigGet(conn, st, req)
	case "config.patch", "config.apply":
		return wsConfigPatch(conn, st, req)
	case "config.defaults":
		return writeGoClaw(conn, req.ID, map[string]any{"values": map[string]any{}})
	case "skills.list":
		return wsSkillsList(conn, st, req)
	case "skills.get":
		return wsSkillGet(conn, st, req)
	case "exec.approval.list":
		return wsApprovalsList(conn, req)
	case "exec.approval.approve":
		return wsApprovalDecide(conn, req, approval.DecisionApprove)
	case "exec.approval.deny":
		return wsApprovalDecide(conn, req, approval.DecisionDeny)
	case "usage.get":
		return writeGoClaw(conn, req.ID, map[string]any{"records": []any{}, "total": 0})
	case "usage.summary":
		return wsUsageSummary(conn, req)
	case "pairing.list":
		return writeGoClaw(conn, req.ID, map[string]any{"pending": []any{}})
	case "pairing.approve", "pairing.deny", "pairing.revoke":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	case "logs.tail":
		return wsLogsTail(conn, req)
	case "sessions.patch":
		return wsSessionPatch(conn, st, req)
	case "sessions.reset":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	case "agents.links.list":
		return wsAgentLinksList(conn, st, req)
	case "agents.links.create", "agents.links.update":
		return wsAgentLinkAdd(conn, st, req)
	case "agents.links.delete":
		return wsAgentLinkRemove(conn, st, req)
	case "cron.update":
		return wsCronUpdate(conn, st, req)
	case "chat.behavior.preview":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}


// wsTenantFilter returns the tenant a caller is limited to. Empty means the
// operator token or an owner, who may read every tenant.
func wsTenantFilter(who *auth.Identity) string {
	if who == nil || who.IsOwner() {
		return ""
	}
	return who.TenantID
}

func wsTeamAllowed(st store.StoreIface, teamID, tenant string) bool {
	if tenant == "" {
		return true
	}
	if st == nil {
		return false
	}
	t, err := st.GetTeam(teamID)
	if err != nil || t == nil {
		return false
	}
	return store.SameTenant(t.TenantID, tenant)
}

func wsTaskAllowed(st store.StoreIface, task *store.TeamTask, tenant string) bool {
	if task == nil {
		return false
	}
	return wsTeamAllowed(st, task.TeamID, tenant)
}

func wsTeamView(t *store.Team, members []*store.TeamMember, agents map[string]*store.Agent) map[string]any {
	lead := ""
	if t != nil && t.LeadAgentID != "" {
		if a := agents[t.LeadAgentID]; a != nil {
			lead = a.AgentKey
		}
	}
	outMembers := []map[string]any{}
	for _, m := range members {
		row := map[string]any{"team_id": m.TeamID, "agent_id": m.AgentID, "role": m.Role}
		if a := agents[m.AgentID]; a != nil {
			row["agent_key"] = a.AgentKey
			row["display_name"] = a.DisplayName
		}
		outMembers = append(outMembers, row)
	}
	created := ""
	if t != nil && !t.CreatedAt.IsZero() {
		created = t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return map[string]any{
		"id": t.ID, "name": t.Name, "lead_agent_id": t.LeadAgentID, "lead_agent_key": lead,
		"status": "active", "created_by": "", "created_at": created, "updated_at": created,
		"member_count": len(outMembers), "members": outMembers,
	}
}

func wsAgentIndex(st store.StoreIface) map[string]*store.Agent {
	idx := map[string]*store.Agent{}
	if st == nil {
		return idx
	}
	for _, a := range st.ListAgents() {
		if a != nil {
			idx[a.ID] = a
		}
	}
	return idx
}

func wsTeamsList(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	out := []map[string]any{}
	if st != nil {
		agents := wsAgentIndex(st)
		for _, t := range st.ListTeams() {
			if t == nil {
				continue
			}
			if tnt != "" && !store.SameTenant(t.TenantID, tnt) {
				continue
			}
			members, _ := st.ListTeamMembers(t.ID)
			out = append(out, wsTeamView(t, members, agents))
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"teams": out})
}

func wsTeamGet(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	t, err := st.GetTeam(strings.TrimSpace(p.ID))
	if err != nil || t == nil || !wsTeamAllowed(st, t.ID, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	members, _ := st.ListTeamMembers(t.ID)
	return writeGoClaw(conn, req.ID, wsTeamView(t, members, wsAgentIndex(st)))
}

func wsTeamCreate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		Name        string `json:"name"`
		LeadAgentID string `json:"lead_agent_id"`
		LeadAgentId2 string `json:"leadAgentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	lead := firstNonEmpty(p.LeadAgentID, p.LeadAgentId2)
	name := strings.TrimSpace(p.Name)
	if st == nil || name == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "name is required")
	}
	owner := tnt
	if owner == "" {
		owner = tenant.Default
	}
	t, err := st.CreateTeam(store.Team{TenantID: owner, Name: name, LeadAgentID: lead})
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	if lead != "" {
		_, _ = st.AddTeamMember(store.TeamMember{TeamID: t.ID, AgentID: lead, Role: "lead"})
	}
	members, _ := st.ListTeamMembers(t.ID)
	return writeGoClaw(conn, req.ID, wsTeamView(t, members, wsAgentIndex(st)))
}

func wsTeamUpdate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		LeadAgentID string `json:"lead_agent_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	t, err := st.GetTeam(strings.TrimSpace(p.ID))
	if err != nil || t == nil || !wsTeamAllowed(st, t.ID, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	if strings.TrimSpace(p.Name) != "" {
		t.Name = strings.TrimSpace(p.Name)
	}
	if strings.TrimSpace(p.LeadAgentID) != "" {
		t.LeadAgentID = strings.TrimSpace(p.LeadAgentID)
	}
	updated, err := st.UpdateTeam(*t)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	members, _ := st.ListTeamMembers(updated.ID)
	return writeGoClaw(conn, req.ID, wsTeamView(updated, members, wsAgentIndex(st)))
}

func wsTeamDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	if !wsTeamAllowed(st, strings.TrimSpace(p.ID), tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	if err := st.DeleteTeam(strings.TrimSpace(p.ID)); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsTeamMemberAdd(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TeamID  string `json:"teamId"`
		TeamID2 string `json:"team_id"`
		AgentID string `json:"agentId"`
		AgentID2 string `json:"agent_id"`
		Role    string `json:"role"`
	}
	_ = json.Unmarshal(req.Params, &p)
	tid := firstNonEmpty(p.TeamID, p.TeamID2)
	aid := firstNonEmpty(p.AgentID, p.AgentID2)
	if st == nil || tid == "" || aid == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "team and agent are required")
	}
	if !wsTeamAllowed(st, tid, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	role := strings.TrimSpace(p.Role)
	if role == "" {
		role = "member"
	}
	m, err := st.AddTeamMember(store.TeamMember{TeamID: tid, AgentID: aid, Role: role})
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"team_id": m.TeamID, "agent_id": m.AgentID, "role": m.Role})
}

func wsTeamMemberRemove(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TeamID  string `json:"teamId"`
		TeamID2 string `json:"team_id"`
		AgentID string `json:"agentId"`
		AgentID2 string `json:"agent_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	tid := firstNonEmpty(p.TeamID, p.TeamID2)
	aid := firstNonEmpty(p.AgentID, p.AgentID2)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "store unavailable")
	}
	if !wsTeamAllowed(st, tid, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	if err := st.RemoveTeamMember(tid, aid); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "member not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsTeamTaskView(t *store.TeamTask) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id": t.ID, "team_id": t.TeamID, "title": t.Title, "status": t.Status,
		"assignee_agent_id": t.AssigneeAgentID,
	}
}

func wsTeamTasksList(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TeamID  string `json:"teamId"`
		TeamID2 string `json:"team_id"`
		Status  string `json:"status"`
	}
	_ = json.Unmarshal(req.Params, &p)
	tid := firstNonEmpty(p.TeamID, p.TeamID2)
	out := []map[string]any{}
	if st != nil && (tid == "" || wsTeamAllowed(st, tid, tnt)) {
		tasks, _ := st.ListTeamTasks(tid, strings.TrimSpace(p.Status))
		for _, t := range tasks {
			if !wsTaskAllowed(st, t, tnt) {
				continue
			}
			out = append(out, wsTeamTaskView(t))
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"tasks": out})
}

func wsTeamTaskCreate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TeamID   string `json:"teamId"`
		TeamID2  string `json:"team_id"`
		Title    string `json:"title"`
		Status   string `json:"status"`
		Assignee string `json:"assigneeAgentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	tid := firstNonEmpty(p.TeamID, p.TeamID2)
	if st == nil || tid == "" || strings.TrimSpace(p.Title) == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "team and title are required")
	}
	if !wsTeamAllowed(st, tid, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "team not found")
	}
	status := strings.TrimSpace(p.Status)
	if status == "" {
		status = "todo"
	}
	t, err := st.CreateTeamTask(store.TeamTask{TeamID: tid, Title: strings.TrimSpace(p.Title), Status: status, AssigneeAgentID: strings.TrimSpace(p.Assignee)})
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, wsTeamTaskView(t))
}

func wsTeamTaskUpdate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TaskID   string `json:"taskId"`
		TaskID2  string `json:"id"`
		Status   string `json:"status"`
		Title    string `json:"title"`
		Assignee string `json:"assigneeAgentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.TaskID, p.TaskID2)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "task is required")
	}
	t, err := st.GetTeamTask(id)
	if err != nil || t == nil || !wsTaskAllowed(st, t, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "task not found")
	}
	if strings.TrimSpace(p.Status) != "" {
		t.Status = strings.TrimSpace(p.Status)
	}
	if strings.TrimSpace(p.Title) != "" {
		t.Title = strings.TrimSpace(p.Title)
	}
	if strings.TrimSpace(p.Assignee) != "" {
		t.AssigneeAgentID = strings.TrimSpace(p.Assignee)
	}
	updated, err := st.UpdateTeamTask(*t)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, wsTeamTaskView(updated))
}

func wsConfigGet(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	rec := httptest.NewRecorder()
	handleGetConfig(st)(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var cfg map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &cfg)
	hash := ""
	if v, ok := cfg["updated_at"].(string); ok {
		hash = v
	}
	return writeGoClaw(conn, req.ID, map[string]any{"config": cfg, "hash": hash, "path": ""})
}

func wsConfigPatch(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		Values map[string]string `json:"values"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "store unavailable")
	}
	cur, _ := st.GetGatewaySettings()
	values := map[string]string{}
	if cur != nil {
		for k, v := range cur.Values {
			values[k] = v
		}
	}
	for k, v := range p.Values {
		values[k] = v
	}
	if _, err := st.PutGatewaySettings(store.GatewaySettings{Values: values}); err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsSkillsList(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	rec := httptest.NewRecorder()
	handleListSkills()(rec, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return writeGoClaw(conn, req.ID, map[string]any{"skills": []any{}})
	}
	return writeGoClaw(conn, req.ID, out)
}

func wsSkillGet(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	want := firstNonEmpty(p.Name, p.ID)
	rec := httptest.NewRecorder()
	handleListSkills()(rec, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	var out struct {
		Skills []map[string]any `json:"skills"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	for _, s := range out.Skills {
		if v, ok := s["name"].(string); ok && v == want {
			return writeGoClaw(conn, req.ID, s)
		}
	}
	return writeGoClawErr(conn, req.ID, "NOT_FOUND", "skill not found")
}

func wsApprovalsList(conn *websocket.Conn, req goclawReq) error {
	out := []any{}
	if wsD.Gate != nil {
		for _, r := range wsD.Gate.List(approval.StatusPending) {
			out = append(out, r)
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"pending": out})
}

func wsApprovalDecide(conn *websocket.Conn, req goclawReq, want string) error {
	var p struct {
		ID      string `json:"approvalId"`
		ID2     string `json:"approval_id"`
		ID3     string `json:"id"`
		Reason  string `json:"reason"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.ID, p.ID2, p.ID3)
	if wsD.Gate == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "approval not found")
	}
	r, err := wsD.Gate.DecideReason(context.Background(), id, want, p.Reason)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, r)
}

func wsUsageSummary(conn *websocket.Conn, req goclawReq) error {
	total := 0
	if wsD.Meter != nil {
		rec := httptest.NewRecorder()
		handleUsage(wsD.Meter)(rec, httptest.NewRequest(http.MethodGet, "/api/usage", nil))
		var body map[string]any
		if json.Unmarshal(rec.Body.Bytes(), &body) == nil {
			if v, ok := body["calls"].(float64); ok {
				total = int(v)
			}
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"byAgent": map[string]any{}, "totalRecords": total})
}

func wsLogsTail(conn *websocket.Conn, req goclawReq) error {
	var p struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(req.Params, &p)
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := []any{}
	if wsD.Logs != nil {
		rec := httptest.NewRecorder()
		handleListLogs(Options{Logs: wsD.Logs})(rec, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
		var body struct {
			Logs []any `json:"logs"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		for i := len(body.Logs) - 1; i >= 0 && len(out) < limit; i-- {
			out = append(out, body.Logs[i])
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"logs": out})
}

func wsSessionPatch(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		SessionID string `json:"sessionId"`
		SessionID2 string `json:"session_id"`
		Label     string `json:"label"`
		Archived  *bool  `json:"archived"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.SessionID, p.SessionID2)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "session not found")
	}
	sess, err := st.GetSession(id)
	if err != nil || sess == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "session not found")
	}
	if strings.TrimSpace(p.Label) != "" {
		sess.Label = strings.TrimSpace(p.Label)
		sess.TouchLabel = true
	}
	if p.Archived != nil {
		sess.Archived = *p.Archived
		sess.TouchArchive = true
	}
	updated, err := st.UpdateSession(*sess)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"session": updated})
}

func wsAgentLinksList(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		AgentID string `json:"agentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	out := []any{}
	if st != nil && strings.TrimSpace(p.AgentID) != "" {
		links, _ := st.ListAgentLinks(strings.TrimSpace(p.AgentID))
		for _, l := range links {
			out = append(out, l)
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"links": out})
}

func wsAgentLinkAdd(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		From   string `json:"fromAgentId"`
		From2  string `json:"from_agent_id"`
		To     string `json:"toAgentId"`
		To2    string `json:"to_agent_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	from := firstNonEmpty(p.From, p.From2)
	to := firstNonEmpty(p.To, p.To2)
	if st == nil || from == "" || to == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "from and to are required")
	}
	if err := st.AddAgentLink(from, to); err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsAgentLinkRemove(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		From  string `json:"fromAgentId"`
		From2 string `json:"from_agent_id"`
		To    string `json:"toAgentId"`
		To2   string `json:"to_agent_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	from := firstNonEmpty(p.From, p.From2)
	to := firstNonEmpty(p.To, p.To2)
	if st == nil || from == "" || to == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "from and to are required")
	}
	if err := st.RemoveAgentLink(from, to); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "link not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsCronUpdate(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		JobID   string `json:"jobId"`
		JobID2  string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.JobID, p.JobID2)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "job is required")
	}
	if p.Enabled != nil {
		if err := st.SetCronEnabled(id, *p.Enabled); err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "job not found")
		}
	}
	j, err := st.GetCronJob(id)
	if err != nil || j == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "job not found")
	}
	return writeGoClaw(conn, req.ID, wsCronUI(j))
}
