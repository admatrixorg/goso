// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contextStores(t *testing.T) map[string]StoreIface {
	t.Helper()
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "ctx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	out := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, out, "goso_ctx_files")
	if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		if _, ok := out["postgres"]; !ok {
			t.Fatal("CI requires GOSO_TEST_POSTGRES_DSN for context files")
		}
	}
	return out
}

func TestContextFile_TenantAndUserIsolation(t *testing.T) {
	const (
		rule = "Chiết khấu tối đa của Minh An là 9 phần trăm."
		note = "Lan chỉ nhận báo giá một dòng."
		hoa  = "Hoa giữ bản báo giá dài."
		ins  = "Tra loi ngan gon."
	)
	for name, s := range contextStores(t) {
		t.Run(name, func(t *testing.T) {
			sales, err := s.CreateAgent(Agent{
				AgentKey: "tu-van-sales-" + name, DisplayName: "Cố vấn Minh An",
				TenantID: "minh-an-sales", Instructions: ins,
			})
			if err != nil {
				t.Fatal(err)
			}
			legal, err := s.CreateAgent(Agent{
				AgentKey: "tu-van-legal-" + name, DisplayName: "Pháp chế Minh An",
				TenantID: "minh-an-legal", Instructions: ins,
			})
			if err != nil {
				t.Fatal(err)
			}
			sess, err := s.CreateSession(Session{AgentID: sales.ID, TenantID: "minh-an-sales"})
			if err != nil {
				t.Fatal(err)
			}
			mem, err := s.PutMemory(Memory{
				SessionID: sess.ID, TenantID: "minh-an-sales", Kind: KindDurable,
				Body: "Minh An ghi nhớ báo giá ngắn cho đại lý mẫu.",
			})
			if err != nil {
				t.Fatal(err)
			}

			got, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: sales.ID,
				Name: "AGENTS.md", Body: rule,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.TenantID != "minh-an-sales" || got.AgentID != sales.ID || got.UserKey != "" || got.Body != rule {
				t.Fatalf("put agent file: %+v", got)
			}
			again, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgent, OwnerID: sales.ID,
				Name: "AGENTS.md", Body: rule + " Không vượt.",
			})
			if err != nil {
				t.Fatal(err)
			}
			if again.ID != got.ID || !strings.Contains(again.Body, "Không vượt") {
				t.Fatalf("upsert id/body: first %s again %+v", got.ID, again)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-legal", Scope: ContextScopeAgent, AgentID: sales.ID,
				Name: "AGENTS.md", Body: "legal-must-not-own-sales-agent",
			}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-tenant agent put: %v", err)
			}
			legalFile, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-legal", Scope: ContextScopeAgent, AgentID: legal.ID,
				Name: "AGENTS.md", Body: "Pháp chế Minh An không báo chiết khấu.",
			})
			if err != nil {
				t.Fatal(err)
			}

			lan, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeUser, UserKey: "lan",
				Name: "USER.md", Body: note,
			})
			if err != nil {
				t.Fatal(err)
			}
			if lan.UserKey != "lan" || lan.AgentID != "" {
				t.Fatalf("user file: %+v", lan)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeUser, UserKey: "hoa",
				Name: "USER.md", Body: hoa,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-legal", Scope: ContextScopeUser, UserKey: "lan",
				Name: "USER.md", Body: "ghi chú pháp chế cho lan",
			}); err != nil {
				t.Fatal(err)
			}

			salesAgent, err := s.GetContextFile("minh-an-sales", ContextScopeAgent, sales.ID, "AGENTS.md")
			if err != nil || !strings.Contains(salesAgent.Body, "Không vượt") {
				t.Fatalf("get sales: %v %+v", err, salesAgent)
			}
			if _, err := s.GetContextFile("minh-an-legal", ContextScopeAgent, sales.ID, "AGENTS.md"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("legal must not read sales agent file: %v", err)
			}
			if _, err := s.GetContextFile("minh-an-sales", ContextScopeAgent, legal.ID, "AGENTS.md"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("sales must not read legal agent file: %v", err)
			}
			listed, err := s.ListContextFiles("minh-an-sales", ContextScopeAgent, sales.ID)
			if err != nil || len(listed) != 1 || listed[0].ID != got.ID {
				t.Fatalf("list sales agent: %v %+v", err, listed)
			}
			legalList, err := s.ListContextFiles("minh-an-legal", ContextScopeAgent, legal.ID)
			if err != nil || len(legalList) != 1 || legalList[0].ID != legalFile.ID || strings.Contains(legalList[0].Body, "Không vượt") {
				t.Fatalf("list legal: %v %+v", err, legalList)
			}

			lanGot, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "lan", "USER.md")
			if err != nil || lanGot.Body != note {
				t.Fatalf("lan sales: %v %+v", err, lanGot)
			}
			hoaGot, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "hoa", "USER.md")
			if err != nil || hoaGot.Body != hoa || strings.Contains(hoaGot.Body, "Lan") {
				t.Fatalf("hoa: %v %+v", err, hoaGot)
			}
			lanLegal, err := s.GetContextFile("minh-an-legal", ContextScopeUser, "lan", "USER.md")
			if err != nil || lanLegal.Body == note || lanLegal.TenantID != "minh-an-legal" {
				t.Fatalf("same user key other tenant: %v %+v", err, lanLegal)
			}
			if _, err := s.ListContextFiles("minh-an-sales", ContextScopeUser, ""); !errors.Is(err, ErrContextInvalid) {
				t.Fatalf("empty user list: %v", err)
			}

			if err := s.DeleteContextFile("minh-an-legal", ContextScopeUser, "lan", "USER.md"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetContextFile("minh-an-legal", ContextScopeUser, "lan", "USER.md"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("deleted legal lan still visible: %v", err)
			}
			if still, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "lan", "USER.md"); err != nil || still.Body != note {
				t.Fatalf("delete leaked across tenants: %v %+v", err, still)
			}
			if err := s.DeleteContextFile("minh-an-sales", ContextScopeUser, "hoa", "USER.md"); err != nil {
				t.Fatal(err)
			}
			if still, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "lan", "USER.md"); err != nil || still.Body != note {
				t.Fatalf("delete leaked across users: %v %+v", err, still)
			}

			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: sales.ID,
				Name: "../AGENTS.md", Body: rule,
			}); !errors.Is(err, ErrContextInvalid) {
				t.Fatalf("path name: %v", err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeUser, UserKey: "lan@minh-an",
				Name: "USER.md", Body: note,
			}); !errors.Is(err, ErrContextInvalid) {
				t.Fatalf("email user key: %v", err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: sales.ID,
				Name: "AGENTS.md", Body: strings.Repeat("a", MaxContextFileBytes+1),
			}); !errors.Is(err, ErrContextTooLarge) {
				t.Fatalf("oversize: %v", err)
			}
			if still, err := s.GetContextFile("minh-an-sales", ContextScopeAgent, sales.ID, "AGENTS.md"); err != nil || strings.Contains(still.Body, "aaa") {
				t.Fatalf("oversize replaced the file: %v %+v", err, still)
			}

			fresh, err := s.GetAgent(sales.ID)
			if err != nil || fresh.Instructions != ins {
				t.Fatalf("instructions changed: %+v %v", fresh, err)
			}
			mems, err := s.ListMemories(sess.ID)
			if err != nil || len(mems) != 1 || mems[0].ID != mem.ID || strings.Contains(mems[0].Body, "9 phần trăm") {
				t.Fatalf("memory rows: %v %+v", err, mems)
			}
			hits, err := s.RecallMemories(RecallQuery{
				TenantID: "minh-an-sales", AgentID: sales.ID, Query: "báo giá ngắn đại lý mẫu",
			})
			if err != nil || len(hits) != 1 || hits[0].Memory.ID != mem.ID {
				t.Fatalf("recall after context file: %v %+v", err, hits)
			}
			if docs := s.ListVaultDocs(); len(docs) != 0 {
				t.Fatalf("context file landed in vault: %d", len(docs))
			}
		})
	}
}
