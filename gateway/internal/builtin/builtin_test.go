// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalog_SevenTools(t *testing.T) {
	got := Catalog()
	if len(got) != 7 {
		t.Fatalf("len %d", len(got))
	}
	byName := map[string]Spec{}
	for _, s := range got {
		byName[s.Name] = s
	}
	for _, n := range []string{ToolWebSearch, ToolSandbox, ToolBrowser, ToolMedia, ToolUseSkill, ToolReadFile, ToolWriteFile} {
		if !IsName(n) {
			t.Fatalf("IsName %s", n)
		}
		if _, ok := byName[n]; !ok {
			t.Fatalf("missing %s", n)
		}
	}
	if byName[ToolWebSearch].RequiresApproval || byName[ToolUseSkill].RequiresApproval || byName[ToolReadFile].RequiresApproval {
		t.Fatal("web_search/use_skill/read_file must not require approval")
	}
	if !byName[ToolSandbox].RequiresApproval || !byName[ToolBrowser].RequiresApproval || !byName[ToolMedia].RequiresApproval {
		t.Fatal("sandbox/browser/media require approval")
	}
	if !byName[ToolWriteFile].RequiresApproval {
		t.Fatal("write_file requires approval")
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
	for _, name := range []string{ToolWebSearch, ToolSandbox, ToolBrowser, ToolMedia, ToolUseSkill, ToolReadFile, ToolWriteFile} {
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
	if m["heading"] != "GOSO" || m["abstract"] != "gateway" {
		t.Fatalf("content %+v", m)
	}
}

func TestInvoke_SandboxNeverSpawns(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "ddg")
	res, err := Invoke(context.Background(), ToolSandbox, map[string]any{"cmd": "true"}, true)
	if err != nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
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
