// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"strings"
	"time"
)

func validTaskStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "todo", "doing", "done":
		return true
	default:
		return false
	}
}

func validMemberRole(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "lead", "member":
		return true
	default:
		return false
	}
}

func (s *Store) CreateTeam(t Team) (*Team, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return nil, errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.TenantID = NormalizeTenant(t.TenantID)
	if LiteEnabled() {
		n := 0
		for _, v := range s.teams {
			if SameTenant(v.TenantID, t.TenantID) {
				n++
			}
		}
		if n >= LiteMaxTeams {
			return nil, ErrLiteCap
		}
	}
	if t.LeadAgentID != "" {
		if _, ok := s.agents[t.LeadAgentID]; !ok {
			return nil, errors.New("lead agent not found")
		}
	}
	t.ID = s.nextID()
	t.CreatedAt = time.Now().UTC()
	cp := t
	s.teams[cp.ID] = &cp
	if t.LeadAgentID != "" {
		s.teamMembers[cp.ID] = append(s.teamMembers[cp.ID], &TeamMember{
			TeamID: cp.ID, AgentID: t.LeadAgentID, Role: "lead",
		})
	}
	return &cp, nil
}

func (s *Store) ListTeams() []*Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Team, 0, len(s.teams))
	for _, v := range s.teams {
		cp := *v
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetTeam(id string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.teams[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (s *Store) UpdateTeam(t Team) (*Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.teams[t.ID]
	if !ok {
		return nil, ErrNotFound
	}
	if name := strings.TrimSpace(t.Name); name != "" {
		cur.Name = name
	}
	if t.LeadAgentID != "" {
		if _, ok := s.agents[t.LeadAgentID]; !ok {
			return nil, errors.New("lead agent not found")
		}
		cur.LeadAgentID = t.LeadAgentID
	}
	cp := *cur
	return &cp, nil
}

func (s *Store) DeleteTeam(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[id]; !ok {
		return ErrNotFound
	}
	delete(s.teams, id)
	delete(s.teamMembers, id)
	delete(s.teamMsgs, id)
	for tid, task := range s.teamTasks {
		if task != nil && task.TeamID == id {
			delete(s.teamTasks, tid)
		}
	}
	return nil
}

func (s *Store) AddTeamMember(m TeamMember) (*TeamMember, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[m.TeamID]; !ok {
		return nil, errors.New("team not found")
	}
	if _, ok := s.agents[m.AgentID]; !ok {
		return nil, errors.New("agent not found")
	}
	for _, ex := range s.teamMembers[m.TeamID] {
		if ex.AgentID == m.AgentID {
			ex.Role = m.Role
			cp := *ex
			if m.Role == "lead" {
				s.teams[m.TeamID].LeadAgentID = m.AgentID
			}
			return &cp, nil
		}
	}
	cp := m
	s.teamMembers[m.TeamID] = append(s.teamMembers[m.TeamID], &cp)
	if m.Role == "lead" {
		s.teams[m.TeamID].LeadAgentID = m.AgentID
	}
	return &cp, nil
}

func (s *Store) ListTeamMembers(teamID string) ([]*TeamMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.teams[teamID]; !ok {
		return nil, ErrNotFound
	}
	list := s.teamMembers[teamID]
	out := make([]*TeamMember, 0, len(list))
	for _, m := range list {
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}

func (s *Store) RemoveTeamMember(teamID, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[teamID]; !ok {
		return ErrNotFound
	}
	list := s.teamMembers[teamID]
	kept := list[:0]
	found := false
	for _, m := range list {
		if m.AgentID == agentID {
			found = true
			continue
		}
		kept = append(kept, m)
	}
	if !found {
		return ErrNotFound
	}
	s.teamMembers[teamID] = kept
	return nil
}

func (s *Store) TeamOfAgent(agentID string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for teamID, members := range s.teamMembers {
		for _, m := range members {
			if m != nil && m.AgentID == agentID {
				t, ok := s.teams[teamID]
				if !ok {
					return nil, ErrNotFound
				}
				cp := *t
				return &cp, nil
			}
		}
	}
	return nil, ErrNotFound
}

func (s *Store) CreateTeamTask(t TeamTask) (*TeamTask, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[t.TeamID]; !ok {
		return nil, errors.New("team not found")
	}
	if t.AssigneeAgentID != "" {
		if _, ok := s.agents[t.AssigneeAgentID]; !ok {
			return nil, errors.New("assignee not found")
		}
	}
	t.ID = s.nextID()
	cp := t
	s.teamTasks[cp.ID] = &cp
	return &cp, nil
}

func (s *Store) ListTeamTasks(teamID, status string) ([]*TeamTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.teams[teamID]; !ok {
		return nil, ErrNotFound
	}
	status = strings.ToLower(strings.TrimSpace(status))
	out := []*TeamTask{}
	for _, t := range s.teamTasks {
		if t == nil || t.TeamID != teamID {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	return out, nil
}

func (s *Store) GetTeamTask(id string) (*TeamTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.teamTasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (s *Store) UpdateTeamTask(t TeamTask) (*TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.teamTasks[t.ID]
	if !ok {
		return nil, ErrNotFound
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
		if _, ok := s.agents[t.AssigneeAgentID]; !ok {
			return nil, errors.New("assignee not found")
		}
		cur.AssigneeAgentID = t.AssigneeAgentID
	}
	cp := *cur
	return &cp, nil
}

func (s *Store) CreateTeamMessage(m TeamMessage) (*TeamMessage, error) {
	m.TeamID = strings.TrimSpace(m.TeamID)
	m.FromAgentID = strings.TrimSpace(m.FromAgentID)
	m.Body = strings.TrimSpace(m.Body)
	if m.TeamID == "" || m.Body == "" {
		return nil, errors.New("team_id and body are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[m.TeamID]; !ok {
		return nil, errors.New("team not found")
	}
	if m.FromAgentID != "" {
		if _, ok := s.agents[m.FromAgentID]; !ok {
			return nil, errors.New("from agent not found")
		}
	}
	m.ID = s.nextID()
	m.CreatedAt = time.Now().UTC()
	cp := m
	s.teamMsgs[m.TeamID] = append(s.teamMsgs[m.TeamID], &cp)
	return &cp, nil
}

func (s *Store) ListTeamMessages(teamID string) ([]*TeamMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.teams[teamID]; !ok {
		return nil, ErrNotFound
	}
	list := s.teamMsgs[teamID]
	out := make([]*TeamMessage, 0, len(list))
	for _, m := range list {
		cp := *m
		out = append(out, &cp)
	}
	return out, nil
}

func (s *Store) AddAgentLink(fromID, toID string) error {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" {
		return errors.New("from and to agent ids are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[fromID]; !ok {
		return errors.New("from agent not found")
	}
	if _, ok := s.agents[toID]; !ok {
		return errors.New("to agent not found")
	}
	for _, id := range s.agentLinks[fromID] {
		if id == toID {
			return nil
		}
	}
	s.agentLinks[fromID] = append(s.agentLinks[fromID], toID)
	return nil
}

func (s *Store) ListAgentLinks(fromID string) ([]AgentLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.agents[fromID]; !ok {
		return nil, ErrNotFound
	}
	tos := s.agentLinks[fromID]
	out := make([]AgentLink, 0, len(tos))
	for _, to := range tos {
		out = append(out, AgentLink{FromAgentID: fromID, ToAgentID: to})
	}
	return out, nil
}

func (s *Store) HasAgentLink(fromID, toID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range s.agentLinks[fromID] {
		if id == toID {
			return true
		}
	}
	return false
}

func (s *Store) RemoveAgentLink(fromID, toID string) error {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" {
		return errors.New("from and to agent ids are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[fromID]; !ok {
		return ErrNotFound
	}
	tos := s.agentLinks[fromID]
	kept := tos[:0]
	found := false
	for _, id := range tos {
		if id == toID {
			found = true
			continue
		}
		kept = append(kept, id)
	}
	if !found {
		return ErrNotFound
	}
	s.agentLinks[fromID] = kept
	return nil
}

func (s *Store) metricLocked(agentID string) *AgentMetrics {
	m, ok := s.metrics[agentID]
	if !ok {
		m = &AgentMetrics{AgentID: agentID, ToolUses: map[string]int{}}
		s.metrics[agentID] = m
	}
	if m.ToolUses == nil {
		m.ToolUses = map[string]int{}
	}
	return m
}

func (s *Store) RecordChatRun(agentID string) {
	if agentID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricLocked(agentID).ChatRuns++
}

func (s *Store) RecordToolUse(agentID, tool string, failed bool) {
	if agentID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.metricLocked(agentID)
	if tool == "" {
		tool = "unknown"
	}
	m.ToolUses[tool]++
	if failed {
		m.ToolErrors++
	}
}

func (s *Store) RecordAdvertisedTools(agentID string, names []string) {
	if agentID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.metricLocked(agentID)
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
}

func (s *Store) GetAgentMetrics(agentID string) AgentMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.metrics[agentID]
	if !ok {
		return AgentMetrics{AgentID: agentID, ToolUses: map[string]int{}}
	}
	cp := *m
	cp.ToolUses = map[string]int{}
	for k, v := range m.ToolUses {
		cp.ToolUses[k] = v
	}
	if m.Advertised != nil {
		cp.Advertised = append([]string{}, m.Advertised...)
	}
	return cp
}

func (s *Store) MarkEvolutionApplied(agentID, suggestionID string) error {
	if agentID == "" || suggestionID == "" {
		return errors.New("agent_id and suggestion_id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agentID]; !ok {
		return ErrNotFound
	}
	if s.evoApplied[agentID] == nil {
		s.evoApplied[agentID] = map[string]bool{}
	}
	s.evoApplied[agentID][suggestionID] = true
	return nil
}

func (s *Store) EvolutionApplied(agentID, suggestionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.evoApplied[agentID][suggestionID]
}

func (s *Store) GetEvolutionGuardrails(agentID string) EvolutionGuardrails {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.evoGuard[agentID]
	if !ok {
		return NormalizeGuardrails(EvolutionGuardrails{})
	}
	return NormalizeGuardrails(g)
}

func (s *Store) PutEvolutionGuardrails(agentID string, g EvolutionGuardrails) error {
	if agentID == "" {
		return errors.New("agent_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agentID]; !ok {
		return ErrNotFound
	}
	s.evoGuard[agentID] = NormalizeGuardrails(g)
	return nil
}
