// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package providerimport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const tenantMinh = "minh-an"

func exportJSON() string {
	return `{"llm_providers":[
{"id":"p1","tenant_id":"minh-an","name":"openai-main","provider_type":"openai","api_base":"https://api.openai.com/v1","api_key":"sk-should-not-leak","enabled":true},
{"id":"p2","tenant_id":"minh-an","name":"local-echo","provider_type":"echo","api_base":"","enabled":true},
{"id":"p3","tenant_id":"minh-an","name":"anth","provider_type":"anthropic","api_base":"https://api.anthropic.com","enabled":false},
{"id":"p4","tenant_id":"other-tenant","name":"theirs","provider_type":"openai","api_base":"https://x.invalid/v1","enabled":true}
]}`
}

func mustRead(t *testing.T, body string) *Export {
	t.Helper()
	ex, err := ReadExport(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func TestDryRunWritesNothing(t *testing.T) {
	st := store.New()
	ex := mustRead(t, exportJSON())
	rep, err := Import(st, ex, Options{SourceTenant: tenantMinh, DestTenant: tenantMinh})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Fatal("dry-run not reported")
	}
	sec := rep.Tenants[0]
	if sec.Input != 4 || sec.Inserted != 3 || sec.Skipped != 1 || sec.Rejected != 0 {
		t.Fatalf("counts %+v", sec)
	}
	if sec.Reasons[reasonTenantMismatch] != 1 {
		t.Fatalf("reasons %+v", sec.Reasons)
	}
	if len(st.ListLLMProviders()) != 0 {
		t.Fatal("dry-run wrote a row")
	}
}

func TestApplyWritesProviders(t *testing.T) {
	st := store.New()
	ex := mustRead(t, exportJSON())
	rep, err := Import(st, ex, Options{SourceTenant: tenantMinh, DestTenant: tenantMinh, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DryRun {
		t.Fatal("apply reported dry-run")
	}
	got := st.ListLLMProviders()
	if len(got) != 3 {
		t.Fatalf("rows %d", len(got))
	}
	byName := map[string]*store.LLMProvider{}
	for _, p := range got {
		byName[p.Name] = p
	}
	if p := byName["openai-main"]; p == nil || p.Type != "openai-compat" || !p.Enabled || p.TenantID != tenantMinh {
		t.Fatalf("openai-main %+v", p)
	}
	if p := byName["local-echo"]; p == nil || p.Type != "echo" {
		t.Fatalf("local-echo %+v", p)
	}
	if p := byName["anth"]; p == nil || p.Type != "anthropic" || p.Enabled {
		t.Fatalf("anth %+v", p)
	}
}

func TestSecondRunSkips(t *testing.T) {
	st := store.New()
	ex := mustRead(t, exportJSON())
	opt := Options{SourceTenant: tenantMinh, DestTenant: tenantMinh, Apply: true}
	if _, err := Import(st, ex, opt); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	sec := rep.Tenants[0]
	if sec.Inserted != 0 || sec.Reasons[reasonNativeExists] != 3 {
		t.Fatalf("second run %+v", sec)
	}
	if len(st.ListLLMProviders()) != 3 {
		t.Fatal("second run added rows")
	}
}

// The api_key in the export must never reach a report or the store.
func TestKeyNeverStoredOrReported(t *testing.T) {
	st := store.New()
	ex := mustRead(t, exportJSON())
	rep, err := Import(st, ex, Options{SourceTenant: tenantMinh, DestTenant: tenantMinh, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-should-not-leak") {
		t.Fatal("report leaked the api key")
	}
	for _, p := range st.ListLLMProviders() {
		if p.BaseURL == "sk-should-not-leak" || p.Model == "sk-should-not-leak" {
			t.Fatal("key stored as a field")
		}
	}
}

func TestDenyStopsBeforeWrite(t *testing.T) {
	st := store.New()
	ex := mustRead(t, exportJSON())
	_, err := Import(st, ex, Options{
		SourceTenant: tenantMinh, DestTenant: tenantMinh, Apply: true,
		Deny: map[string]struct{}{tenantMinh: {}},
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("deny %v", err)
	}
	if len(st.ListLLMProviders()) != 0 {
		t.Fatal("denied run wrote a row")
	}
}

func TestUnknownTypeBecomesOpenAICompat(t *testing.T) {
	name, kind, base, ok := normalize(Row{Name: "x", ProviderType: "zhipu", APIBase: "https://api.zhipu.ai/v4"})
	if !ok || name != "x" || kind != "openai-compat" || base != "https://api.zhipu.ai/v4" {
		t.Fatalf("normalize %q %q %q %v", name, kind, base, ok)
	}
	if _, kind, _, _ := normalize(Row{Name: "y"}); kind != "echo" {
		t.Fatalf("no base url must be echo, got %q", kind)
	}
}

func TestRejectsControlChars(t *testing.T) {
	if _, _, _, ok := normalize(Row{Name: "bad\nname"}); ok {
		t.Fatal("control char in name was accepted")
	}
}

func TestJSONLInput(t *testing.T) {
	body := "{\"tenant_id\":\"minh-an\",\"name\":\"a\",\"provider_type\":\"openai\"}\n" +
		"{\"tenant_id\":\"minh-an\",\"name\":\"b\",\"provider_type\":\"echo\"}\n"
	ex := mustRead(t, body)
	if len(ex.Rows) != 2 {
		t.Fatalf("rows %d", len(ex.Rows))
	}
}
