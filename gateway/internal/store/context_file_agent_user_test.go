// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentUserOwner(t *testing.T) {
	owner := AgentUserOwner("20260101-ab12", "zalo:s-lan")
	a, u, ok := SplitAgentUserOwner(owner)
	if !ok || a != "20260101-ab12" || u != "zalo:s-lan" {
		t.Fatalf("split %q: %q %q %v", owner, a, u, ok)
	}
	for _, bad := range []string{"", "/lan", "agent/", "agent", "a/b/lan", "agent/lan@minh-an", "agent/l an"} {
		if _, _, ok := SplitAgentUserOwner(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestContextFile_AgentUserScope(t *testing.T) {
	for name, s := range contextStores(t) {
		t.Run(name, func(t *testing.T) {
			sales, err := s.CreateAgent(Agent{AgentKey: "au-sales-" + name, DisplayName: "Cố vấn Minh An", TenantID: "minh-an-sales"})
			if err != nil {
				t.Fatal(err)
			}
			care, err := s.CreateAgent(Agent{AgentKey: "au-care-" + name, DisplayName: "Chăm sóc Minh An", TenantID: "minh-an-sales"})
			if err != nil {
				t.Fatal(err)
			}
			other, err := s.CreateAgent(Agent{AgentKey: "au-legal-" + name, DisplayName: "Pháp chế Minh An", TenantID: "minh-an-legal"})
			if err != nil {
				t.Fatal(err)
			}
			// Two agents keep different bodies for the same user and name.
			a, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: sales.ID, UserKey: "lan",
				Name: "USER.md", Body: "Lan mua sỉ.",
			})
			if err != nil {
				t.Fatal(err)
			}
			if a.AgentID != sales.ID || a.UserKey != "lan" || a.Scope != ContextScopeAgentUser {
				t.Fatalf("presented %+v", a)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: care.ID, UserKey: "lan",
				Name: "USER.md", Body: "Lan hỏi bảo hành.",
			}); err != nil {
				t.Fatal(err)
			}
			// The tenant-wide user file with the same name is a separate row.
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeUser, UserKey: "lan",
				Name: "USER.md", Body: "Lan dùng Zalo.",
			}); err != nil {
				t.Fatal(err)
			}
			got, err := s.GetContextFile("minh-an-sales", ContextScopeAgentUser, AgentUserOwner(care.ID, "lan"), "USER.md")
			if err != nil || got.Body != "Lan hỏi bảo hành." {
				t.Fatalf("care file %+v %v", got, err)
			}
			got, err = s.GetContextFile("minh-an-sales", ContextScopeAgentUser, AgentUserOwner(sales.ID, "lan"), "USER.md")
			if err != nil || got.Body != "Lan mua sỉ." {
				t.Fatalf("sales file %+v %v", got, err)
			}
			listed, err := s.ListContextFiles("minh-an-sales", ContextScopeAgentUser, AgentUserOwner(sales.ID, "lan"))
			if err != nil || len(listed) != 1 || listed[0].AgentID != sales.ID || listed[0].UserKey != "lan" {
				t.Fatalf("list %v %+v", err, listed)
			}
			shared, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "lan", "USER.md")
			if err != nil || shared.Body != "Lan dùng Zalo." {
				t.Fatalf("tenant user file %+v %v", shared, err)
			}

			// Isolation: an agent of another tenant cannot own a file here,
			// and a read from that tenant sees nothing.
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: other.ID, UserKey: "lan",
				Name: "USER.md", Body: "x",
			}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-tenant agent write: %v", err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-legal", Scope: ContextScopeAgentUser, AgentID: sales.ID, UserKey: "lan",
				Name: "USER.md", Body: "x",
			}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("write into the other tenant: %v", err)
			}
			if _, err := s.GetContextFile("minh-an-legal", ContextScopeAgentUser, AgentUserOwner(sales.ID, "lan"), "USER.md"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-tenant read: %v", err)
			}
			if l, err := s.ListContextFiles("minh-an-legal", ContextScopeAgentUser, AgentUserOwner(sales.ID, "lan")); err != nil || len(l) != 0 {
				t.Fatalf("cross-tenant list %v %d", err, len(l))
			}
			if err := s.DeleteContextFile("minh-an-legal", ContextScopeAgentUser, AgentUserOwner(sales.ID, "lan"), "USER.md"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-tenant delete: %v", err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: "missing", UserKey: "lan",
				Name: "USER.md", Body: "x",
			}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unknown agent: %v", err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: sales.ID, UserKey: "lan@minh-an",
				Name: "USER.md", Body: "x",
			}); !errors.Is(err, ErrContextInvalid) {
				t.Fatalf("bad user key: %v", err)
			}

			if err := s.DeleteContextFile("minh-an-sales", ContextScopeAgentUser, AgentUserOwner(sales.ID, "lan"), "USER.md"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.GetContextFile("minh-an-sales", ContextScopeAgentUser, AgentUserOwner(care.ID, "lan"), "USER.md"); err != nil {
				t.Fatalf("delete touched the other agent: %v", err)
			}
			if _, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "lan", "USER.md"); err != nil {
				t.Fatalf("delete touched the tenant user file: %v", err)
			}
		})
	}
}

// TestContextFile_DefaultCapsMatchGoClaw stores what GoClaw accepts: eleven
// knowledge files of about 80 KB for one agent, more than 16 files, and one
// body over 32 KiB. Only a body over 512 KiB is refused.
func TestContextFile_DefaultCapsMatchGoClaw(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_MAX_FILES", "")
	t.Setenv("GOSO_CONTEXT_MAX_BYTES", "")
	t.Setenv("GOSO_CONTEXT_FILE_MAX_BYTES", "")
	caps := ContextLimits()
	if caps.FileBytes != 512<<10 || caps.FilesPerOwner != 0 || caps.BytesPerOwner != 0 {
		t.Fatalf("defaults %+v", caps)
	}
	for name, s := range contextStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, err := s.CreateAgent(Agent{AgentKey: "kn-" + name, DisplayName: "Kiến thức Minh An", TenantID: "minh-an-sales"})
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 11; i++ {
				if _, err := s.PutContextFile(ContextFile{
					TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: ag.ID,
					Name: fmt.Sprintf("kn%02d.md", i), Body: minhAnSized(7400),
				}); err != nil {
					t.Fatalf("knowledge file %d: %v", i, err)
				}
			}
			for i := 12; i <= 20; i++ {
				if _, err := s.PutContextFile(ContextFile{
					TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: ag.ID,
					Name: fmt.Sprintf("kn%02d.md", i), Body: "Minh An ghi chú.",
				}); err != nil {
					t.Fatalf("file %d: %v", i, err)
				}
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: ag.ID, UserKey: "lan",
				Name: "big.md", Body: minhAnSized(100 << 10),
			}); err != nil {
				t.Fatalf("100 KiB body: %v", err)
			}
			if _, err := s.PutContextFile(ContextFile{
				TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: ag.ID,
				Name: "huge.md", Body: minhAnSized(512<<10 + 1),
			}); !errors.Is(err, ErrContextTooLarge) {
				t.Fatalf("over 512 KiB: %v", err)
			}
			files, err := s.ListContextFiles("minh-an-sales", ContextScopeAgent, ag.ID)
			if err != nil || len(files) != 20 {
				t.Fatalf("list %v %d", err, len(files))
			}
			if err := ContextFilesOverCap(files); err != nil {
				t.Fatalf("chat would refuse the set: %v", err)
			}
		})
	}
}

func TestContextLimitsFromEnv(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILE_MAX_BYTES", "1024")
	t.Setenv("GOSO_CONTEXT_MAX_FILES", "3")
	t.Setenv("GOSO_CONTEXT_MAX_BYTES", "2048")
	if c := ContextLimits(); c.FileBytes != 1024 || c.FilesPerOwner != 3 || c.BytesPerOwner != 2048 {
		t.Fatalf("env caps %+v", c)
	}
	t.Setenv("GOSO_CONTEXT_FILE_MAX_BYTES", "0")
	t.Setenv("GOSO_CONTEXT_MAX_FILES", "-1")
	t.Setenv("GOSO_CONTEXT_MAX_BYTES", "lots")
	if c := ContextLimits(); c.FileBytes != DefaultContextFileMaxBytes || c.FilesPerOwner != 0 || c.BytesPerOwner != 0 {
		t.Fatalf("fallback caps %+v", c)
	}
	files := []*ContextFile{{Body: "a"}, {Body: "b"}, {Body: "c"}, {Body: "d"}}
	if err := ContextFilesOverCap(files); err != nil {
		t.Fatalf("no limit: %v", err)
	}
	t.Setenv("GOSO_CONTEXT_MAX_FILES", "3")
	if err := ContextFilesOverCap(files); !errors.Is(err, ErrContextOwnerCap) {
		t.Fatalf("file cap: %v", err)
	}
}

// TestContextFile_AgentUserOnExistingDB opens a database that already holds
// agent and tenant-user rows, opens it again (migrations run twice), and
// checks the old rows are unchanged and agent_user rows work beside them.
func TestContextFile_AgentUserOnExistingDB(t *testing.T) {
	type opener func() (StoreIface, func())
	backends := map[string]opener{}
	path := filepath.Join(t.TempDir(), "legacy.db")
	backends["sqlite"] = func() (StoreIface, func()) {
		s, err := OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		return s, func() { _ = s.Close() }
	}
	if dsn := postgresTestDSN(); dsn != "" {
		pgDSN := prepareSchema(t, dsn, "goso_ctx_au_mig", false)
		backends["postgres"] = func() (StoreIface, func()) {
			s, err := OpenPostgres(pgDSN)
			if err != nil {
				t.Fatal(err)
			}
			return s, func() { _ = s.Close() }
		}
	}
	for name, open := range backends {
		t.Run(name, func(t *testing.T) {
			s, closeFn := open()
			ag, err := s.CreateAgent(Agent{AgentKey: "mig-" + name, DisplayName: "Cố vấn Minh An", TenantID: "minh-an-sales"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.PutContextFile(ContextFile{TenantID: "minh-an-sales", Scope: ContextScopeAgent, AgentID: ag.ID, Name: "AGENTS.md", Body: "Quy tắc Minh An."}); err != nil {
				t.Fatal(err)
			}
			legacy, err := s.PutContextFile(ContextFile{TenantID: "minh-an-sales", Scope: ContextScopeUser, UserKey: "lan", Name: "USER.md", Body: "Lan dùng Zalo."})
			if err != nil {
				t.Fatal(err)
			}
			closeFn()
			for round := 0; round < 2; round++ {
				s, closeFn = open()
				got, err := s.GetContextFile("minh-an-sales", ContextScopeUser, "lan", "USER.md")
				if err != nil || got.ID != legacy.ID || got.Body != "Lan dùng Zalo." {
					t.Fatalf("round %d legacy user row %+v %v", round, got, err)
				}
				if _, err := s.GetContextFile("minh-an-sales", ContextScopeAgent, ag.ID, "AGENTS.md"); err != nil {
					t.Fatalf("round %d agent row: %v", round, err)
				}
				if _, err := s.PutContextFile(ContextFile{TenantID: "minh-an-sales", Scope: ContextScopeAgentUser, AgentID: ag.ID, UserKey: "lan", Name: "USER.md", Body: fmt.Sprintf("Lan vòng %d.", round)}); err != nil {
					t.Fatalf("round %d agent_user put: %v", round, err)
				}
				au, err := s.ListContextFiles("minh-an-sales", ContextScopeAgentUser, AgentUserOwner(ag.ID, "lan"))
				if err != nil || len(au) != 1 || !strings.Contains(au[0].Body, fmt.Sprint(round)) {
					t.Fatalf("round %d agent_user list %v %+v", round, err, au)
				}
				closeFn()
			}
		})
	}
}
