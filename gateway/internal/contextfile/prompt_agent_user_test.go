// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextfile

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func putFile(t *testing.T, st store.StoreIface, f store.ContextFile) {
	t.Helper()
	if _, err := st.PutContextFile(f); err != nil {
		t.Fatal(err)
	}
}

// TestPrompt_AgentUserBeforeTenantUser: for agent A and user U the prompt has
// A's files, then A's files for U, then U's tenant-wide files whose name A has
// no per-user file for. Agent B and another tenant never see A's U files.
func TestPrompt_AgentUserBeforeTenantUser(t *testing.T) {
	st := store.New()
	sales, err := st.CreateAgent(store.Agent{AgentKey: "p-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	care, err := st.CreateAgent(store.Agent{AgentKey: "p-care", DisplayName: "Chăm sóc Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAgent(store.Agent{AgentKey: "p-other", DisplayName: "Chi nhánh Minh An", TenantID: "minh-an-2"})
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, st, store.ContextFile{TenantID: "minh-an", Scope: store.ContextScopeAgent, AgentID: sales.ID, Name: "AGENTS.md", Body: "Quy tắc bán hàng."})
	putFile(t, st, store.ContextFile{TenantID: "minh-an", Scope: store.ContextScopeAgentUser, AgentID: sales.ID, UserKey: "lan", Name: "USER.md", Body: "Lan mua sỉ (sales)."})
	putFile(t, st, store.ContextFile{TenantID: "minh-an", Scope: store.ContextScopeUser, UserKey: "lan", Name: "USER.md", Body: "Lan chung."})
	putFile(t, st, store.ContextFile{TenantID: "minh-an", Scope: store.ContextScopeUser, UserKey: "lan", Name: "PREFS.md", Body: "Lan thích Zalo."})
	putFile(t, st, store.ContextFile{TenantID: "minh-an", Scope: store.ContextScopeAgentUser, AgentID: care.ID, UserKey: "lan", Name: "NOTES.md", Body: "Lan hỏi bảo hành (care)."})
	putFile(t, st, store.ContextFile{TenantID: "minh-an-2", Scope: store.ContextScopeAgentUser, AgentID: other.ID, UserKey: "lan", Name: "USER.md", Body: "Lan chi nhánh."})

	got, err := Prompt(st, "minh-an", sales.ID, "lan")
	if err != nil {
		t.Fatal(err)
	}
	want := "AGENTS.md:\nQuy tắc bán hàng.\n\nUSER.md:\nLan mua sỉ (sales).\n\nPREFS.md:\nLan thích Zalo."
	if got != want {
		t.Fatalf("sales prompt\n%s\nwant\n%s", got, want)
	}

	got, err = Prompt(st, "minh-an", care.ID, "lan")
	if err != nil {
		t.Fatal(err)
	}
	want = "NOTES.md:\nLan hỏi bảo hành (care).\n\nPREFS.md:\nLan thích Zalo.\n\nUSER.md:\nLan chung."
	if got != want {
		t.Fatalf("care prompt\n%s\nwant\n%s", got, want)
	}

	// No agent on the turn: tenant-wide user files only.
	got, err = Prompt(st, "minh-an", "", "lan")
	if err != nil || strings.Contains(got, "(sales)") || strings.Contains(got, "(care)") || !strings.Contains(got, "Lan chung.") {
		t.Fatalf("no agent %q %v", got, err)
	}

	// Another tenant: the sales agent id finds nothing there, and that
	// tenant's own agent_user file never reaches minh-an.
	got, err = Prompt(st, "minh-an-2", sales.ID, "lan")
	if err != nil || got != "" {
		t.Fatalf("cross-tenant %q %v", got, err)
	}
	got, err = Prompt(st, "minh-an", other.ID, "lan")
	if err != nil || strings.Contains(got, "chi nhánh") {
		t.Fatalf("other tenant agent %q %v", got, err)
	}
}

// TestPrompt_KnowledgeSetUnderDefaultCaps is the case GoClaw loaded and the old
// 64 KiB cap refused: eleven knowledge files of about 80 KB for one agent.
func TestPrompt_KnowledgeSetUnderDefaultCaps(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_MAX_FILES", "")
	t.Setenv("GOSO_CONTEXT_MAX_BYTES", "")
	t.Setenv("GOSO_CONTEXT_FILE_MAX_BYTES", "")
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "p-kn", DisplayName: "Kiến thức Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 11; i++ {
		putFile(t, st, store.ContextFile{
			TenantID: "minh-an", Scope: store.ContextScopeAgent, AgentID: ag.ID,
			Name: fmt.Sprintf("kn%02d.md", i), Body: "Minh An " + strings.Repeat("k", 7400),
		})
	}
	got, err := Prompt(st, "minh-an", ag.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 80000 || strings.Count(got, ".md:\n") != 11 {
		t.Fatalf("prompt %d bytes, %d files", len(got), strings.Count(got, ".md:\n"))
	}
}
