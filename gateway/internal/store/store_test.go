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

func TestStore_AgentLLMProvider(t *testing.T) {
	s := New()
	a, err := s.CreateAgent(Agent{AgentKey: "p", DisplayName: "P", LLMProvider: "p-a", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAgent(a.ID)
	if err != nil || got.LLMProvider != "p-a" {
		t.Fatalf("create llm_provider %#v %v", got, err)
	}
	upd, err := s.UpdateAgent(Agent{ID: a.ID, Instructions: got.Instructions, OrchestrationMode: got.OrchestrationMode, Model: got.Model, LLMProvider: ""})
	if err != nil || upd.LLMProvider != "" {
		t.Fatalf("clear llm_provider %#v %v", upd, err)
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
	upd, err := s.UpdateSession(Session{ID: sess.ID, PromptMode: "minimal"})
	if err != nil || upd.PromptMode != "minimal" {
		t.Fatalf("UpdateSession: %v %+v", err, upd)
	}
	got, err := s.GetSession(sess.ID)
	if err != nil || got.PromptMode != "minimal" {
		t.Fatalf("GetSession prompt_mode: %v %+v", err, got)
	}
	if _, err := s.UpdateSession(Session{ID: "nope", PromptMode: "full"}); err != ErrNotFound {
		t.Fatalf("UpdateSession missing: %v", err)
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

func TestStore_VaultWikilinksAndSearch(t *testing.T) {
	s := New()
	a, err := s.PutVaultDoc(VaultDoc{Title: "Alpha", Path: "alpha.md", SHA256: "aa", Body: "see [[Beta]] needle"})
	if err != nil || a.ID == "" {
		t.Fatalf("put alpha: %v %#v", err, a)
	}
	if err := s.ReplaceVaultLinks(a.ID, []string{"Beta"}); err != nil {
		t.Fatalf("links: %v", err)
	}
	ob, ib, err := s.ListVaultDocLinks(a.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != "" || ob[0].Raw != "[[Beta]]" {
		t.Fatalf("unresolved %v %#v %#v", err, ob, ib)
	}
	b, err := s.PutVaultDoc(VaultDoc{Title: "Beta", Path: "beta.md", SHA256: "bb", Body: "see [[Alpha]]"})
	if err != nil {
		t.Fatalf("put beta: %v", err)
	}
	if err := s.ReplaceVaultLinks(b.ID, []string{"Alpha"}); err != nil {
		t.Fatalf("beta links: %v", err)
	}
	_ = s.ReResolveVaultLinks()
	ob, ib, err = s.ListVaultDocLinks(a.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != b.ID {
		t.Fatalf("alpha outbound %v %#v", err, ob)
	}
	if len(ib) != 1 || ib[0].FromID != b.ID {
		t.Fatalf("alpha inbound %#v", ib)
	}
	obB, ibB, err := s.ListVaultDocLinks(b.ID)
	if err != nil || len(obB) != 1 || obB[0].ToID != a.ID || len(ibB) != 1 {
		t.Fatalf("beta edges %v %#v %#v", err, obB, ibB)
	}
	hits, err := s.SearchVault("needle")
	if err != nil || len(hits) != 1 || hits[0].ID != a.ID {
		t.Fatalf("search %v %#v", err, hits)
	}
	empty, err := s.SearchVault("no-such-token")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty %v %#v", err, empty)
	}
	if _, err := s.GetVaultDoc("missing"); err != ErrNotFound {
		t.Fatalf("get missing: %v", err)
	}
	if err := s.DeleteVaultDoc(b.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ob, _, _ = s.ListVaultDocLinks(a.ID)
	if len(ob) != 1 || ob[0].ToID != "" {
		t.Fatalf("dangling after delete %#v", ob)
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
	ep := "http://127.0.0.1:9"
	on := true
	upd, err := s.UpdateConnector("zalocrm", &on, &ep, nil)
	if err != nil || !upd.Enabled || upd.Endpoint != ep {
		t.Fatalf("update %v %+v", err, upd)
	}
}

func TestStore_ToolFlagsDefaultOff(t *testing.T) {
	s := New()
	if s.GetToolFlag("web_search") {
		t.Fatal("default off")
	}
	if err := s.SetToolFlag("web_search", true); err != nil {
		t.Fatal(err)
	}
	if !s.GetToolFlag("web_search") {
		t.Fatal("enabled")
	}
	flags := s.ListToolFlags()
	if !flags["web_search"] {
		t.Fatalf("%v", flags)
	}
}
