// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer s.Close()

	a, err := s.CreateAgent(Agent{AgentKey: "k1", DisplayName: "K1"})
	if err != nil || a.ID == "" {
		t.Fatalf("CreateAgent: %v %v", err, a)
	}
	if _, err := s.CreateAgent(Agent{AgentKey: "k1"}); err != ErrExists {
		t.Fatalf("expected ErrExists, got %v", err)
	}
	list := s.ListAgents()
	if len(list) != 1 {
		t.Fatalf("ListAgents %d", len(list))
	}
	got, _ := s.GetAgent(a.ID)
	if got.AgentKey != "k1" {
		t.Fatalf("GetAgent %v", got)
	}
	if _, err := s.GetAgent("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound")
	}

	sess, err := s.CreateSession(Session{AgentID: a.ID, Label: "hello"})
	if err != nil || sess.ID == "" {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := s.CreateSession(Session{AgentID: "nope"}); err == nil {
		t.Fatal("expected agent not found")
	}
	msgs, _ := s.ListMessages(sess.ID)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 msgs, got %d", len(msgs))
	}
	m, _ := s.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "hi"})
	if m.ID == "" {
		t.Fatal("AddMessage no ID")
	}
	msgs2, err := s.ListMessages(sess.ID)
	if err != nil || len(msgs2) != 1 || msgs2[0].Content != "hi" {
		t.Fatalf("ListMessages %v %v", err, msgs2)
	}
	_ = os.Getenv // keep import
}

func TestSQLiteStore_PersistReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")
	s1, _ := OpenSQLite(path)
	a, _ := s1.CreateAgent(Agent{AgentKey: "persist", DisplayName: "P"})
	sess, _ := s1.CreateSession(Session{AgentID: a.ID})
	_, _ = s1.AddMessage(Message{SessionID: sess.ID, Content: "first"})
	_ = s1.Close()

	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if len(s2.ListAgents()) != 1 {
		t.Fatalf("agents not persisted")
	}
	if len(s2.ListSessions()) != 1 {
		t.Fatalf("sessions not persisted")
	}
	msgs, _ := s2.ListMessages(sess.ID)
	if len(msgs) != 1 || msgs[0].Content != "first" {
		t.Fatalf("messages not persisted %v", msgs)
	}
}

func TestSQLiteStore_Memory(t *testing.T) {
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite memory: %v", err)
	}
	defer s.Close()
	a, _ := s.CreateAgent(Agent{AgentKey: "mem", DisplayName: "Mem"})
	if a.ID == "" {
		t.Fatal("mem agent no ID")
	}
}
