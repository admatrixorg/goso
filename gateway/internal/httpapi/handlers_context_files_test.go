// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	httpRule = "Chiết khấu tối đa của Minh An là 9 phần trăm."
	httpNote = "Lan chỉ nhận báo giá một dòng."
)

type obeyMinhAn struct{}

func (obeyMinhAn) Name() string      { return "obey" }
func (obeyMinhAn) ModelName() string { return "obey" }

func (obeyMinhAn) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	var sys strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			sys.WriteString(m.Content)
		}
	}
	text := sys.String()
	if strings.Contains(text, httpRule) && strings.Contains(text, httpNote) {
		return httpRule + " " + httpNote, nil
	}
	if strings.Contains(text, httpRule) {
		return httpRule, nil
	}
	return "không rõ", nil
}

type ctxPutFail struct{ store.StoreIface }

func (ctxPutFail) PutContextFile(store.ContextFile) (*store.ContextFile, error) {
	return nil, errors.New("disk I/O Chiết khấu lan")
}

type ctxPutCount struct {
	store.StoreIface
	puts int
}

func (c *ctxPutCount) PutContextFile(f store.ContextFile) (*store.ContextFile, error) {
	c.puts++
	return c.StoreIface.PutContextFile(f)
}

func TestContextFilesHTTP_FlagTenantAndAnswer(t *testing.T) {
	st := store.New()
	sales, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-http", DisplayName: "Cố vấn Minh An",
		TenantID: "minh-an-sales", Instructions: "Tra loi ngan gon.",
	})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := st.CreateAgent(store.Agent{
		AgentKey: "phap-che-http", DisplayName: "Pháp chế Minh An",
		TenantID: "minh-an-legal",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = legal
	sess, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: "minh-an-sales"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("flag off", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "")
		counted := &ctxPutCount{StoreIface: st}
		h := NewRouter(Options{Store: counted, Version: "t"})
		req := httptest.NewRequest(http.MethodPut, "/api/context-files", strings.NewReader(`{"scope":"agent","agent_id":"`+sales.ID+`","name":"AGENTS.md","body":"`+httpRule+`"}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if counted.puts != 0 {
			t.Fatalf("flag off wrote %d", counted.puts)
		}
		if strings.Contains(rec.Body.String(), "Chiết") {
			t.Fatalf("body echoed the file: %s", rec.Body.String())
		}
	})

	t.Setenv("GOSO_CONTEXT_FILES", "1")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-ctx")
	h := NewRouter(Options{Store: st, Version: "t", Runtime: agent.New(st, nil, nil, nil, obeyMinhAn{})})

	put := func(tenant, payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/context-files", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer admin-ctx")
		req.Header.Set("X-Goso-Tenant", tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := put("minh-an-sales", `{"scope":"agent","agent_id":"`+sales.ID+`","name":"AGENTS.md","body":"`+httpRule+`","tenant_id":"minh-an-legal"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), httpRule) {
		t.Fatalf("put %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "minh-an-legal") {
		t.Fatalf("forged tenant stored: %s", rec.Body.String())
	}
	rec = put("minh-an-sales", `{"scope":"user","user_key":"lan","name":"USER.md","body":"`+httpNote+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put user %d %s", rec.Code, rec.Body.String())
	}

	get := func(tenant, raw string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/context-files?"+raw, nil)
		req.Header.Set("Authorization", "Bearer admin-ctx")
		req.Header.Set("X-Goso-Tenant", tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec = get("minh-an-legal", "scope=agent&agent_id="+sales.ID+"&name=AGENTS.md")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get %d %s", rec.Code, rec.Body.String())
	}
	rec = get("minh-an-sales", "scope=user&user_key=hoa&name=USER.md")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other user get %d %s", rec.Code, rec.Body.String())
	}
	rec = get("minh-an-sales", "scope=agent&agent_id="+sales.ID+"&name=AGENTS.md")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), httpRule) {
		t.Fatalf("sales get %d %s", rec.Code, rec.Body.String())
	}

	chat := func(user string) *httptest.ResponseRecorder {
		t.Helper()
		body := `{"session_id":"` + sess.ID + `","message":"Chiết khấu tối đa là bao nhiêu?","prompt_mode":"full"}`
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin-ctx")
		req.Header.Set("X-Goso-Tenant", "minh-an-sales")
		if user != "" {
			req.Header.Set("X-Goso-User", user)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec = chat("lan")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), httpRule) || !strings.Contains(rec.Body.String(), httpNote) {
		t.Fatalf("chat lan %d %s", rec.Code, rec.Body.String())
	}
	rec = chat("hoa")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), httpRule) || strings.Contains(rec.Body.String(), httpNote) {
		t.Fatalf("chat hoa %d %s", rec.Code, rec.Body.String())
	}

	t.Setenv("GOSO_CONTEXT_FILES", "")
	rec = chat("lan")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), httpRule) || !strings.Contains(rec.Body.String(), "không rõ") {
		t.Fatalf("flag off chat %d %s", rec.Code, rec.Body.String())
	}
	fresh, err := st.GetAgent(sales.ID)
	if err != nil || fresh.Instructions != "Tra loi ngan gon." {
		t.Fatalf("instructions: %+v %v", fresh, err)
	}
}

func TestContextFilesHTTP_StoreFailure(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	h := NewRouter(Options{Store: ctxPutFail{StoreIface: store.New()}, Version: "t"})
	req := httptest.NewRequest(http.MethodPut, "/api/context-files", bytes.NewBufferString(`{"scope":"user","user_key":"lan","name":"USER.md","body":"Lan chỉ nhận báo giá một dòng."}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"store"}` {
		t.Fatalf("body %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Chiết") || strings.Contains(rec.Body.String(), "Lan") || strings.Contains(rec.Body.String(), "disk") {
		t.Fatalf("leaked %s", rec.Body.String())
	}
}

func TestContextFilesHTTP_OwnerCap(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-http-cap", DisplayName: "Cố vấn Minh An",
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Options{Store: st, Version: "t"})
	put := func(name, body string) *httptest.ResponseRecorder {
		t.Helper()
		raw := fmt.Sprintf(`{"scope":"agent","agent_id":"%s","name":"%s","body":"%s"}`, ag.ID, name, body)
		req := httptest.NewRequest(http.MethodPut, "/api/context-files", strings.NewReader(raw))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for i := 1; i <= store.MaxContextFilesPerOwner; i++ {
		rec := put(fmt.Sprintf("n%02d.md", i), "Minh An ghi chu.")
		if rec.Code != http.StatusOK {
			t.Fatalf("file %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := put("n17.md", "Minh An vuot so file.")
	if rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != `{"error":"too_large"}` {
		t.Fatalf("17th: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := st.GetContextFile("default", store.ContextScopeAgent, ag.ID, "n17.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("17th stored: %v", err)
	}
	listed, err := st.ListContextFiles("default", store.ContextScopeAgent, ag.ID)
	if err != nil || len(listed) != store.MaxContextFilesPerOwner {
		t.Fatalf("rows %v %d", err, len(listed))
	}
}

type httpOverCap struct{ store.StoreIface }

func (httpOverCap) ListContextFiles(tenant, scope, owner string) ([]*store.ContextFile, error) {
	out := make([]*store.ContextFile, store.MaxContextFilesPerOwner+1)
	for i := range out {
		out[i] = &store.ContextFile{
			TenantID: tenant, Scope: scope, AgentID: owner,
			Name: fmt.Sprintf("n%02d.md", i+1), Body: "Minh An ghi chu.",
		}
	}
	return out, nil
}

type callCount struct{ calls int }

func (c *callCount) Name() string      { return "obey" }
func (c *callCount) ModelName() string { return "obey" }
func (c *callCount) Chat(context.Context, []llm.Message) (string, error) {
	c.calls++
	return "không rõ", nil
}

func TestContextFilesHTTP_ChatOverCap(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	inner := store.New()
	ag, err := inner.CreateAgent(store.Agent{
		AgentKey: "tu-van-chat-cap", DisplayName: "Cố vấn Minh An",
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inner.CreateSession(store.Session{AgentID: ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	st := httpOverCap{StoreIface: inner}
	model := &callCount{}
	h := NewRouter(Options{Store: st, Version: "t", Runtime: agent.New(st, nil, nil, nil, model)})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(
		`{"session_id":"`+sess.ID+`","message":"Chiết khấu tối đa là bao nhiêu?","prompt_mode":"full"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != `{"error":"too_large"}` {
		t.Fatalf("chat %d %s", rec.Code, rec.Body.String())
	}
	if model.calls != 0 {
		t.Fatal("model ran")
	}
	msgs, err := inner.ListMessages(sess.ID)
	if err != nil || len(msgs) != 0 {
		t.Fatalf("partial turn: %v %d", err, len(msgs))
	}
}

func TestContextFilesHTTP_ViewTokenCannotWrite(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-ctx")
	t.Setenv("GOSO_VIEW_TOKEN", "view-ctx")
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-view", DisplayName: "Cố vấn Minh An",
	})
	if err != nil {
		t.Fatal(err)
	}
	original := "Minh An giu nguyen."
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: "default", Scope: store.ContextScopeAgent, AgentID: ag.ID,
		Name: "AGENTS.md", Body: original,
	}); err != nil {
		t.Fatal(err)
	}
	h := auth.RequireTokens("admin-ctx", "view-ctx", nil)(NewRouter(Options{Store: st, Version: "t"}))
	put := httptest.NewRequest(http.MethodPut, "/api/context-files", strings.NewReader(
		`{"scope":"agent","agent_id":"`+ag.ID+`","name":"AGENTS.md","body":"Minh An bi sua."}`))
	put.Header.Set("Authorization", "Bearer view-ctx")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, put)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("put status %d %s", rec.Code, rec.Body.String())
	}
	del := httptest.NewRequest(http.MethodDelete, "/api/context-files?scope=agent&agent_id="+ag.ID+"&name=AGENTS.md", nil)
	del.Header.Set("Authorization", "Bearer view-ctx")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, del)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("delete status %d %s", rec.Code, rec.Body.String())
	}
	got, err := st.GetContextFile("default", store.ContextScopeAgent, ag.ID, "AGENTS.md")
	if err != nil || got.Body != original {
		t.Fatalf("row changed: %v %+v", err, got)
	}
}
