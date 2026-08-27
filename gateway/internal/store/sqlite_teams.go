// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (s *SQLiteStore) CreateTeam(t Team) (*Team, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return nil, errors.New("name is required")
	}
	if LiteEnabled() {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM teams`).Scan(&n); err != nil {
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
	_, err := s.db.Exec(`INSERT INTO teams(id, name, lead_agent_id, created_at) VALUES(?,?,?,?)`,
		t.ID, t.Name, t.LeadAgentID, formatTime(t.CreatedAt))
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
	rows, err := s.db.Query(`SELECT id, name, lead_agent_id, created_at FROM teams ORDER BY created_at`)
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
	row := s.db.QueryRow(`SELECT id, name, lead_agent_id, created_at FROM teams WHERE id=?`, id)
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
	var lead sql.NullString
	if err := sc.Scan(&t.ID, &t.Name, &lead, &ts); err != nil {
		return nil, err
	}
	t.LeadAgentID = lead.String
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

func (s *SQLiteStore) CreateTeamTask(t TeamTask) (*TeamTask, error) {
	t.Title = strings.TrimSpace(t.Title)
	t.TeamID = strings.TrimSpace(t.TeamID)
	t.Status = strings.ToLower(strings.TrimSpace(t.Status))
	if t.Title == "" || t.TeamID == "" {
		return nil, errors.New("team_id and title are required")
	}
	if t.Status == "" {
		t.Status = "todo"
	}
	if !validTaskStatus(t.Status) {
		return nil, errors.New("status must be todo, doing, or done")
	}
	if _, err := s.GetTeam(t.TeamID); err != nil {
		return nil, errors.New("team not found")
	}
	if t.AssigneeAgentID != "" {
		if _, err := s.GetAgent(t.AssigneeAgentID); err != nil {
			return nil, errors.New("assignee not found")
		}
	}
	t.ID = newID()
	_, err := s.db.Exec(`INSERT INTO team_tasks(id, team_id, title, status, assignee_agent_id) VALUES(?,?,?,?,?)`,
		t.ID, t.TeamID, t.Title, t.Status, t.AssigneeAgentID)
	if err != nil {
		return nil, err
	}
	cp := t
	return &cp, nil
}

func (s *SQLiteStore) ListTeamTasks(teamID, status string) ([]*TeamTask, error) {
	if _, err := s.GetTeam(teamID); err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(`SELECT id, team_id, title, status, assignee_agent_id FROM team_tasks WHERE team_id=?`, teamID)
	} else {
		rows, err = s.db.Query(`SELECT id, team_id, title, status, assignee_agent_id FROM team_tasks WHERE team_id=? AND status=?`, teamID, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TeamTask{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *SQLiteStore) GetTeamTask(id string) (*TeamTask, error) {
	row := s.db.QueryRow(`SELECT id, team_id, title, status, assignee_agent_id FROM team_tasks WHERE id=?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func scanTask(sc scanner) (*TeamTask, error) {
	var t TeamTask
	var assignee sql.NullString
	if err := sc.Scan(&t.ID, &t.TeamID, &t.Title, &t.Status, &assignee); err != nil {
		return nil, err
	}
	t.AssigneeAgentID = assignee.String
	return &t, nil
}

func (s *SQLiteStore) UpdateTeamTask(t TeamTask) (*TeamTask, error) {
	cur, err := s.GetTeamTask(t.ID)
	if err != nil {
		return nil, err
	}
	if title := strings.TrimSpace(t.Title); title != "" {
		cur.Title = title
	}
	if st := strings.ToLower(strings.TrimSpace(t.Status)); st != "" {
		if !validTaskStatus(st) {
			return nil, errors.New("status must be todo, doing, or done")
		}
		cur.Status = st
	}
	if t.AssigneeAgentID != "" {
		if _, err := s.GetAgent(t.AssigneeAgentID); err != nil {
			return nil, errors.New("assignee not found")
		}
		cur.AssigneeAgentID = t.AssigneeAgentID
	}
	_, err = s.db.Exec(`UPDATE team_tasks SET title=?, status=?, assignee_agent_id=? WHERE id=?`,
		cur.Title, cur.Status, cur.AssigneeAgentID, cur.ID)
	if err != nil {
		return nil, err
	}
	return cur, nil
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
	if _, err := s.GetAgent(fromID); err != nil {
		return errors.New("from agent not found")
	}
	if _, err := s.GetAgent(toID); err != nil {
		return errors.New("to agent not found")
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO agent_links(from_agent_id, to_agent_id) VALUES(?,?)`, fromID, toID)
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
