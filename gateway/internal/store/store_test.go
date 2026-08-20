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
