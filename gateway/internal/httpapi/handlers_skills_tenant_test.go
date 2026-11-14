// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestSkillsHTTP_TenantScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-184")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})

	rec := skillHTTP(t, h, "POST", "/api/skills", `{"name":"tu-van","body":"shared counsel"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("default create %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "POST", "/api/skills", `{"name":"tu-van","body":"Cố vấn Minh An rieng"}`, "minh-an-sales")
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "t/minh-an-sales/tu-van/SKILL.md") {
		t.Fatalf("sales create %d %s", rec.Code, rec.Body.String())
	}
	rootBody, err := os.ReadFile(filepath.Join(root, "tu-van", "SKILL.md"))
	if err != nil || string(rootBody) != "shared counsel" {
		t.Fatalf("root rewritten %v %q", err, rootBody)
	}
	rec = skillHTTP(t, h, "POST", "/api/skills", `{"name":"bao-gia","body":"Cố vấn Minh An bao gia"}`, "minh-an-sales")
	if rec.Code != http.StatusCreated {
		t.Fatalf("sales private %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "bao-gia")); !os.IsNotExist(err) {
		t.Fatalf("private skill landed at the root: %v", err)
	}

	rec = skillHTTP(t, h, "GET", "/api/skills", "", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "bao-gia") || !strings.Contains(rec.Body.String(), "tu-van") {
		t.Fatalf("default list %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "GET", "/api/skills?name=tu-van", "", "minh-an-sales")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "rieng") {
		t.Fatalf("sales load %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "GET", "/api/skills?name=tu-van", "", "minh-an-legal")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "shared counsel") || strings.Contains(rec.Body.String(), "rieng") {
		t.Fatalf("legal load %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "GET", "/api/skills?q=baogia", "", "minh-an-legal")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "bao-gia") {
		t.Fatalf("legal search %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "GET", "/api/skills?q=bao", "", "minh-an-sales")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "bao-gia") {
		t.Fatalf("sales search %d %s", rec.Code, rec.Body.String())
	}

	rec = skillHTTP(t, h, "DELETE", "/api/skills/tu-van", "", "minh-an-legal")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("legal delete %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "tu-van", "SKILL.md")); err != nil {
		t.Fatalf("shared skill deleted: %v", err)
	}
	rec = skillHTTP(t, h, "DELETE", "/api/skills/bao-gia", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("default delete of tenant skill %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an-sales", "bao-gia", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	rec = skillHTTP(t, h, "POST", "/api/skills", `{"name":"t","body":"nope"}`, "minh-an-sales")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid name") {
		t.Fatalf("reserved name %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "GET", "/api/skills?name=../tu-van", "", "minh-an-sales")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "path escape") {
		t.Fatalf("name escape %d %s", rec.Code, rec.Body.String())
	}
	rec = skillHTTP(t, h, "GET", "/api/skills", "", "..")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "path escape") {
		t.Fatalf("tenant escape %d %s", rec.Code, rec.Body.String())
	}
}

func TestChat_SkillToolsUseAgentTenant(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-184")

	st := store.New()
	sales, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an-sales"})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-legal", DisplayName: "Cố vấn Minh An", TenantID: "minh-an-legal"})
	if err != nil {
		t.Fatal(err)
	}
	salesSess, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: "minh-an-sales"})
	if err != nil {
		t.Fatal(err)
	}
	legalSess, err := st.CreateSession(store.Session{AgentID: legal.ID, TenantID: "minh-an-legal"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Options{Store: st, Version: "0.1.0"})
	skillHTTP(t, h, "POST", "/api/skills", `{"name":"bao-gia","body":"shared quote"}`, "")
	skillHTTP(t, h, "POST", "/api/skills", `{"name":"bao-gia","body":"Cố vấn Minh An sales quote"}`, "minh-an-sales")
	skillHTTP(t, h, "POST", "/api/skills", `{"name":"bao-gia","body":"Cố vấn Minh An legal quote"}`, "minh-an-legal")

	salesTools := chatSkillTools(t, st, salesSess.ID, "minh-an-sales")
	if !strings.Contains(salesTools, "sales quote") || strings.Contains(salesTools, "legal quote") {
		t.Fatalf("sales chat %s", salesTools)
	}
	legalTools := chatSkillTools(t, st, legalSess.ID, "minh-an-legal")
	if !strings.Contains(legalTools, "legal quote") || strings.Contains(legalTools, "sales quote") {
		t.Fatalf("legal chat %s", legalTools)
	}
}

func skillHTTP(t *testing.T, h http.Handler, method, path, body, tenant string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if tenant != "" {
		r.Header.Set("X-Goso-Tenant", tenant)
		r.Header.Set("Authorization", "Bearer admin-184")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func chatSkillTools(t *testing.T, st store.StoreIface, sessionID, tenant string) string {
	t.Helper()
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "u1", Name: "builtin__use_skill", Arguments: map[string]any{"name": "bao-gia"}}}},
		{ToolCalls: []llm.ToolCall{{ID: "s1", Name: "builtin__skill_search", Arguments: map[string]any{"query": "quote"}}}},
		{Text: "done"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(16), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	payload, _ := json.Marshal(map[string]string{"session_id": sessionID, "message": "find the quote"})
	w := skillHTTP(t, h, "POST", "/api/chat", string(payload), tenant)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	msgs, err := st.ListMessages(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, m := range msgs {
		if m != nil && m.Role == "tool" {
			b.WriteString(m.Content)
			b.WriteByte('\n')
		}
	}
	if !strings.Contains(b.String(), "use_skill") && !strings.Contains(b.String(), "bao-gia") {
		t.Fatalf("tool messages missing: %s", b.String())
	}
	return b.String()
}
