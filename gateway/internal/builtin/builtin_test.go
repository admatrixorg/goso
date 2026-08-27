// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalog_FourTools(t *testing.T) {
	got := Catalog()
	if len(got) != 4 {
		t.Fatalf("len %d", len(got))
	}
	if !IsName("web_search") || !IsName("sandbox") || !IsName("browser") || !IsName("media") {
		t.Fatal("names")
	}
	if Catalog()[0].RequiresApproval {
		t.Fatal("web_search must not require approval")
	}
	if !Catalog()[1].RequiresApproval || !Catalog()[2].RequiresApproval || !Catalog()[3].RequiresApproval {
		t.Fatal("sandbox/browser/media require approval")
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

	for _, name := range []string{ToolWebSearch, ToolSandbox, ToolBrowser, ToolMedia} {
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
