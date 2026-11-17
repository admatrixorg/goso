// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package toolsettingsimport

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// A GoClaw-shaped export: a global read_image chain, a tenant override for
// read_image, a tenant read_video chain given as a JSON string with a
// provider_id-only entry, and a non-chain tool.
const exportJSON = `{
 "builtin_tools": [
  {"name":"read_image","settings":{"providers":[{"provider":"openrouter","model":"global-model","enabled":true}]}},
  {"name":"create_image","settings":{"providers":[{"provider":"gemini","model":"gemini-2.5-flash-image","enabled":true,"params":{"aspect_ratio":"1:1"}}]}},
  {"name":"web_search","settings":{"provider":"brave"}}
 ],
 "builtin_tool_tenant_configs": [
  {"tool_name":"read_image","tenant_id":"src-1","enabled":true,"settings":{"providers":[
    {"provider":"gemini","model":"gemini-2.5-flash","enabled":true,"timeout":120,"max_retries":2,"params":{"api_key":"x","max_tokens":2048}},
    {"provider":"openrouter","model":"google/gemini-2.5-flash","enabled":true},
    {"provider":"dashscope","model":"qwen-vl","enabled":true},
    {"provider":"gemini","model":"gemini-2.0-flash"}]}},
  {"tool_name":"read_video","tenant_id":"src-1","enabled":false,"settings":"{\"providers\":[{\"provider_id\":\"p-1\",\"model\":\"gemini-2.5-flash\",\"enabled\":true},{\"provider_id\":\"p-9\",\"enabled\":true}]}"},
  {"tool_name":"read_image","tenant_id":"src-2","enabled":true,"settings":{"providers":[{"provider":"other","enabled":true}]}},
  {"tool_name":"web_fetch","tenant_id":"src-1","enabled":true,"settings":{"max_chars":1000}}
 ],
 "llm_providers": [
  {"id":"p-1","tenant_id":"src-1","name":"gemini"},
  {"id":"p-2","tenant_id":"src-2","name":"other"}
 ]
}`

func importFixture(t *testing.T) (*store.Store, *Export) {
	t.Helper()
	st := store.New()
	for _, p := range []store.LLMProvider{
		{Name: "gem-shop", TenantID: "shop-a", Type: "openai-compat", BaseURL: "https://gemini.example.test/v1beta/openai", Model: "gemini-2.5-flash", Enabled: true},
		{Name: "openrouter", TenantID: "shop-a", Type: "openai-compat", BaseURL: "https://router.example.test/api/v1", Model: "m", Enabled: false},
		{Name: "gem-other", TenantID: "shop-b", Type: "openai-compat", BaseURL: "https://gemini.example.test/v1beta/openai", Model: "m", Enabled: true},
	} {
		if _, err := st.CreateLLMProvider(p); err != nil {
			t.Fatal(err)
		}
	}
	ex, err := ReadExport(strings.NewReader(exportJSON))
	if err != nil {
		t.Fatal(err)
	}
	return st, ex
}

func toolResult(rep *Report, tool string) *ToolResult {
	for i := range rep.Tools {
		if rep.Tools[i].Tool == tool {
			return &rep.Tools[i]
		}
	}
	return nil
}

func TestImport_DryRunMapsAndReports(t *testing.T) {
	st, ex := importFixture(t)
	opt := Options{SourceTenant: "src-1", DestTenant: "shop-a", ProviderMap: map[string]string{"gemini": "gem-shop"}}
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Fatal("dry run expected")
	}
	img := toolResult(rep, "read_image")
	if img == nil || img.Source != "tenant" || img.Action != "insert" {
		t.Fatalf("read_image %+v", img)
	}
	want := []string{"gem-shop/gemini-2.5-flash", "openrouter/google/gemini-2.5-flash", "gem-shop/gemini-2.0-flash (disabled)"}
	if strings.Join(img.Providers, ",") != strings.Join(want, ",") {
		t.Fatalf("providers %v", img.Providers)
	}
	// provider_id resolves through llm_providers; an unknown id is reported.
	vid := toolResult(rep, "read_video")
	if vid == nil || len(vid.Providers) != 1 || vid.Providers[0] != "gem-shop/gemini-2.5-flash" {
		t.Fatalf("read_video %+v", vid)
	}
	// create_image falls back to the global chain.
	if ci := toolResult(rep, "create_image"); ci == nil || ci.Source != "global" || ci.Action != "insert" {
		t.Fatalf("create_image %+v", ci)
	}
	unmapped := map[string]bool{}
	for _, u := range rep.Unmapped {
		unmapped[u.Tool+":"+u.Provider] = true
	}
	if !unmapped["read_image:dashscope"] || !unmapped["read_video:(provider_id only)"] || len(rep.Unmapped) != 2 {
		t.Fatalf("unmapped %+v", rep.Unmapped)
	}
	joined := strings.Join(rep.Warnings, "\n")
	for _, w := range []string{"dropped params.api_key", `"openrouter" is disabled`, "read_video is disabled for the source tenant"} {
		if !strings.Contains(joined, w) {
			t.Fatalf("missing warning %q in %s", w, joined)
		}
	}
	if strings.Contains(joined, "max_tokens") {
		t.Fatal("max_tokens is not a secret")
	}
	ni := strings.Join(rep.NotImported, "\n")
	if !strings.Contains(ni, "web_search") || !strings.Contains(ni, "web_fetch") {
		t.Fatalf("not imported %v", rep.NotImported)
	}
	if rows, _ := st.ListToolSettings("shop-a"); len(rows) != 0 {
		t.Fatalf("dry run wrote %d rows", len(rows))
	}
	raw, _ := json.Marshal(rep)
	if strings.Contains(string(raw), `"x"`) {
		t.Fatalf("report leaks a param value: %s", raw)
	}
}

func TestImport_ApplyExistsAndOverwrite(t *testing.T) {
	st, ex := importFixture(t)
	opt := Options{SourceTenant: "src-1", DestTenant: "shop-a", Apply: true, ProviderMap: map[string]string{"gemini": "gem-shop"}}
	if _, err := Import(st, ex, opt); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetToolSettings("shop-a", "read_image")
	if err != nil {
		t.Fatal(err)
	}
	s, err := builtin.ParseMediaSettings(row.Settings)
	if err != nil {
		t.Fatalf("stored settings do not parse: %v %s", err, row.Settings)
	}
	first := s.Providers[0]
	if len(s.Providers) != 3 || first.Provider != "gem-shop" || first.Timeout != 120 || first.MaxRetries != 2 || first.Params["api_key"] != nil || first.Params["max_tokens"] == nil {
		t.Fatalf("stored %s", row.Settings)
	}
	if s.Providers[2].On() {
		t.Fatal("GoClaw entry without enabled must stay off")
	}
	// The imported chain is live for the tenant.
	builtinPrev := builtin.ProviderStore
	builtin.ProviderStore = st
	t.Cleanup(func() { builtin.ProviderStore = builtinPrev })
	if !builtin.ConfiguredForTenant("shop-a", "read_video") {
		t.Fatal("read_video chain not configured after import")
	}

	// Second run: existing rows are skipped.
	rep, _ := Import(st, ex, opt)
	if r := toolResult(rep, "read_image"); r.Action != "skip" || r.Reason != "exists" {
		t.Fatalf("second run %+v", r)
	}
	opt.Overwrite = true
	rep, _ = Import(st, ex, opt)
	if r := toolResult(rep, "read_image"); r.Action != "update" {
		t.Fatalf("overwrite %+v", r)
	}
}

func TestImport_TenantChecks(t *testing.T) {
	st, ex := importFixture(t)
	// shop-b cannot use shop-a's providers: everything is unmapped.
	rep, err := Import(st, ex, Options{SourceTenant: "src-1", DestTenant: "shop-b", Apply: true, ProviderMap: map[string]string{"gemini": "gem-shop"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Tools {
		if r.Action != "skip" || r.Reason != "no-mapped-providers" {
			t.Fatalf("shop-b %+v", r)
		}
	}
	if rows, _ := st.ListToolSettings("shop-b"); len(rows) != 0 {
		t.Fatalf("shop-b rows %d", len(rows))
	}
	// An env provider declared on the command line counts as present.
	rep, _ = Import(st, ex, Options{SourceTenant: "src-1", DestTenant: "shop-b", ProviderMap: map[string]string{"gemini": "gem-other", "openrouter": "router-env"}, EnvProviders: map[string]struct{}{"router-env": {}}})
	if r := toolResult(rep, "read_image"); r == nil || r.Action != "insert" || len(r.Providers) != 3 || r.Providers[1] != "router-env/google/gemini-2.5-flash" {
		t.Fatalf("env provider %+v", r)
	}
	var cfg *ConfigError
	if _, err := Import(st, ex, Options{SourceTenant: "src-1", DestTenant: "shop-a", Deny: map[string]struct{}{"shop-a": {}}}); !errors.As(err, &cfg) {
		t.Fatalf("deny %v", err)
	}
	if _, err := Import(st, ex, Options{SourceTenant: "", DestTenant: "shop-a"}); !errors.As(err, &cfg) {
		t.Fatalf("missing source %v", err)
	}
}
