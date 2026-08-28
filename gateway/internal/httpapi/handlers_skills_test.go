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

func TestGetSkills_EmptyEnvFailClosed(t *testing.T) {
	t.Setenv("GOSO_SKILLS_DIR", "")
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Skills []map[string]any `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Skills == nil || len(listed.Skills) != 0 {
		t.Fatalf("want empty list %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=demo", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "not_configured") {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
}

func TestGetSkills_TempDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo", "SKILL.md"), []byte("# Demo\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"demo"`) {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "# Demo") {
		t.Fatal("list must not include body")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=demo", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "# Demo") {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=../demo", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "path escape") {
		t.Fatalf("escape %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/tools/invoke", bytes.NewBufferString(`{"connector":"builtin","tool":"use_skill","arguments":{"name":"demo"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "# Demo") {
		t.Fatalf("invoke %d %s", w.Code, w.Body.String())
	}
}

func TestSkills_SearchCreateDelete(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "invoices"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "invoices", "SKILL.md"), []byte("---\ndescription: vendor invoices and billing\n---\n# Invoices\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "weather"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "weather", "SKILL.md"), []byte("---\ndescription: rain forecast\n---\n# Weather\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?q=invoices", nil))
	if w.Code != 200 {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	var ranked struct {
		Skills []struct {
			Name    string  `json:"name"`
			Score   float64 `json:"score"`
			Snippet string  `json:"snippet"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ranked); err != nil {
		t.Fatal(err)
	}
	if len(ranked.Skills) == 0 || ranked.Skills[0].Name != "invoices" {
		t.Fatalf("rank %+v", ranked.Skills)
	}
	if ranked.Skills[0].Snippet == "" || ranked.Skills[0].Score <= 0 {
		t.Fatalf("hit %+v", ranked.Skills[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?q=xylophoneuniquezzz", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"skills":[]`) && !strings.Contains(w.Body.String(), `"skills": []`) {
		body := strings.TrimSpace(w.Body.String())
		if w.Code != 200 || (!strings.Contains(body, `"skills":[]`) && !strings.Contains(body, `"skills": []`)) {
			var empty struct {
				Skills []any `json:"skills"`
			}
			if json.Unmarshal(w.Body.Bytes(), &empty) != nil || len(empty.Skills) != 0 {
				t.Fatalf("unrelated %d %s", w.Code, w.Body.String())
			}
		}
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?q=", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"invoices"`) {
		t.Fatalf("empty q keeps list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"score"`) {
		t.Fatal("empty q must be the name list, not ranked hits")
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewBufferString(`{"name":"ledger","body":"---\ndescription: ledger entries\n---\n# Ledger\n"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"name":"ledger"`) {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?q=ledger", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"ledger"`) {
		t.Fatalf("search created %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/skills/ledger", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?name=ledger", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/skills", bytes.NewBufferString(`{"name":"../x","body":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("jail create %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/skills/%2e%2e", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("jail delete %d %s", w.Code, w.Body.String())
	}
}

func TestSkills_EmptyEnvSearchManageFailClosed(t *testing.T) {
	t.Setenv("GOSO_SKILLS_DIR", "")
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/skills?q=invoices", nil))
	if w.Code != 200 {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Skills []any `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Skills == nil || len(listed.Skills) != 0 {
		t.Fatalf("want empty search %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewBufferString(`{"name":"x","body":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if (w.Code != 200 && w.Code != 400) || !strings.Contains(w.Body.String(), "not_configured") {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/skills/x", nil))
	if (w.Code != 200 && w.Code != 400) || !strings.Contains(w.Body.String(), "not_configured") {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
}

func TestChat_SkillSearchToolCalls(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "invoices"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "invoices", "SKILL.md"), []byte("---\ndescription: vendor invoices\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")

	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "skilltool"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "s1", Name: "builtin__skill_search", Arguments: map[string]any{"query": "invoices"}}}},
		{Text: "found invoices skill"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(32), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"find invoice skill"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, tools := range scripted.RecordedTools {
		for _, ts := range tools {
			if ts.Name == "builtin__skill_search" || ts.Name == "skill_search" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("skill_search not advertised %#v", scripted.RecordedTools)
	}
	if !strings.Contains(w.Body.String(), "found invoices skill") {
		t.Fatalf("reply %s", w.Body.String())
	}
	msgs, _ := st.ListMessages(sess.ID)
	sawHit := false
	for _, m := range msgs {
		if m == nil || m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, "invoices") {
			sawHit = true
		}
	}
	if !sawHit {
		t.Fatalf("tool result missing invoices %#v", msgs)
	}
}

func TestChat_SkillSearchEmptyQueryFailClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "invoices"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "invoices", "SKILL.md"), []byte("invoices body"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")

	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "emptyq"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "s1", Name: "builtin__skill_search", Arguments: map[string]any{"query": "  "}}}},
		{Text: "no search"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"search"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	msgs, _ := st.ListMessages(sess.ID)
	sawErr := false
	for _, m := range msgs {
		if m == nil || m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, "query is required") {
			sawErr = true
		}
		if strings.Contains(m.Content, `"name":"invoices"`) && strings.Contains(m.Content, `"score"`) {
			t.Fatalf("empty query must not rank %#v", m)
		}
	}
	if !sawErr {
		t.Fatalf("expected fail-closed error, msgs=%#v", msgs)
	}
}
