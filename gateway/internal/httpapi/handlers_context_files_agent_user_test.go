// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// echoSystem answers with the system text so a test can see which context
// files reached the model.
type echoSystem struct{}

func (echoSystem) Name() string      { return "echo" }
func (echoSystem) ModelName() string { return "echo" }

func (echoSystem) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	var sys strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			sys.WriteString(m.Content)
		}
	}
	return sys.String(), nil
}

// TestContextFilesHTTP_AgentUser: scope=user with agent_id and scope=agent_user
// address the same per-agent file. It stays inside its tenant and its agent,
// and a chat turn with that agent reads it before the tenant-wide file.
func TestContextFilesHTTP_AgentUser(t *testing.T) {
	st := store.New()
	sales, err := st.CreateAgent(store.Agent{AgentKey: "au-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	care, err := st.CreateAgent(store.Agent{AgentKey: "au-care", DisplayName: "Chăm sóc Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := st.CreateAgent(store.Agent{AgentKey: "au-branch", DisplayName: "Chi nhánh Minh An", TenantID: "minh-an-2"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-ctx")
	h := NewRouter(Options{Store: st, Version: "t", Runtime: agent.New(st, nil, nil, nil, echoSystem{})})

	do := func(method, tenant, query, body string) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, "/api/context-files?"+query, nil)
		} else {
			req = httptest.NewRequest(method, "/api/context-files", strings.NewReader(body))
		}
		req.Header.Set("Authorization", "Bearer admin-ctx")
		req.Header.Set("X-Goso-Tenant", tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// scope=user + agent_id is the per-agent file.
	rec := do(http.MethodPut, "minh-an", "", `{"scope":"user","agent_id":"`+sales.ID+`","user_key":"lan","name":"USER.md","body":"Lan mua sỉ (sales)."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put user+agent %d %s", rec.Code, rec.Body.String())
	}
	var saved store.ContextFile
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Scope != store.ContextScopeAgentUser || saved.AgentID != sales.ID || saved.UserKey != "lan" {
		t.Fatalf("saved %+v", saved)
	}
	// scope=agent_user with both is the same row.
	rec = do(http.MethodPut, "minh-an", "", `{"scope":"agent_user","agent_id":"`+care.ID+`","user_key":"lan","name":"USER.md","body":"Lan hỏi bảo hành (care)."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put agent_user %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodPut, "minh-an", "", `{"scope":"user","user_key":"lan","name":"PREFS.md","body":"Lan thích Zalo."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put tenant user %d %s", rec.Code, rec.Body.String())
	}

	bad := []string{
		`{"scope":"agent_user","user_key":"lan","name":"USER.md","body":"x"}`,
		`{"scope":"agent_user","agent_id":"` + sales.ID + `","name":"USER.md","body":"x"}`,
		`{"scope":"user","agent_id":"a/b","user_key":"lan","name":"USER.md","body":"x"}`,
	}
	for _, b := range bad {
		if rec := do(http.MethodPut, "minh-an", "", b); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad %s -> %d %s", b, rec.Code, rec.Body.String())
		}
	}
	// Another tenant's agent cannot own a minh-an file, and minh-an cannot
	// write under it.
	if rec := do(http.MethodPut, "minh-an", "", `{"scope":"agent_user","agent_id":"`+branch.ID+`","user_key":"lan","name":"USER.md","body":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign agent put %d %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodGet, "minh-an", "scope=agent_user&agent_id="+sales.ID+"&user_key=lan&name=USER.md", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "(sales)") {
		t.Fatalf("get sales %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, "minh-an", "scope=user&agent_id="+care.ID+"&user_key=lan", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "(care)") || strings.Contains(rec.Body.String(), "(sales)") || strings.Contains(rec.Body.String(), "Zalo") {
		t.Fatalf("list care %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, "minh-an", "scope=user&user_key=lan", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Zalo") || strings.Contains(rec.Body.String(), "(sales)") {
		t.Fatalf("list tenant user %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, "minh-an-2", "scope=agent_user&agent_id="+sales.ID+"&user_key=lan&name=USER.md", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, "minh-an-2", "scope=agent_user&agent_id="+sales.ID+"&user_key=lan", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "(sales)") {
		t.Fatalf("cross-tenant list %d %s", rec.Code, rec.Body.String())
	}

	chat := func(agentID string) string {
		t.Helper()
		sess, err := st.CreateSession(store.Session{AgentID: agentID, TenantID: "minh-an"})
		if err != nil {
			t.Fatal(err)
		}
		body := `{"session_id":"` + sess.ID + `","message":"Lan cần gì?","prompt_mode":"full"}`
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin-ctx")
		req.Header.Set("X-Goso-Tenant", "minh-an")
		req.Header.Set("X-Goso-User", "lan")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	got := chat(sales.ID)
	if !strings.Contains(got, "(sales)") || strings.Contains(got, "(care)") || !strings.Contains(got, "Zalo") {
		t.Fatalf("sales chat %s", got)
	}
	got = chat(care.ID)
	if !strings.Contains(got, "(care)") || strings.Contains(got, "(sales)") {
		t.Fatalf("care chat %s", got)
	}

	// Delete from another tenant misses; from minh-an it removes only sales.
	if rec := do(http.MethodDelete, "minh-an-2", "scope=agent_user&agent_id="+sales.ID+"&user_key=lan&name=USER.md", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodDelete, "minh-an", "scope=user&agent_id="+sales.ID+"&user_key=lan&name=USER.md", ""); rec.Code/100 != 2 {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	if _, err := st.GetContextFile("minh-an", store.ContextScopeAgentUser, store.AgentUserOwner(care.ID, "lan"), "USER.md"); err != nil {
		t.Fatalf("care file gone: %v", err)
	}
	got = chat(sales.ID)
	if strings.Contains(got, "(sales)") || !strings.Contains(got, "Zalo") {
		t.Fatalf("sales chat after delete %s", got)
	}
}
