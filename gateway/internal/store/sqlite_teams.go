// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) CreateTeam(t Team) (*Team, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return nil, errors.New("name is required")
	}
	t.TenantID = NormalizeTenant(t.TenantID)
	if LiteEnabled() {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM teams WHERE tenant_id=?`, t.TenantID).Scan(&n); err != nil {
			return nil, err
		}
		if n >= LiteMaxTeams {
			return nil, ErrLiteCap
		}
	}
	if t.LeadAgentID != "" {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id=?`, t.LeadAgentID).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, errors.New("lead agent not found")
		}
	}
	t.ID = newID()
	t.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO teams(id, name, lead_agent_id, created_at, tenant_id) VALUES(?,?,?,?,?)`,
		t.ID, t.Name, t.LeadAgentID, formatTime(t.CreatedAt), t.TenantID)
	if err != nil {
		return nil, err
	}
	if t.LeadAgentID != "" {
		_, _ = s.db.Exec(`INSERT OR IGNORE INTO team_members(team_id, agent_id, role) VALUES(?,?,?)`,
			t.ID, t.LeadAgentID, "lead")
	}
	cp := t
	return &cp, nil
}

func (s *SQLiteStore) ListTeams() []*Team {
	rows, err := s.db.Query(`SELECT id, name, lead_agent_id, created_at, tenant_id FROM teams ORDER BY created_at`)
	if err != nil {
		return []*Team{}
	}
	defer rows.Close()
	var out []*Team
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	if out == nil {
		out = []*Team{}
	}
	return out
}

func (s *SQLiteStore) GetTeam(id string) (*Team, error) {
	row := s.db.QueryRow(`SELECT id, name, lead_agent_id, created_at, tenant_id FROM teams WHERE id=?`, id)
	t, err := scanTeam(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func scanTeam(sc scanner) (*Team, error) {
	var t Team
	var ts string
	var lead, tenant sql.NullString
	if err := sc.Scan(&t.ID, &t.Name, &lead, &ts, &tenant); err != nil {
		return nil, err
	}
	t.LeadAgentID = lead.String
	t.TenantID = NormalizeTenant(tenant.String)
	t.CreatedAt = parseTime(ts)
	return &t, nil
}

func (s *SQLiteStore) UpdateTeam(t Team) (*Team, error) {
	cur, err := s.GetTeam(t.ID)
	if err != nil {
		return nil, err
	}
	if name := strings.TrimSpace(t.Name); name != "" {
		cur.Name = name
	}
	if t.LeadAgentID != "" {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id=?`, t.LeadAgentID).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, errors.New("lead agent not found")
		}
		cur.LeadAgentID = t.LeadAgentID
	}
	_, err = s.db.Exec(`UPDATE teams SET name=?, lead_agent_id=? WHERE id=?`, cur.Name, cur.LeadAgentID, cur.ID)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

func (s *SQLiteStore) DeleteTeam(id string) error {
	res, err := s.db.Exec(`DELETE FROM teams WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM team_members WHERE team_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM team_tasks WHERE team_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM team_messages WHERE team_id=?`, id)
	return nil
}

func (s *SQLiteStore) AddTeamMember(m TeamMember) (*TeamMember, error) {
	m.AgentID = strings.TrimSpace(m.AgentID)
	m.TeamID = strings.TrimSpace(m.TeamID)
	m.Role = strings.ToLower(strings.TrimSpace(m.Role))
	if m.TeamID == "" || m.AgentID == "" {
		return nil, errors.New("team_id and agent_id are required")
	}
	if m.Role == "" {
		m.Role = "member"
	}
	if !validMemberRole(m.Role) {
		return nil, errors.New("role must be lead or member")
	}
	if _, err := s.GetTeam(m.TeamID); err != nil {
		return nil, errors.New("team not found")
	}
	if _, err := s.GetAgent(m.AgentID); err != nil {
		return nil, errors.New("agent not found")
	}
	_, err := s.db.Exec(`INSERT INTO team_members(team_id, agent_id, role) VALUES(?,?,?)
		ON CONFLICT(team_id, agent_id) DO UPDATE SET role=excluded.role`, m.TeamID, m.AgentID, m.Role)
	if err != nil {
		return nil, err
	}
	if m.Role == "lead" {
		_, _ = s.db.Exec(`UPDATE teams SET lead_agent_id=? WHERE id=?`, m.AgentID, m.TeamID)
	}
	cp := m
	return &cp, nil
}

func (s *SQLiteStore) ListTeamMembers(teamID string) ([]*TeamMember, error) {
	if _, err := s.GetTeam(teamID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT team_id, agent_id, role FROM team_members WHERE team_id=?`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TeamMember{}
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.TeamID, &m.AgentID, &m.Role); err != nil {
			continue
		}
		cp := m
		out = append(out, &cp)
	}
	return out, nil
}

func (s *SQLiteStore) RemoveTeamMember(teamID, agentID string) error {
	if _, err := s.GetTeam(teamID); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM team_members WHERE team_id=? AND agent_id=?`, teamID, agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) TeamOfAgent(agentID string) (*Team, error) {
	var teamID string
	err := s.db.QueryRow(`SELECT team_id FROM team_members WHERE agent_id=? LIMIT 1`, agentID).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetTeam(teamID)
}

const teamTaskColumns = `id, team_id, title, subject, description, status, priority, assignee_agent_id, owner_agent_id,
	assignee_user_id, created_by_agent_id, parent_id, user_id, channel, chat_id, task_type, task_number, identifier,
	result, blocked_by, progress_percent, progress_step, followup_at, followup_count, followup_max, followup_message,
	followup_channel, followup_chat_id, metadata, tenant_id, created_at, updated_at`

func (s *SQLiteStore) CreateTeamTask(t TeamTask) (*TeamTask, error) {
	t.Title = strings.TrimSpace(firstNonEmptyStr(t.Title, t.Subject))
	t.Subject = strings.TrimSpace(firstNonEmptyStr(t.Subject, t.Title))
	t.TeamID = strings.TrimSpace(t.TeamID)
	st, ok := NormalizeTeamTaskStatus(t.Status)
	if !ok {
		st = "todo"
	}
	t.Status = st
	if t.Title == "" || t.TeamID == "" {
		return nil, errors.New("team_id and title are required")
	}
	if _, err := s.GetTeam(t.TeamID); err != nil {
		return nil, errors.New("team not found")
	}
	if t.AssigneeAgentID != "" {
		if _, err := s.GetAgent(t.AssigneeAgentID); err != nil {
			return nil, errors.New("assignee not found")
		}
	}
	if t.TaskType == "" {
		t.TaskType = "general"
	}
	if t.TenantID == "" {
		if team, err := s.GetTeam(t.TeamID); err == nil && team != nil {
			t.TenantID = team.TenantID
		}
	}
	if t.TenantID == "" {
		t.TenantID = NormalizeTenant("")
	}
	if t.TaskNumber == 0 {
		var maxN int
		_ = s.db.QueryRow(`SELECT coalesce(max(task_number),0) FROM team_tasks WHERE team_id=?`, t.TeamID).Scan(&maxN)
		t.TaskNumber = maxN + 1
	}
	if t.Identifier == "" && t.TaskNumber > 0 {
		t.Identifier = fmt.Sprintf("T-%d", t.TaskNumber)
	}
	t.ID = newID()
	now := formatTime(time.Now().UTC())
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO team_tasks(`+teamTaskColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.TeamID, t.Title, t.Subject, t.Description, t.Status, t.Priority, t.AssigneeAgentID,
		t.OwnerAgentID, t.AssigneeUserID, t.CreatedByAgentID, t.ParentID, t.UserID, t.Channel, t.ChatID,
		t.TaskType, t.TaskNumber, t.Identifier, t.Result, encodeTaskIDs(t.BlockedBy), t.ProgressPercent,
		t.ProgressStep, t.FollowupAt, t.FollowupCount, t.FollowupMax, t.FollowupMessage, t.FollowupChannel,
		t.FollowupChatID, t.Metadata, t.TenantID, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return s.GetTeamTask(t.ID)
}

func encodeTaskIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return strings.Join(ids, ",")
}

func decodeTaskIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *SQLiteStore) ListTeamTasks(teamID, status string) ([]*TeamTask, error) {
	if _, err := s.GetTeam(teamID); err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(`SELECT `+teamTaskColumns+` FROM team_tasks WHERE team_id=? ORDER BY task_number, created_at`, teamID)
	} else {
		rows, err = s.db.Query(`SELECT `+teamTaskColumns+` FROM team_tasks WHERE team_id=? AND status=? ORDER BY task_number, created_at`, teamID, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TeamTask{}
	for rows.Next() {
		tk, err := scanTaskRow(rows)
		if err != nil {
			continue
		}
		out = append(out, tk)
	}
	return out, nil
}

func (s *SQLiteStore) GetTeamTask(id string) (*TeamTask, error) {
	row := s.db.QueryRow(`SELECT `+teamTaskColumns+` FROM team_tasks WHERE id=?`, strings.TrimSpace(id))
	tk, err := scanTaskRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return tk, nil
}

func scanTaskRow(sc scanner) (*TeamTask, error) {
	var t TeamTask
	var assignee, owner, assigneeUser, createdBy, parent, blockedBy, followupAt, followupMsg, followupCh, followupChat sql.NullString
	err := sc.Scan(&t.ID, &t.TeamID, &t.Title, &t.Subject, &t.Description, &t.Status, &t.Priority,
		&assignee, &owner, &assigneeUser, &createdBy, &parent, &t.UserID, &t.Channel, &t.ChatID,
		&t.TaskType, &t.TaskNumber, &t.Identifier, &t.Result, &blockedBy, &t.ProgressPercent,
		&t.ProgressStep, &followupAt, &t.FollowupCount, &t.FollowupMax, &followupMsg, &followupCh,
		&followupChat, &t.Metadata, &t.TenantID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.AssigneeAgentID = assignee.String
	t.OwnerAgentID = owner.String
	t.AssigneeUserID = assigneeUser.String
	t.CreatedByAgentID = createdBy.String
	t.ParentID = parent.String
	t.BlockedBy = decodeTaskIDs(blockedBy.String)
	t.FollowupAt = followupAt.String
	t.FollowupMessage = followupMsg.String
	t.FollowupChannel = followupCh.String
	t.FollowupChatID = followupChat.String
	if t.Subject == "" {
		t.Subject = t.Title
	}
	if t.Title == "" {
		t.Title = t.Subject
	}
	return &t, nil
}

func (s *SQLiteStore) UpdateTeamTask(t TeamTask) (*TeamTask, error) {
	cur, err := s.GetTeamTask(t.ID)
	if err != nil {
		return nil, err
	}
	if title := strings.TrimSpace(firstNonEmptyStr(t.Title, t.Subject)); title != "" {
		cur.Title = title
		cur.Subject = title
	}
	if st := strings.ToLower(strings.TrimSpace(t.Status)); st != "" {
		v, ok := NormalizeTeamTaskStatus(st)
		if !ok {
			return nil, errors.New("unknown task status")
		}
		cur.Status = v
	}
	if t.AssigneeAgentID != "" {
		if _, err := s.GetAgent(t.AssigneeAgentID); err != nil {
			return nil, errors.New("assignee not found")
		}
		cur.AssigneeAgentID = t.AssigneeAgentID
	}
	if t.Description != "" {
		cur.Description = t.Description
	}
	if t.Result != "" {
		cur.Result = t.Result
	}
	if t.Priority != 0 {
		cur.Priority = t.Priority
	}
	if t.ProgressPercent != 0 {
		cur.ProgressPercent = t.ProgressPercent
	}
	if t.ProgressStep != "" {
		cur.ProgressStep = t.ProgressStep
	}
	if t.Channel != "" {
		cur.Channel = t.Channel
	}
	if t.ChatID != "" {
		cur.ChatID = t.ChatID
	}
	cur.UpdatedAt = formatTime(time.Now().UTC())
	_, err = s.db.Exec(`UPDATE team_tasks SET title=?, subject=?, description=?, status=?, priority=?,
		assignee_agent_id=?, owner_agent_id=?, assignee_user_id=?, created_by_agent_id=?, parent_id=?,
		user_id=?, channel=?, chat_id=?, task_type=?, task_number=?, identifier=?, result=?, blocked_by=?,
		progress_percent=?, progress_step=?, followup_at=?, followup_count=?, followup_max=?,
		followup_message=?, followup_channel=?, followup_chat_id=?, metadata=?, tenant_id=?, updated_at=?
		WHERE id=?`,
		cur.Title, cur.Subject, cur.Description, cur.Status, cur.Priority, cur.AssigneeAgentID,
		cur.OwnerAgentID, cur.AssigneeUserID, cur.CreatedByAgentID, cur.ParentID, cur.UserID,
		cur.Channel, cur.ChatID, cur.TaskType, cur.TaskNumber, cur.Identifier, cur.Result,
		encodeTaskIDs(cur.BlockedBy), cur.ProgressPercent, cur.ProgressStep, cur.FollowupAt,
		cur.FollowupCount, cur.FollowupMax, cur.FollowupMessage, cur.FollowupChannel,
		cur.FollowupChatID, cur.Metadata, cur.TenantID, cur.UpdatedAt, cur.ID)
	if err != nil {
		return nil, err
	}
	return s.GetTeamTask(cur.ID)
}

func (s *SQLiteStore) DeleteTeamTask(id string) error {
	res, err := s.db.Exec(`DELETE FROM team_tasks WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM team_task_comments WHERE task_id=?`, strings.TrimSpace(id))
	_, _ = s.db.Exec(`DELETE FROM team_task_attachments WHERE task_id=?`, strings.TrimSpace(id))
	return nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *SQLiteStore) CreateTeamMessage(m TeamMessage) (*TeamMessage, error) {
	m.TeamID = strings.TrimSpace(m.TeamID)
	m.FromAgentID = strings.TrimSpace(m.FromAgentID)
	m.Body = strings.TrimSpace(m.Body)
	if m.TeamID == "" || m.Body == "" {
		return nil, errors.New("team_id and body are required")
	}
	if _, err := s.GetTeam(m.TeamID); err != nil {
		return nil, errors.New("team not found")
	}
	if m.FromAgentID != "" {
		if _, err := s.GetAgent(m.FromAgentID); err != nil {
			return nil, errors.New("from agent not found")
		}
	}
	m.ID = newID()
	m.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO team_messages(id, team_id, from_agent_id, body, created_at) VALUES(?,?,?,?,?)`,
		m.ID, m.TeamID, m.FromAgentID, m.Body, formatTime(m.CreatedAt))
	if err != nil {
		return nil, err
	}
	cp := m
	return &cp, nil
}

func (s *SQLiteStore) ListTeamMessages(teamID string) ([]*TeamMessage, error) {
	if _, err := s.GetTeam(teamID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, team_id, from_agent_id, body, created_at FROM team_messages WHERE team_id=? ORDER BY created_at`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TeamMessage{}
	for rows.Next() {
		var m TeamMessage
		var ts string
		var from sql.NullString
		if err := rows.Scan(&m.ID, &m.TeamID, &from, &m.Body, &ts); err != nil {
			continue
		}
		m.FromAgentID = from.String
		m.CreatedAt = parseTime(ts)
		cp := m
		out = append(out, &cp)
	}
	return out, nil
}

func (s *SQLiteStore) AddAgentLink(fromID, toID string) error {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" {
		return errors.New("from and to agent ids are required")
	}
	from, err := s.GetAgent(fromID)
	if err != nil {
		return errors.New("from agent not found")
	}
	to, err := s.GetAgent(toID)
	if err != nil {
		return errors.New("to agent not found")
	}
	if !SameTenant(from.TenantID, to.TenantID) {
		return ErrCrossTenantLink
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO agent_links(from_agent_id, to_agent_id, tenant_id) VALUES(?,?,?)`, fromID, toID, NormalizeTenant(from.TenantID))
	return err
}

func (s *SQLiteStore) ListAgentLinks(fromID string) ([]AgentLink, error) {
	if _, err := s.GetAgent(fromID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT from_agent_id, to_agent_id FROM agent_links WHERE from_agent_id=?`, fromID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentLink{}
	for rows.Next() {
		var l AgentLink
		if err := rows.Scan(&l.FromAgentID, &l.ToAgentID); err != nil {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (s *SQLiteStore) HasAgentLink(fromID, toID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM agent_links WHERE from_agent_id=? AND to_agent_id=?`, fromID, toID).Scan(&n)
	return n > 0
}

func (s *SQLiteStore) RemoveAgentLink(fromID, toID string) error {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" {
		return errors.New("from and to agent ids are required")
	}
	if _, err := s.GetAgent(fromID); err != nil {
		return ErrNotFound
	}
	res, err := s.db.Exec(`DELETE FROM agent_links WHERE from_agent_id=? AND to_agent_id=?`, fromID, toID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) loadMetrics(agentID string) AgentMetrics {
	m := AgentMetrics{AgentID: agentID, ToolUses: map[string]int{}}
	var uses, adv sql.NullString
	err := s.db.QueryRow(`SELECT chat_runs, tool_errors, tool_uses_json, advertised_json FROM agent_metrics WHERE agent_id=?`, agentID).
		Scan(&m.ChatRuns, &m.ToolErrors, &uses, &adv)
	if err != nil {
		return m
	}
	if uses.String != "" {
		_ = json.Unmarshal([]byte(uses.String), &m.ToolUses)
	}
	if m.ToolUses == nil {
		m.ToolUses = map[string]int{}
	}
	if adv.String != "" {
		_ = json.Unmarshal([]byte(adv.String), &m.Advertised)
	}
	return m
}

func (s *SQLiteStore) saveMetrics(m AgentMetrics) {
	if m.ToolUses == nil {
		m.ToolUses = map[string]int{}
	}
	uses, _ := json.Marshal(m.ToolUses)
	adv, _ := json.Marshal(m.Advertised)
	_, _ = s.db.Exec(`INSERT INTO agent_metrics(agent_id, chat_runs, tool_errors, tool_uses_json, advertised_json)
		VALUES(?,?,?,?,?)
		ON CONFLICT(agent_id) DO UPDATE SET chat_runs=excluded.chat_runs, tool_errors=excluded.tool_errors,
			tool_uses_json=excluded.tool_uses_json, advertised_json=excluded.advertised_json`,
		m.AgentID, m.ChatRuns, m.ToolErrors, string(uses), string(adv))
}

func (s *SQLiteStore) RecordChatRun(agentID string) {
	if agentID == "" {
		return
	}
	m := s.loadMetrics(agentID)
	m.ChatRuns++
	s.saveMetrics(m)
}

func (s *SQLiteStore) RecordToolUse(agentID, tool string, failed bool) {
	if agentID == "" {
		return
	}
	m := s.loadMetrics(agentID)
	if tool == "" {
		tool = "unknown"
	}
	m.ToolUses[tool]++
	if failed {
		m.ToolErrors++
	}
	s.saveMetrics(m)
}

func (s *SQLiteStore) RecordAdvertisedTools(agentID string, names []string) {
	if agentID == "" {
		return
	}
	m := s.loadMetrics(agentID)
	seen := map[string]struct{}{}
	for _, n := range m.Advertised {
		seen[n] = struct{}{}
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		m.Advertised = append(m.Advertised, n)
	}
	s.saveMetrics(m)
}

func (s *SQLiteStore) GetAgentMetrics(agentID string) AgentMetrics {
	return s.loadMetrics(agentID)
}

func (s *SQLiteStore) MarkEvolutionApplied(agentID, suggestionID string) error {
	if agentID == "" || suggestionID == "" {
		return errors.New("agent_id and suggestion_id are required")
	}
	if _, err := s.GetAgent(agentID); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO evolution_applies(agent_id, suggestion_id) VALUES(?,?)`, agentID, suggestionID)
	return err
}

func (s *SQLiteStore) EvolutionApplied(agentID, suggestionID string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM evolution_applies WHERE agent_id=? AND suggestion_id=?`, agentID, suggestionID).Scan(&n)
	return n > 0
}

func (s *SQLiteStore) GetEvolutionGuardrails(agentID string) EvolutionGuardrails {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM evolution_guardrails WHERE agent_id=?`, agentID).Scan(&payload)
	if err != nil || payload == "" {
		return NormalizeGuardrails(EvolutionGuardrails{})
	}
	var g EvolutionGuardrails
	if json.Unmarshal([]byte(payload), &g) != nil {
		return NormalizeGuardrails(EvolutionGuardrails{})
	}
	return NormalizeGuardrails(g)
}

func (s *SQLiteStore) PutEvolutionGuardrails(agentID string, g EvolutionGuardrails) error {
	if agentID == "" {
		return errors.New("agent_id is required")
	}
	if _, err := s.GetAgent(agentID); err != nil {
		return err
	}
	g = NormalizeGuardrails(g)
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO evolution_guardrails(agent_id, payload) VALUES(?,?)
		ON CONFLICT(agent_id) DO UPDATE SET payload=excluded.payload`, agentID, string(raw))
	return err
}
