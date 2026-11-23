package httpapi

import (
	"encoding/json"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleGoClawCRUD(conn *websocket.Conn, st store.StoreIface, req goclawReq, who *auth.Identity) error {
	tenant := tenantOfIdentity(who)
	tnt := wsTenantFilter(who)
	switch req.Method {
	case "agents.create":
		return wsAgentCreate(conn, st, req, tenant)
	case "agents.update":
		return wsAgentUpdate(conn, st, req, tenant)
	case "agents.delete":
		return wsAgentDelete(conn, st, req, tenant)
	case "channels.instances.list":
		return wsChannelInstancesList(conn, st, req, tenant)
	case "channels.instances.create":
		return wsChannelInstanceCreate(conn, st, req, tenant)
	case "channels.instances.update":
		return wsChannelInstanceUpdate(conn, st, req, tenant)
	case "channels.instances.delete":
		return wsChannelInstanceDelete(conn, st, req, tenant)
	case "teams.tasks.get", "teams.tasks.get-light":
		return wsTeamTaskGet(conn, st, req, tnt)
	case "teams.tasks.assign":
		return wsTeamTaskPatch(conn, st, req, "assign", tnt, tenant)
	case "teams.tasks.approve":
		return wsTeamTaskPatch(conn, st, req, "approve", tnt, tenant)
	case "teams.tasks.reject":
		return wsTeamTaskPatch(conn, st, req, "reject", tnt, tenant)
	case "teams.tasks.comment":
		return wsTeamTaskComment(conn, st, req, who)
	case "teams.tasks.comments":
		return wsTeamTaskComments(conn, st, req, tenant, tnt)
	case "teams.tasks.events":
		return wsTeamTaskEvents(conn, st, req, tenant, tnt, true)
	case "teams.events.list":
		return wsTeamTaskEvents(conn, st, req, tenant, tnt, false)
	case "teams.tasks.delete":
		return wsTeamTaskDelete(conn, st, req, tnt)
	case "teams.tasks.delete-bulk":
		return wsTeamTaskDeleteBulk(conn, st, req, tnt)
	case "teams.tasks.attachments":
		return wsTeamTaskAttachments(conn, st, req, tenant)
	case "teams.tasks.attachment_add":
		return wsTeamTaskAttachmentAdd(conn, st, req, tenant, tnt)
	case "teams.tasks.attachment_delete":
		return wsTeamTaskAttachmentDelete(conn, st, req, tenant)
	case "teams.scopes":
		return writeGoClaw(conn, req.ID, map[string]any{"scopes": []any{}})
	case "teams.workspace.list":
		return writeGoClaw(conn, req.ID, map[string]any{"files": []any{}})
	case "teams.workspace.read":
		return writeGoClaw(conn, req.ID, map[string]any{"content": ""})
	case "teams.workspace.delete":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	case "sessions.preview":
		return wsSessionPreview(conn, st, req, tenant)
	case "run.timeline.get":
		return writeGoClaw(conn, req.ID, map[string]any{"items": []any{}})
	case "browser.pairing.status":
		return writeGoClaw(conn, req.ID, map[string]any{"status": "not_configured", "paired": false})
	case "chat_behavior.preview":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}

func wsAgentCreate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		AgentKey     string `json:"agent_key"`
		AgentKey2    string `json:"agentKey"`
		DisplayName  string `json:"display_name"`
		DisplayName2 string `json:"displayName"`
		Model        string `json:"model"`
		Provider     string `json:"provider"`
		Provider2    string `json:"llm_provider"`
		Instructions string `json:"instructions"`
	}
	_ = json.Unmarshal(req.Params, &p)
	key := firstNonEmpty(p.AgentKey, p.AgentKey2)
	if st == nil || key == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "agent_key is required")
	}
	a, err := st.CreateAgent(store.Agent{
		TenantID: tenant, AgentKey: key,
		DisplayName: firstNonEmpty(p.DisplayName, p.DisplayName2),
		Model:       p.Model, LLMProvider: firstNonEmpty(p.Provider, p.Provider2),
		Instructions: p.Instructions,
	})
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, publicAgentView(a, false, false))
}

func wsAgentUpdate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		ID           string  `json:"id"`
		ID2          string  `json:"agentId"`
		DisplayName  *string `json:"display_name"`
		DisplayName2 *string `json:"displayName"`
		Model        *string `json:"model"`
		Provider     *string `json:"provider"`
		Provider2    *string `json:"llm_provider"`
		Instructions *string `json:"instructions"`
		Enabled      *bool   `json:"enabled"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.ID, p.ID2)
	a, err := agentVisible(st, id, tenant)
	if err != nil || a == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "agent not found")
	}
	if v := p.DisplayName; v != nil {
		a.DisplayName = strings.TrimSpace(*v)
	}
	if p.DisplayName2 != nil {
		a.DisplayName = strings.TrimSpace(*p.DisplayName2)
	}
	if p.Model != nil {
		a.Model = strings.TrimSpace(*p.Model)
	}
	if v := p.Provider; v != nil {
		a.LLMProvider = strings.TrimSpace(*v)
	}
	if p.Provider2 != nil {
		a.LLMProvider = strings.TrimSpace(*p.Provider2)
	}
	if p.Instructions != nil {
		a.Instructions = *p.Instructions
	}
	if p.Enabled != nil {
		a.Enabled = *p.Enabled
	}
	upd, err := st.UpdateAgent(*a)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, publicAgentView(upd, false, false))
}

func wsAgentDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		ID  string `json:"id"`
		ID2 string `json:"agentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.ID, p.ID2)
	a, err := agentVisible(st, id, tenant)
	if err != nil || a == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "agent not found")
	}
	if err := st.DeleteAgent(a.ID); err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsChannelInstancesList(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	rows := []map[string]any{}
	if st != nil {
		for _, cfg := range st.ListChannelConfigs() {
			if cfg == nil {
				continue
			}
			if owner, ok := channel.TenantOf(cfg.Name); ok && !store.SameTenant(owner, tenant) {
				continue
			}
			rows = append(rows, channelInstanceView(cfg))
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"instances": rows, "total": len(rows)})
}

func wsChannelInstanceCreate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		Name        string `json:"name"`
		ChannelType string `json:"channel_type"`
		Type        string `json:"type"`
		AgentID     string `json:"agent_id"`
		Enabled     *bool  `json:"enabled"`
	}
	_ = json.Unmarshal(req.Params, &p)
	name := strings.TrimSpace(p.Name)
	catalog := firstNonEmpty(p.ChannelType, p.Type)
	if st == nil || name == "" || catalog == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "name and channel_type are required")
	}
	full := tenant + "/" + catalog + "/" + name
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	cfg := store.ChannelConfig{Name: full, Enabled: enabled, AgentID: strings.TrimSpace(p.AgentID)}
	if err := st.PutChannelConfig(cfg); err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	saved, _ := st.GetChannelConfig(full)
	if saved == nil {
		saved = &cfg
	}
	return writeGoClaw(conn, req.ID, channelInstanceView(saved))
}

func wsChannelInstanceUpdate(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		ID      string  `json:"id"`
		AgentID *string `json:"agent_id"`
		Enabled *bool   `json:"enabled"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := strings.TrimSpace(p.ID)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "id is required")
	}
	if owner, ok := channel.TenantOf(id); ok && !store.SameTenant(owner, tenant) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "instance not found")
	}
	cfg, err := st.GetChannelConfig(id)
	if err != nil || cfg == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "instance not found")
	}
	if p.AgentID != nil {
		cfg.AgentID = strings.TrimSpace(*p.AgentID)
	}
	if p.Enabled != nil {
		cfg.Enabled = *p.Enabled
	}
	if err := st.PutChannelConfig(*cfg); err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, channelInstanceView(cfg))
}

func wsChannelInstanceDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := strings.TrimSpace(p.ID)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "id is required")
	}
	if owner, ok := channel.TenantOf(id); ok && !store.SameTenant(owner, tenant) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "instance not found")
	}
	if err := st.DeleteChannelConfig(id); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "instance not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsTeamTaskGet(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TaskID  string `json:"taskId"`
		TaskID2 string `json:"id"`
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
	return writeGoClaw(conn, req.ID, wsTeamTaskView(t))
}

func wsTeamTaskPatch(conn *websocket.Conn, st store.StoreIface, req goclawReq, action string, tnt, tenant string) error {
	var p struct {
		TaskID   string `json:"taskId"`
		TaskID2  string `json:"id"`
		Assignee string `json:"assigneeAgentId"`
		Status   string `json:"status"`
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
	eventKind := "teams.tasks." + action
	if xt := store.TeamTasks(st); xt != nil {
		_, _ = xt.PutTeamTaskEvent(store.TeamTaskEvent{TenantID: tenant, TaskID: t.ID, Kind: eventKind, Actor: "api"})
	}
	switch action {
	case "assign":
		if strings.TrimSpace(p.Assignee) != "" {
			t.AssigneeAgentID = strings.TrimSpace(p.Assignee)
		}
	case "approve":
		t.Status = "done"
	case "reject":
		t.Status = "todo"
	}
	upd, err := st.UpdateTeamTask(*t)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, wsTeamTaskView(upd))
}

func wsTeamTaskComment(conn *websocket.Conn, st store.StoreIface, req goclawReq, who *auth.Identity) error {
	var p struct {
		TaskID  string `json:"taskId"`
		Body    string `json:"body"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := strings.TrimSpace(p.TaskID)
	body := firstNonEmpty(p.Body, p.Content)
	if st == nil || id == "" || body == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "taskId and body are required")
	}
	t, err := st.GetTeamTask(id)
	if err != nil || t == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "task not found")
	}
	from := "system"
	if who != nil {
		from = firstNonEmpty(who.Email, who.UserID, "system")
	}
	if xt := store.TeamTasks(st); xt != nil {
		row, cerr := xt.PutTeamTaskComment(store.TeamTaskComment{TenantID: tenantOfIdentity(who), TaskID: t.ID, Author: from, Body: body})
		if cerr != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", cerr.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "comment": row})
	}
	msg := store.TeamMessage{TeamID: t.TeamID, FromAgentID: from, Body: body}
	if _, err := st.CreateTeamMessage(msg); err != nil {
		// A human commenter is not an agent row; fall back to the task assignee
		// or any team agent so the note lands in the team mailbox.
		fallback := strings.TrimSpace(t.AssigneeAgentID)
		if fallback == "" {
			if members, _ := st.ListTeamMembers(t.TeamID); len(members) > 0 && members[0] != nil {
				fallback = members[0].AgentID
			}
		}
		if fallback == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		msg.FromAgentID = fallback
		if _, err2 := st.CreateTeamMessage(msg); err2 != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err2.Error())
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsSessionPreview(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		SessionID  string `json:"sessionId"`
		SessionID2 string `json:"session_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.SessionID, p.SessionID2)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "sessionId is required")
	}
	sess, err := st.GetSession(id)
	if err != nil || sess == nil || !store.SameTenant(sess.TenantID, tenant) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "session not found")
	}
	msgs, _ := st.ListMessages(id)
	out := []map[string]any{}
	start := len(msgs) - 3
	if start < 0 {
		start = 0
	}
	for _, m := range msgs[start:] {
		if m == nil {
			continue
		}
		out = append(out, map[string]any{"role": m.Role, "content": m.Content, "created_at": m.CreatedAt})
	}
	return writeGoClaw(conn, req.ID, map[string]any{"session": sess, "messages": out})
}

func wsTeamTaskComments(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant, tnt string) error {
	var p struct {
		TaskID  string `json:"taskId"`
		TaskID2 string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.TaskID, p.TaskID2)
	if xt := store.TeamTasks(st); xt != nil && id != "" {
		rows := xt.ListTeamTaskComments(tenant, id)
		return writeGoClaw(conn, req.ID, map[string]any{"comments": rows, "total": len(rows)})
	}
	return writeGoClaw(conn, req.ID, map[string]any{"comments": []any{}})
}

func wsTeamTaskEvents(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant, tnt string, scoped bool) error {
	var p struct {
		TaskID  string `json:"taskId"`
		TaskID2 string `json:"id"`
		TeamID  string `json:"teamId"`
		Limit   int    `json:"limit"`
	}
	_ = json.Unmarshal(req.Params, &p)
	taskID := ""
	if scoped {
		taskID = firstNonEmpty(p.TaskID, p.TaskID2)
	}
	if xt := store.TeamTasks(st); xt != nil {
		rows := xt.ListTeamTaskEvents(tenant, taskID, p.Limit)
		return writeGoClaw(conn, req.ID, map[string]any{"events": rows, "total": len(rows)})
	}
	return writeGoClaw(conn, req.ID, map[string]any{"events": []any{}})
}

func wsTeamTaskDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TaskID  string `json:"taskId"`
		TaskID2 string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.TaskID, p.TaskID2)
	t, err := st.GetTeamTask(id)
	if err != nil || t == nil || !wsTaskAllowed(st, t, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "task not found")
	}
	if err := st.DeleteTeamTask(t.ID); err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "deleted": 1})
}

func wsTeamTaskDeleteBulk(conn *websocket.Conn, st store.StoreIface, req goclawReq, tnt string) error {
	var p struct {
		TaskIDs  []string `json:"taskIds"`
		TaskIDs2 []string `json:"ids"`
	}
	_ = json.Unmarshal(req.Params, &p)
	ids := p.TaskIDs
	if len(ids) == 0 {
		ids = p.TaskIDs2
	}
	deleted := 0
	for _, id := range ids {
		t, err := st.GetTeamTask(id)
		if err != nil || t == nil || !wsTaskAllowed(st, t, tnt) {
			continue
		}
		if st.DeleteTeamTask(t.ID) == nil {
			deleted++
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "deleted": deleted})
}

func wsTeamTaskAttachments(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		TaskID  string `json:"taskId"`
		TaskID2 string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.TaskID, p.TaskID2)
	x := store.Attachments(st)
	if x == nil || id == "" {
		return writeGoClaw(conn, req.ID, map[string]any{"attachments": []any{}, "total": 0})
	}
	rows := x.ListTeamTaskAttachments(tenant, id)
	return writeGoClaw(conn, req.ID, map[string]any{"attachments": rows, "total": len(rows)})
}

func wsTeamTaskAttachmentAdd(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant, tnt string) error {
	var p struct {
		TaskID   string `json:"taskId"`
		TaskID2  string `json:"id"`
		Path     string `json:"path"`
		FileSize int64  `json:"file_size"`
		MimeType string `json:"mime_type"`
		ChatID   string `json:"chat_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.TaskID, p.TaskID2)
	t, err := st.GetTeamTask(id)
	if err != nil || t == nil || !wsTaskAllowed(st, t, tnt) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "task not found")
	}
	x := store.Attachments(st)
	if x == nil {
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", "attachments unavailable")
	}
	saved, perr := x.PutTeamTaskAttachment(store.TeamTaskAttachment{
		TenantID: tenant, TaskID: t.ID, TeamID: t.TeamID, ChatID: p.ChatID,
		Path: p.Path, FileSize: p.FileSize, MimeType: p.MimeType,
	})
	if perr != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", perr.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"attachment": saved})
}

func wsTeamTaskAttachmentDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq, tenant string) error {
	var p struct {
		ID     string `json:"id"`
		FileID string `json:"attachmentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	x := store.Attachments(st)
	if x == nil {
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", "attachments unavailable")
	}
	if err := x.DeleteTeamTaskAttachment(tenant, firstNonEmpty(p.FileID, p.ID)); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "attachment not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}
