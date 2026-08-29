// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/connector"
)

func TestCatalog_Tools(t *testing.T) {
	got := Catalog()
	if len(got) != 16 {
		t.Fatalf("len %d", len(got))
	}
	byName := map[string]Spec{}
	for _, s := range got {
		byName[s.Name] = s
	}
	want := []string{
		ToolWebSearch, ToolWebFetch, ToolSandbox, ToolBrowser, ToolMedia, ToolImageGen, ToolTTS,
		ToolUseSkill, ToolSkillSearch, ToolReadFile, ToolWriteFile, ToolListFiles, ToolEdit, ToolSendFile,
		ToolSearch, ToolGlob,
	}
	for _, n := range want {
		if !IsName(n) {
			t.Fatalf("IsName %s", n)
		}
		if _, ok := byName[n]; !ok {
			t.Fatalf("missing %s", n)
		}
	}
	if byName[ToolWebSearch].RequiresApproval || byName[ToolWebFetch].RequiresApproval || byName[ToolUseSkill].RequiresApproval || byName[ToolSkillSearch].RequiresApproval || byName[ToolReadFile].RequiresApproval {
		t.Fatal("web_search/web_fetch/use_skill/skill_search/read_file must not require approval")
	}
	if byName[ToolListFiles].RequiresApproval || byName[ToolSendFile].RequiresApproval || byName[ToolSearch].RequiresApproval || byName[ToolGlob].RequiresApproval {
		t.Fatal("list_files/send_file/search/glob must not require approval")
	}
	if !byName[ToolSandbox].RequiresApproval || !byName[ToolBrowser].RequiresApproval || !byName[ToolMedia].RequiresApproval {
		t.Fatal("sandbox/browser/media require approval")
	}
	if !byName[ToolImageGen].RequiresApproval || !byName[ToolTTS].RequiresApproval {
		t.Fatal("image_gen/tts require approval")
	}
	if !byName[ToolWriteFile].RequiresApproval || !byName[ToolEdit].RequiresApproval {
		t.Fatal("write_file/edit require approval")
	}
}

func TestInvoke_UnconfiguredNoNetwork(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "")
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		t.Error("must not network when unconfigured")
	}))
	defer srv.Close()
	prev := InstantAnswerBase
	InstantAnswerBase = srv.URL
	defer func() { InstantAnswerBase = prev }()

	t.Setenv("GOSO_SKILLS_DIR", "")
	t.Setenv("GOSO_WORKSPACE", "")
	t.Setenv("GOSO_MEDIA", "")
	for _, name := range []string{
		ToolWebSearch, ToolSandbox, ToolBrowser, ToolMedia, ToolImageGen, ToolTTS,
		ToolUseSkill, ToolSkillSearch, ToolReadFile, ToolWriteFile, ToolListFiles, ToolEdit, ToolSendFile,
		ToolSearch, ToolGlob,
	} {
		res, err := Invoke(context.Background(), name, map[string]any{"q": "goso"}, false)
		if err != nil {
			t.Fatalf("%s err %v", name, err)
		}
		if res == nil || res.Status != "not_configured" {
			t.Fatalf("%s %+v", name, res)
		}
	}
	if hit != 0 {
		t.Fatalf("network hits %d", hit)
	}
}

func TestInvoke_WebSearchUIOnEnvOff(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "")
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
	}))
	defer srv.Close()
	prev := InstantAnswerBase
	InstantAnswerBase = srv.URL
	defer func() { InstantAnswerBase = prev }()
	res, err := Invoke(context.Background(), ToolWebSearch, map[string]any{"q": "x"}, true)
	if err != nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if hit != 0 {
		t.Fatal("env off must not hit DDG")
	}
}

func TestInvoke_WebSearchDDGHttptest(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "ddg")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "goso" {
			t.Errorf("q=%s", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("format=%s", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Heading":      "GOSO",
			"AbstractText": "gateway",
			"Answer":       "",
			"RelatedTopics": []map[string]string{
				{"Text": "one"},
			},
		})
	}))
	defer srv.Close()
	prev, prevC := InstantAnswerBase, InstantAnswerClient
	InstantAnswerBase = srv.URL
	InstantAnswerClient = srv.Client()
	defer func() {
		InstantAnswerBase = prev
		InstantAnswerClient = prevC
	}()
	res, err := Invoke(context.Background(), ToolWebSearch, map[string]any{"q": "goso"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ok" {
		t.Fatalf("status %s", res.Status)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["results"].([]map[string]any)
	if len(hits) == 0 {
		t.Fatalf("results %+v", m)
	}
	if hits[0]["title"] != "GOSO" || hits[0]["snippet"] != "gateway" {
		t.Fatalf("hit %+v", hits[0])
	}
}

func TestInvoke_WebSearchEmptyQueryNoNetwork(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "ddg")
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
	}))
	defer srv.Close()
	prev, prevC := InstantAnswerBase, InstantAnswerClient
	InstantAnswerBase = srv.URL
	InstantAnswerClient = srv.Client()
	defer func() {
		InstantAnswerBase = prev
		InstantAnswerClient = prevC
	}()
	res, err := Invoke(context.Background(), ToolWebSearch, map[string]any{"q": "  "}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if hit != 0 {
		t.Fatal("empty query must not network")
	}
}

func TestInvoke_WebSearchEmptyBaseNoNetwork(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "1")
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
	}))
	defer srv.Close()
	prev, prevC := InstantAnswerBase, InstantAnswerClient
	InstantAnswerBase = ""
	InstantAnswerClient = srv.Client()
	defer func() {
		InstantAnswerBase = prev
		InstantAnswerClient = prevC
	}()
	res, err := Invoke(context.Background(), ToolWebSearch, map[string]any{"q": "goso"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	if hit != 0 {
		t.Fatal("empty base must not network")
	}
}

func TestInvoke_WebSearchEmptyJSON(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "ddg")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	prev, prevC := InstantAnswerBase, InstantAnswerClient
	InstantAnswerBase = srv.URL
	InstantAnswerClient = srv.Client()
	defer func() {
		InstantAnswerBase = prev
		InstantAnswerClient = prevC
	}()
	res, err := Invoke(context.Background(), ToolWebSearch, map[string]any{"q": "goso"}, true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	hits, _ := m["results"].([]map[string]any)
	if len(hits) != 0 {
		t.Fatalf("want empty list %+v", m)
	}
}

func TestInvoke_SandboxNeverSpawns(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "ddg")
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolBrowser, map[string]any{"url": "https://example.com"}, true)
	if err != nil || res.Status != "not_configured" {
		t.Fatalf("browser %v %+v", err, res)
	}
}

func TestInvoke_MediaFailClosedUnlessDouble(t *testing.T) {
	t.Cleanup(func() { MediaInvoke = nil })
	MediaInvoke = nil
	t.Setenv("GOSO_MEDIA", "1")
	for _, name := range []string{ToolMedia, ToolImageGen, ToolTTS} {
		res, err := Invoke(context.Background(), name, map[string]any{"prompt": "x"}, true)
		if err != nil || res == nil || res.Status != "not_configured" {
			t.Fatalf("%s env-only %v %+v", name, err, res)
		}
		m, _ := res.Content.(map[string]any)
		if m["error"] != "not_configured" {
			t.Fatalf("%s public error %+v", name, m)
		}
	}
	t.Setenv("GOSO_MEDIA", "")
	called := 0
	MediaInvoke = func(ctx context.Context, name string, args map[string]any) (*connector.InvokeResult, error) {
		called++
		return &connector.InvokeResult{Tool: name, Connector: ConnectorName, Status: "ok", Content: map[string]any{"ok": true}}, nil
	}
	res, err := Invoke(context.Background(), ToolImageGen, map[string]any{"prompt": "x"}, true)
	if err != nil || res.Status != "not_configured" {
		t.Fatalf("double without env %v %+v", err, res)
	}
	if called != 0 {
		t.Fatal("must not invoke double without env")
	}
	t.Setenv("GOSO_MEDIA_IMAGE", "1")
	res, err = Invoke(context.Background(), ToolImageGen, map[string]any{"prompt": "x"}, true)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("double+env %v %+v", err, res)
	}
	if called != 1 {
		t.Fatalf("double calls %d", called)
	}
}

func TestInvoke_UseSkillTempDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo", "SKILL.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	res, err := Invoke(context.Background(), ToolUseSkill, map[string]any{"name": "demo"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Status != "ok" {
		t.Fatalf("%+v", res)
	}
	m, _ := res.Content.(map[string]any)
	if m["body"] != "# hi" || m["name"] != "demo" {
		t.Fatalf("content %+v", m)
	}
	res, err = Invoke(context.Background(), ToolUseSkill, map[string]any{"name": "../demo"}, false)
	if err != nil || res.Status != "error" {
		t.Fatalf("escape %v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolUseSkill, map[string]any{"name": "missing"}, false)
	if err != nil || res.Status != "not_found" {
		t.Fatalf("missing %v %+v", err, res)
	}
}

func TestInvoke_UseSkillEmptyEnv(t *testing.T) {
	t.Setenv("GOSO_SKILLS_DIR", "")
	res, err := Invoke(context.Background(), ToolUseSkill, map[string]any{"name": "demo"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	res, err = Invoke(context.Background(), ToolSkillSearch, map[string]any{"query": "demo"}, true)
	if err != nil || res == nil || res.Status != "not_configured" {
		t.Fatalf("skill_search %v %+v", err, res)
	}
}

func TestInvoke_SkillSearchTempDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "invoices"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "invoices", "SKILL.md"), []byte("---\ndescription: vendor invoices\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "weather"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "weather", "SKILL.md"), []byte("---\ndescription: rain forecast\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	res, err := Invoke(context.Background(), ToolSkillSearch, map[string]any{"query": "invoices"}, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	raw, _ := json.Marshal(res.Content)
	if !strings.Contains(string(raw), `"name":"invoices"`) {
		t.Fatalf("hits %s", raw)
	}
	res, err = Invoke(context.Background(), ToolSkillSearch, map[string]any{"query": "  "}, false)
	if err != nil || res == nil || res.Status != "error" {
		t.Fatalf("empty q %v %+v", err, res)
	}
	m, _ := res.Content.(map[string]any)
	if m["error"] != "query is required" {
		t.Fatalf("empty q content %+v", m)
	}
}

func TestWebSearchNetworkAllowed(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "")
	if WebSearchNetworkAllowed() {
		t.Fatal("empty")
	}
	t.Setenv("GOSO_WEB_SEARCH", "ddg")
	if !WebSearchNetworkAllowed() {
		t.Fatal("ddg")
	}
	t.Setenv("GOSO_WEB_SEARCH", "1")
	if !WebSearchNetworkAllowed() {
		t.Fatal("1")
	}
}
