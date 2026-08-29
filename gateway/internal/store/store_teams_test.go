// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
)

func TestStore_TeamsKanbanMailboxAndLinks(t *testing.T) {
	s := New()
	lead, _ := s.CreateAgent(Agent{AgentKey: "lead", DisplayName: "Lead"})
	mem, _ := s.CreateAgent(Agent{AgentKey: "mem", DisplayName: "Mem"})
	team, err := s.CreateTeam(Team{Name: "Ops", LeadAgentID: lead.ID})
	if err != nil || team.ID == "" {
		t.Fatalf("CreateTeam: %v %#v", err, team)
	}
	members, err := s.ListTeamMembers(team.ID)
	if err != nil || len(members) != 1 || members[0].Role != "lead" {
		t.Fatalf("lead member %v %#v", err, members)
	}
	if _, err := s.AddTeamMember(TeamMember{TeamID: team.ID, AgentID: mem.ID, Role: "member"}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	got, err := s.TeamOfAgent(mem.ID)
	if err != nil || got.ID != team.ID {
		t.Fatalf("TeamOfAgent %v %#v", err, got)
	}
	todo, err := s.CreateTeamTask(TeamTask{TeamID: team.ID, Title: "Ship", Status: "todo", AssigneeAgentID: mem.ID})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	doing, err := s.UpdateTeamTask(TeamTask{ID: todo.ID, Status: "doing"})
	if err != nil || doing.Status != "doing" {
		t.Fatalf("kanban move %v %#v", err, doing)
	}
	onlyTodo, _ := s.ListTeamTasks(team.ID, "todo")
	if len(onlyTodo) != 0 {
		t.Fatalf("filter todo %#v", onlyTodo)
	}
	doingList, _ := s.ListTeamTasks(team.ID, "doing")
	if len(doingList) != 1 {
		t.Fatalf("filter doing %#v", doingList)
	}
	msg, err := s.CreateTeamMessage(TeamMessage{TeamID: team.ID, FromAgentID: lead.ID, Body: "hello mailbox"})
	if err != nil || msg.ID == "" {
		t.Fatalf("mailbox %v %#v", err, msg)
	}
	msgs, _ := s.ListTeamMessages(team.ID)
	if len(msgs) != 1 || msgs[0].Body != "hello mailbox" {
		t.Fatalf("list mailbox %#v", msgs)
	}
	if err := s.AddAgentLink(lead.ID, mem.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := s.AddAgentLink(mem.ID, lead.ID); err != nil {
		t.Fatalf("bidir: %v", err)
	}
	if !s.HasAgentLink(lead.ID, mem.ID) || !s.HasAgentLink(mem.ID, lead.ID) {
		t.Fatal("expected bidirectional links")
	}
	if err := s.RemoveAgentLink(lead.ID, mem.ID); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if s.HasAgentLink(lead.ID, mem.ID) || !s.HasAgentLink(mem.ID, lead.ID) {
		t.Fatal("directed unlink should drop only from→to")
	}
	if err := s.RemoveAgentLink(mem.ID, lead.ID); err != nil {
		t.Fatalf("unlink reverse: %v", err)
	}
	if err := s.RemoveAgentLink(mem.ID, lead.ID); err != ErrNotFound {
		t.Fatalf("missing link want ErrNotFound got %v", err)
	}
	s.RecordChatRun(lead.ID)
	s.RecordToolUse(lead.ID, "delegate", true)
	m := s.GetAgentMetrics(lead.ID)
	if m.ChatRuns != 1 || m.ToolErrors != 1 || m.ToolUses["delegate"] != 1 {
		t.Fatalf("metrics %#v", m)
	}
}

func TestStore_LiteCaps(t *testing.T) {
	t.Setenv("GOSO_LITE", "1")
	s := New()
	for i := 0; i < LiteMaxAgents; i++ {
		if _, err := s.CreateAgent(Agent{AgentKey: "a" + itoa(int64(i+1))}); err != nil {
			t.Fatalf("agent %d: %v", i, err)
		}
	}
	if _, err := s.CreateAgent(Agent{AgentKey: "overflow"}); err != ErrLiteCap {
		t.Fatalf("6th agent want ErrLiteCap got %v", err)
	}
	lead := s.ListAgents()[0]
	if _, err := s.CreateTeam(Team{Name: "one", LeadAgentID: lead.ID}); err != nil {
		t.Fatalf("first team: %v", err)
	}
	if _, err := s.CreateTeam(Team{Name: "two", LeadAgentID: lead.ID}); err != ErrLiteCap {
		t.Fatalf("2nd team want ErrLiteCap got %v", err)
	}
}

func TestStore_LiteOffNoCap(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	s := New()
	for i := 0; i < 6; i++ {
		if _, err := s.CreateAgent(Agent{AgentKey: "n" + itoa(int64(i+1))}); err != nil {
			t.Fatalf("agent %d: %v", i, err)
		}
	}
	lead := s.ListAgents()[0]
	if _, err := s.CreateTeam(Team{Name: "t1", LeadAgentID: lead.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTeam(Team{Name: "t2", LeadAgentID: lead.ID}); err != nil {
		t.Fatalf("second team without lite: %v", err)
	}
}

func TestSQLiteStore_TeamsAndLite(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "teams.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := s.CreateAgent(Agent{AgentKey: "k", DisplayName: "K", Instructions: "be brief"})
	team, err := s.CreateTeam(Team{Name: "SQ", LeadAgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateTeamTask(TeamTask{TeamID: team.ID, Title: "T"})
	_, _ = s.CreateTeamMessage(TeamMessage{TeamID: team.ID, FromAgentID: a.ID, Body: "m"})
	_ = s.AddAgentLink(a.ID, a.ID)
	if err := s.RemoveAgentLink(a.ID, a.ID); err != nil {
		t.Fatalf("sqlite unlink: %v", err)
	}
	if s.HasAgentLink(a.ID, a.ID) {
		t.Fatal("sqlite unlink leftover")
	}
	s.RecordChatRun(a.ID)
	_ = s.Close()

	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetAgent(a.ID)
	if err != nil || got.Instructions != "be brief" {
		t.Fatalf("agent persist %v %#v", err, got)
	}
	if len(s2.ListTeams()) != 1 {
		t.Fatalf("teams persist")
	}
	tasks, _ := s2.ListTeamTasks(team.ID, "")
	if len(tasks) != 1 {
		t.Fatalf("tasks persist %#v", tasks)
	}
	if s2.GetAgentMetrics(a.ID).ChatRuns != 1 {
		t.Fatalf("metrics persist")
	}
}

func TestStore_EvolutionGuardrailsDefaultAndPersist(t *testing.T) {
	s := New()
	a, err := s.CreateAgent(Agent{AgentKey: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := s.GetEvolutionGuardrails(a.ID)
	if g.AutoAdapt || g.MinRuns != DefaultMinRuns {
		t.Fatalf("defaults %#v", g)
	}
	for _, need := range []string{"display_name", "agent_key"} {
		found := false
		for _, k := range g.Locked {
			if k == need {
				found = true
			}
		}
		if !found {
			t.Fatalf("locked missing %s %#v", need, g.Locked)
		}
	}
	if err := s.PutEvolutionGuardrails(a.ID, EvolutionGuardrails{AutoAdapt: true, MinRuns: 7, Locked: nil}); err != nil {
		t.Fatal(err)
	}
	g = s.GetEvolutionGuardrails(a.ID)
	if !g.AutoAdapt || g.MinRuns != 7 {
		t.Fatalf("put %#v", g)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "guard.db")
	sql, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := sql.CreateAgent(Agent{AgentKey: "sg"})
	if err := sql.PutEvolutionGuardrails(b.ID, EvolutionGuardrails{AutoAdapt: true, MinRuns: 9, Locked: []string{}}); err != nil {
		t.Fatal(err)
	}
	_ = sql.Close()
	sql2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sql2.Close()
	got := sql2.GetEvolutionGuardrails(b.ID)
	if !got.AutoAdapt || got.MinRuns != 9 {
		t.Fatalf("sqlite persist %#v", got)
	}
	foundName := false
	for _, k := range got.Locked {
		if k == "display_name" {
			foundName = true
		}
	}
	if !foundName {
		t.Fatalf("sqlite locked %#v", got.Locked)
	}
}

func TestUpdateAgent_DoesNotRename(t *testing.T) {
	s := New()
	a, _ := s.CreateAgent(Agent{AgentKey: "keep", DisplayName: "Keep"})
	_, err := s.UpdateAgent(Agent{ID: a.ID, Instructions: "prefix", OrchestrationMode: "auto", Enabled: a.Enabled})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetAgent(a.ID)
	if got.AgentKey != "keep" || got.DisplayName != "Keep" {
		t.Fatalf("identity changed %#v", got)
	}
	if got.Instructions != "prefix" || got.OrchestrationMode != "auto" {
		t.Fatalf("prefix/mode %#v", got)
	}
}
