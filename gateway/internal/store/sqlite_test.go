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
	if _, err := s.PutMemory(Memory{SessionID: sess.ID, Body: "note"}); err != nil {
		t.Fatalf("PutMemory: %v", err)
	}
	if err := s.DeleteSession(sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.GetSession(sess.ID); err != ErrNotFound {
		t.Fatalf("GetSession after delete: %v", err)
	}
	if _, err := s.ListMessages(sess.ID); err != ErrNotFound {
		t.Fatalf("ListMessages after delete: %v", err)
	}
	if _, err := s.ListMemories(sess.ID); err != ErrNotFound {
		t.Fatalf("ListMemories after delete: %v", err)
	}
	if err := s.DeleteSession("missing"); err != ErrNotFound {
		t.Fatalf("DeleteSession missing: %v", err)
	}
	_ = os.Getenv // keep import
}

func TestSQLiteStore_AgentLLMProviderPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llmprov.db")
	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s1.CreateAgent(Agent{AgentKey: "lp", DisplayName: "LP", LLMProvider: "p-a", Model: "m-a"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetAgent(a.ID)
	if err != nil || got.LLMProvider != "p-a" || got.Model != "m-a" {
		t.Fatalf("persist %#v %v", got, err)
	}
	_, err = s2.UpdateAgent(Agent{ID: a.ID, Instructions: got.Instructions, Model: got.Model, LLMProvider: "p-b", Enabled: got.Enabled})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = s2.GetAgent(a.ID)
	if got.LLMProvider != "p-b" {
		t.Fatalf("update %#v", got)
	}
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

func TestSQLiteStore_AgentLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-life.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateAgent(Agent{AgentKey: "life", DisplayName: "Life", Instructions: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.Enabled {
		t.Fatal("create should be enabled")
	}
	sess, err := s.CreateSession(Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessage(Message{SessionID: sess.ID, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	off, err := s.UpdateAgent(Agent{ID: a.ID, Instructions: a.Instructions, Model: a.Model, LLMProvider: a.LLMProvider, Enabled: false, UpdatedAt: a.Stamp()})
	if err != nil || off.Enabled {
		t.Fatalf("disable %#v %v", off, err)
	}
	if _, err := s.UpdateAgent(Agent{ID: a.ID, Instructions: "x", Enabled: true, UpdatedAt: a.Stamp()}); err != ErrConflict {
		t.Fatalf("stale want conflict %v", err)
	}
	_ = s.Close()
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetAgent(a.ID)
	if err != nil || got.Enabled || got.Instructions != "keep" {
		t.Fatalf("persist %#v %v", got, err)
	}
	if err := s2.DeleteAgent(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.GetAgent(a.ID); err != ErrNotFound {
		t.Fatalf("deleted %v", err)
	}
	if _, err := s2.GetSession(sess.ID); err != ErrNotFound {
		t.Fatalf("session leftover %v", err)
	}
}

func TestSQLiteStore_PromptModeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt-mode.db")
	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	a, err := s1.CreateAgent(Agent{AgentKey: "pm", DisplayName: "PM"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s1.CreateSession(Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sess.PromptMode != "" {
		t.Fatalf("default prompt_mode %q", sess.PromptMode)
	}
	got, err := s1.UpdateSession(Session{ID: sess.ID, PromptMode: "task"})
	if err != nil || got.PromptMode != "task" {
		t.Fatalf("update %v %+v", err, got)
	}
	_ = s1.Close()

	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	again, err := s2.GetSession(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.PromptMode != "task" {
		t.Fatalf("persisted prompt_mode %q", again.PromptMode)
	}
}

func TestSQLiteStore_RestartSafeIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ids.db")
	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	a, err := s1.CreateAgent(Agent{AgentKey: "k1", DisplayName: "K1"})
	if err != nil || a.ID == "" {
		t.Fatalf("CreateAgent: %v %+v", err, a)
	}
	sess, err := s1.CreateSession(Session{AgentID: a.ID, Label: "s1"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	m1, err := s1.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "first"})
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	_ = s1.Close()

	sqliteSeq = 0
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	a2, err := s2.CreateAgent(Agent{AgentKey: "k2", DisplayName: "K2"})
	if err != nil {
		t.Fatalf("second CreateAgent: %v", err)
	}
	sess2, err := s2.CreateSession(Session{AgentID: a.ID, Label: "s2"})
	if err != nil {
		t.Fatalf("second CreateSession: %v", err)
	}
	m2, err := s2.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "second"})
	if err != nil {
		t.Fatalf("second AddMessage: %v", err)
	}
	if a2.ID == a.ID || sess2.ID == sess.ID || m2.ID == m1.ID {
		t.Fatalf("collided ids a=%s/%s sess=%s/%s msg=%s/%s", a.ID, a2.ID, sess.ID, sess2.ID, m1.ID, m2.ID)
	}
	if len(s2.ListAgents()) != 2 {
		t.Fatalf("agents %d", len(s2.ListAgents()))
	}
	msgs, err := s2.ListMessages(sess.ID)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages %v %v", err, msgs)
	}
}

func TestSQLiteStore_ConnectorLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conn.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	a, _ := s.CreateAgent(Agent{AgentKey: "k", DisplayName: "K"})
	_, err = s.CreateConnector(ConnectorRecord{Name: "pos", Transport: "http", Endpoint: "http://x", Enabled: true})
	if err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	if err := s.LinkAgentConnector(a.ID, "pos"); err != nil {
		t.Fatalf("link: %v", err)
	}
	names, err := s.ListAgentConnectors(a.ID)
	if err != nil || len(names) != 1 {
		t.Fatalf("links %v %v", err, names)
	}
	if s.GetToolFlag("web_search") {
		t.Fatal("default off")
	}
	if err := s.SetToolFlag("web_search", true); err != nil {
		t.Fatal(err)
	}
	if !s.GetToolFlag("web_search") {
		t.Fatal("flag")
	}
	ep := "http://y"
	upd, err := s.UpdateConnector("pos", nil, &ep, nil)
	if err != nil || upd.Endpoint != ep {
		t.Fatalf("update %v %+v", err, upd)
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

func TestSQLiteStore_FTSAndSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fts.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	a, _ := s.CreateAgent(Agent{AgentKey: "k", DisplayName: "K"})
	sess, _ := s.CreateSession(Session{AgentID: a.ID})
	_, err = s.PutMemory(Memory{SessionID: sess.ID, Body: "episodic pineapple note"})
	if err != nil {
		t.Fatalf("PutMemory: %v", err)
	}
	_, _ = s.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "talk about mango"})
	hits, err := s.SearchMemory("pineapple")
	if err != nil || len(hits) == 0 {
		t.Fatalf("fts pineapple %v %v fts=%v", err, hits, s.fts)
	}
	hits, err = s.SearchMemory("mango")
	if err != nil || len(hits) == 0 {
		t.Fatalf("fts mango %v %v", err, hits)
	}
	none, err := s.SearchMemory("zzzz-absent")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("empty %v %v", err, none)
	}
	sum, err := s.SaveSummary(sess.ID, "alpha omega")
	if err != nil {
		t.Fatalf("SaveSummary: %v", err)
	}
	sum2, err := s.SaveSummary(sess.ID, "alpha omega 2")
	if err != nil || sum2.ID != sum.ID {
		t.Fatalf("upsert %v %v", err, sum2)
	}
	got, err := s.LatestSummary(sess.ID)
	if err != nil || got.Body != "alpha omega 2" {
		t.Fatalf("latest %v %v", err, got)
	}

	b, _ := s.CreateAgent(Agent{AgentKey: "k2", DisplayName: "K2"})
	sess2, _ := s.CreateSession(Session{AgentID: b.ID})
	dur, err := s.PutMemory(Memory{SessionID: sess.ID, Body: "durable charter", Kind: "document"})
	if err != nil || dur.Kind != KindDurable {
		t.Fatalf("durable alias %v %#v", err, dur)
	}
	_, _ = s.PutMemory(Memory{SessionID: sess2.ID, Body: "other agent note", Kind: KindDurable})
	byAgent, err := s.QueryMemories(MemoryQuery{AgentID: a.ID})
	if err != nil || len(byAgent) < 2 {
		t.Fatalf("query agent %v %d", err, len(byAgent))
	}
	byKind, err := s.QueryMemories(MemoryQuery{Kind: KindDurable, SessionID: sess.ID})
	if err != nil || len(byKind) != 1 || byKind[0].ID != dur.ID {
		t.Fatalf("query kind %v %#v", err, byKind)
	}
	upd, err := s.UpdateMemory(Memory{ID: dur.ID, Body: "durable charter v2", Kind: KindDurable})
	if err != nil || upd.Body != "durable charter v2" {
		t.Fatalf("update %v %#v", err, upd)
	}
	if err := s.DeleteMemory(dur.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetMemory(dur.ID); err != ErrNotFound {
		t.Fatalf("deleted get: %v", err)
	}
}

func TestSQLiteStore_VaultFTSAndLinks(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenSQLite(filepath.Join(dir, "vault.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	a, err := s.PutVaultDoc(VaultDoc{Title: "Alpha", Path: "alpha.md", SHA256: "aa", Body: "wikilink pineapple [[Beta]]"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := s.ReplaceVaultLinks(a.ID, []string{"Beta"}); err != nil {
		t.Fatalf("links: %v", err)
	}
	b, err := s.PutVaultDoc(VaultDoc{Title: "Beta", Path: "beta.md", SHA256: "bb", Body: "mango [[Alpha]]"})
	if err != nil {
		t.Fatalf("beta: %v", err)
	}
	if err := s.ReplaceVaultLinks(b.ID, []string{"Alpha"}); err != nil {
		t.Fatalf("beta links: %v", err)
	}
	_ = s.ReResolveVaultLinks()
	ob, ib, err := s.ListVaultDocLinks(a.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != b.ID || len(ib) != 1 {
		t.Fatalf("edges %v %#v %#v", err, ob, ib)
	}
	hits, err := s.SearchVault("pineapple")
	if err != nil || len(hits) == 0 {
		t.Fatalf("fts pineapple %v %#v vaultFTS=%v", err, hits, s.vaultFTS)
	}
	hits, err = s.SearchVault("mango")
	if err != nil || len(hits) == 0 {
		t.Fatalf("fts mango %v %#v", err, hits)
	}
	none, err := s.SearchVault("zzzz-absent")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("empty %v %#v", err, none)
	}
}
