// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import "testing"

func TestStore_AgentCRUD(t *testing.T) {
	s := New()
	a, err := s.CreateAgent(Agent{AgentKey: "alpha", DisplayName: "Alpha"})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected ID")
	}
	if _, err := s.CreateAgent(Agent{AgentKey: "alpha"}); err == nil {
		t.Fatal("expected duplicate error")
	}
	if _, err := s.CreateAgent(Agent{}); err == nil {
		t.Fatal("expected validation error")
	}
	list := s.ListAgents()
	if len(list) != 1 {
		t.Fatalf("ListAgents len=%d", len(list))
	}
	got, err := s.GetAgent(a.ID)
	if err != nil || got.AgentKey != "alpha" {
		t.Fatalf("GetAgent: %v %v", err, got)
	}
	if _, err := s.GetAgent("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_SessionAndMessage(t *testing.T) {
	s := New()
	a, _ := s.CreateAgent(Agent{AgentKey: "k1", DisplayName: "K1"})
	if _, err := s.CreateSession(Session{}); err == nil {
		t.Fatal("expected agent_id required")
	}
	if _, err := s.CreateSession(Session{AgentID: "nope"}); err == nil {
		t.Fatal("expected agent not found")
	}
	sess, err := s.CreateSession(Session{AgentID: a.ID, Label: "hello"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if len(s.ListSessions()) != 1 {
		t.Fatal("ListSessions len")
	}
	if _, err := s.GetSession("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound")
	}
	_ = sess
	msg, err := s.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "hi"})
	if err != nil || msg.ID == "" {
		t.Fatalf("AddMessage: %v %v", err, msg)
	}
	msgs, err := s.ListMessages(sess.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Fatalf("ListMessages: %v %v", err, msgs)
	}
	if _, err := s.ListMessages("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for messages")
	}
	if _, err := s.AddMessage(Message{SessionID: "nope", Content: "x"}); err == nil {
		t.Fatal("expected session not found")
	}
}

func TestStore_MemoryAndSearch(t *testing.T) {
	s := New()
	a, _ := s.CreateAgent(Agent{AgentKey: "mem", DisplayName: "M"})
	sess, _ := s.CreateSession(Session{AgentID: a.ID})
	_, err := s.PutMemory(Memory{SessionID: sess.ID, Body: "alpha needle omega"})
	if err != nil {
		t.Fatalf("PutMemory: %v", err)
	}
	list, err := s.ListMemories(sess.ID)
	if err != nil || len(list) != 1 || list[0].Kind != KindEpisodic {
		t.Fatalf("ListMemories %v %v", err, list)
	}
	hits, err := s.SearchMemory("needle")
	if err != nil || len(hits) != 1 || hits[0].Kind != KindEpisodic {
		t.Fatalf("search memory %v %v", err, hits)
	}
	_, _ = s.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "user mentioned zebra"})
	hits, err = s.SearchMemory("zebra")
	if err != nil || len(hits) != 1 || hits[0].Kind != KindMessage {
		t.Fatalf("search message %v %v", err, hits)
	}
	empty, err := s.SearchMemory("no-such-token")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty search %v %v", err, empty)
	}
	sum, err := s.SaveSummary(sess.ID, "first last")
	if err != nil || sum.Body != "first last" {
		t.Fatalf("SaveSummary %v %v", err, sum)
	}
	sum2, err := s.SaveSummary(sess.ID, "updated")
	if err != nil || sum2.ID != sum.ID || sum2.Body != "updated" {
		t.Fatalf("upsert %v %v", err, sum2)
	}
	got, err := s.LatestSummary(sess.ID)
	if err != nil || got.Body != "updated" {
		t.Fatalf("LatestSummary %v %v", err, got)
	}
}

func TestStore_ConnectorAndLink(t *testing.T) {
	s := New()
	a, _ := s.CreateAgent(Agent{AgentKey: "k", DisplayName: "K"})
	c, err := s.CreateConnector(ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://127.0.0.1:8089", Enabled: true})
	if err != nil || c.Name != "zalocrm" {
		t.Fatalf("CreateConnector: %v %v", err, c)
	}
	if _, err := s.CreateConnector(ConnectorRecord{Name: "zalocrm"}); err != ErrExists {
		t.Fatalf("dup: %v", err)
	}
	if err := s.LinkAgentConnector(a.ID, "zalocrm"); err != nil {
		t.Fatalf("link: %v", err)
	}
	names, err := s.ListAgentConnectors(a.ID)
	if err != nil || len(names) != 1 || names[0] != "zalocrm" {
		t.Fatalf("list links: %v %v", err, names)
	}
	if err := s.SetConnectorEnabled("zalocrm", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got, _ := s.GetConnector("zalocrm")
	if got.Enabled {
		t.Fatal("expected disabled")
	}
}
