// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func instanceRouter(t *testing.T) (http.Handler, store.StoreIface, *channel.Manager) {
	t.Helper()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-ch")
	t.Setenv("GOSO_LITE", "")
	st := store.New()
	mgr := channel.NewManager()
	mgr.Store = st
	return NewRouter(Options{Store: st, Version: "t", Channels: mgr}), st, mgr
}

func tenantCall(t *testing.T, h http.Handler, tenant, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-ch")
	req.Header.Set("X-Goso-Tenant", tenant)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// chPath escapes the instance name into one path segment (each "/" as %2F).
func chPath(name, suffix string) string {
	return "/api/channels/" + url.PathEscape(name) + suffix
}

func listedNames(t *testing.T, w *httptest.ResponseRecorder) map[string]bool {
	t.Helper()
	var body struct {
		Channels []struct {
			Name string `json:"name"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, c := range body.Channels {
		out[c.Name] = true
	}
	return out
}

func TestTenantChannelInstancesAreIsolated(t *testing.T) {
	h, st, _ := instanceRouter(t)
	agentA, _ := st.CreateAgent(store.Agent{AgentKey: "a", DisplayName: "A", TenantID: "tenant-a"})
	agentB, _ := st.CreateAgent(store.Agent{AgentKey: "b", DisplayName: "B", TenantID: "tenant-b"})
	botA, botB := "tenant-a/telegram/shop", "tenant-b/telegram/shop"

	// Each tenant pairs its own bot: token, then agent.
	for _, tc := range []struct{ tenant, bot, agent string }{{"tenant-a", botA, agentA.ID}, {"tenant-b", botB, agentB.ID}} {
		if w := tenantCall(t, h, tc.tenant, "PUT", chPath(tc.bot, "/secrets"), `{"bot_token":"tok-`+tc.tenant+`"}`); w.Code != 200 {
			t.Fatalf("put secret %s: %d %s", tc.bot, w.Code, w.Body.String())
		}
		if w := tenantCall(t, h, tc.tenant, "PATCH", chPath(tc.bot, ""), `{"agent_id":"`+tc.agent+`","dm_policy":"open"}`); w.Code != 200 {
			t.Fatalf("patch %s: %d %s", tc.bot, w.Code, w.Body.String())
		}
	}
	cfg, err := st.GetChannelConfig(botA)
	if err != nil || cfg.Enabled || cfg.AgentID != agentA.ID {
		t.Fatalf("bot A row %+v %v: a new tenant bot must start off", cfg, err)
	}
	if v, err := secrets.Get(st, channel.SecretName(botA, channel.KindBot)); err != nil || string(v) != "tok-tenant-a" {
		t.Fatal("bot A token not stored under its own name")
	}

	// Each tenant sees only its own bot.
	names := listedNames(t, tenantCall(t, h, "tenant-a", "GET", "/api/channels", ""))
	if !names[botA] || names[botB] || !names["telegram"] {
		t.Fatalf("tenant-a list %v", names)
	}

	// Another tenant's bot does not exist for you.
	for _, w := range []*httptest.ResponseRecorder{
		tenantCall(t, h, "tenant-a", "PATCH", chPath(botB, ""), `{"enabled":false}`),
		tenantCall(t, h, "tenant-a", "PUT", chPath(botB, "/secrets"), `{"bot_token":"stolen"}`),
		tenantCall(t, h, "tenant-a", "DELETE", chPath(botB, "/secrets"), ``),
		tenantCall(t, h, "tenant-a", "GET", chPath(botB, "/health"), ``),
		tenantCall(t, h, "tenant-a", "POST", chPath(botB, "/test"), ``),
	} {
		if w.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant call %d %s", w.Code, w.Body.String())
		}
	}
	if v, _ := secrets.Get(st, channel.SecretName(botB, channel.KindBot)); string(v) != "tok-tenant-b" {
		t.Fatal("cross-tenant PUT changed bot B's token")
	}

	// You cannot bind another tenant's agent.
	if w := tenantCall(t, h, "tenant-a", "PATCH", chPath(botA, ""), `{"agent_id":"`+agentB.ID+`"}`); w.Code != http.StatusNotFound {
		t.Fatalf("foreign agent bind %d", w.Code)
	}
	// Enabling needs an agent.
	if w := tenantCall(t, h, "tenant-a", "PATCH", chPath("tenant-a/telegram/empty", ""), `{"enabled":true}`); w.Code != http.StatusBadRequest {
		t.Fatalf("enable without agent %d", w.Code)
	}
	if w := tenantCall(t, h, "tenant-a", "GET", chPath(botA, "/health"), ``); w.Code != 200 || !strings.Contains(w.Body.String(), `"health":"stopped"`) {
		t.Fatalf("health %d %s", w.Code, w.Body.String())
	}
}

func TestTenantBotPairingHiddenFromOtherTenant(t *testing.T) {
	h, st, _ := instanceRouter(t)
	p, err := st.CreateChannelPairing(store.ChannelPairing{Channel: "tenant-b/telegram/shop", SenderID: "99", Status: "pending", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if w := tenantCall(t, h, "tenant-a", "GET", "/api/channel-pairing", ""); strings.Contains(w.Body.String(), p.ID) {
		t.Fatalf("tenant-a sees tenant-b pairing: %s", w.Body.String())
	}
	if w := tenantCall(t, h, "tenant-a", "POST", "/api/channel-pairing/"+p.ID+"/approve", ""); w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant approve %d", w.Code)
	}
	if w := tenantCall(t, h, "tenant-b", "GET", "/api/channel-pairing", ""); !strings.Contains(w.Body.String(), p.ID) {
		t.Fatalf("tenant-b misses its pairing: %s", w.Body.String())
	}
}

func TestTenantTelegramWebhookUnknownBot(t *testing.T) {
	h, _, _ := instanceRouter(t)
	w := tenantCall(t, h, "tenant-a", "POST", "/api/channels/telegram/webhook/tenant-a/nobody", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("webhook for a bot that is not running %d", w.Code)
	}
}

// A ZaloCRM bridge channel is set up with plain settings plus one key.
func TestZCRMBridgeChannelSetup(t *testing.T) {
	h, st, _ := instanceRouter(t)
	agent, _ := st.CreateAgent(store.Agent{AgentKey: "a", DisplayName: "Minh An", TenantID: "tenant-a"})
	inst := "tenant-a/zalo-personal/shop"
	if err := st.PutChannelConfig(store.ChannelConfig{Name: inst, AgentID: agent.ID, Settings: map[string]string{"account_id": "acc-1"}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"settings":{"crm_base_url":"https://crm.example.test","group_enabled":true}}`, 200},
		{`{"settings":{"crm_base_url":"ftp://crm.example.test"}}`, 400},
		{`{"settings":{"bridge_secret":"x"}}`, 400},
	} {
		if w := tenantCall(t, h, "tenant-a", "PATCH", chPath(inst, ""), tc.body); w.Code != tc.code {
			t.Fatalf("patch %s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	if w := tenantCall(t, h, "tenant-a", "PUT", chPath(inst, "/secrets"), `{"bridge_secret":"k"}`); w.Code != 200 {
		t.Fatalf("put bridge key %d %s", w.Code, w.Body.String())
	}
	cfg, _ := st.GetChannelConfig(inst)
	if cfg.Settings["account_id"] != "acc-1" || cfg.Settings["crm_base_url"] != "https://crm.example.test" || cfg.Settings["group_enabled"] != "true" {
		t.Fatalf("settings %v", cfg.Settings)
	}
	if v, err := secrets.Get(st, channel.SecretName(inst, channel.KindBridge)); err != nil || string(v) != "k" {
		t.Fatalf("bridge key %q %v", v, err)
	}
	// Another tenant cannot change it.
	if w := tenantCall(t, h, "tenant-b", "PATCH", chPath(inst, ""), `{"settings":{"crm_base_url":"https://evil.example.test"}}`); w.Code == 200 {
		t.Fatalf("tenant-b patched tenant-a's channel")
	}
	// The key never comes back in the list.
	w := tenantCall(t, h, "tenant-a", "GET", "/api/channels", "")
	if strings.Contains(w.Body.String(), `"k"`) || !strings.Contains(w.Body.String(), "crm.example.test") {
		t.Fatalf("list %s", w.Body.String())
	}
}

// The channels page asks for one page at a time, so the list must honour limit,
// offset and search instead of returning every instance on every call.
func TestChannelInstanceListPages(t *testing.T) {
	h, st, _ := instanceRouter(t)
	for _, slug := range []string{"a1", "a2", "a3", "a4", "a5"} {
		name := "tenant-a/zalo-personal/" + slug
		if err := st.PutChannelConfig(store.ChannelConfig{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	first := tenantCall(t, h, "tenant-a", "GET", "/v1/channel-instances?limit=2&offset=0", "")
	if first.Code != 200 {
		t.Fatalf("first page %d %s", first.Code, first.Body.String())
	}
	var page struct {
		Instances []map[string]any `json:"instances"`
		Total     int              `json:"total"`
		Limit     int              `json:"limit"`
		Offset    int              `json:"offset"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 || len(page.Instances) != 2 || page.Limit != 2 || page.Offset != 0 {
		t.Fatalf("first page %+v", page)
	}
	if got := page.Instances[0]["name"]; got != "tenant-a/zalo-personal/a1" {
		t.Fatalf("first row %v", got)
	}

	last := tenantCall(t, h, "tenant-a", "GET", "/v1/channel-instances?limit=2&offset=4", "")
	var tail struct {
		Instances []map[string]any `json:"instances"`
		Total     int              `json:"total"`
	}
	if err := json.Unmarshal(last.Body.Bytes(), &tail); err != nil {
		t.Fatal(err)
	}
	if tail.Total != 5 || len(tail.Instances) != 1 || tail.Instances[0]["name"] != "tenant-a/zalo-personal/a5" {
		t.Fatalf("last page %+v", tail)
	}

	// A bigger page size must return more rows, which is what makes the size
	// selector on the page do something.
	wide := tenantCall(t, h, "tenant-a", "GET", "/v1/channel-instances?limit=100&offset=0", "")
	var wideBody struct {
		Instances []map[string]any `json:"instances"`
	}
	if err := json.Unmarshal(wide.Body.Bytes(), &wideBody); err != nil {
		t.Fatal(err)
	}
	if len(wideBody.Instances) != 5 {
		t.Fatalf("wide page returned %d rows", len(wideBody.Instances))
	}

	// Search narrows the total, so the page count follows the filter.
	found := tenantCall(t, h, "tenant-a", "GET", "/v1/channel-instances?search=a3", "")
	var hit struct {
		Instances []map[string]any `json:"instances"`
		Total     int              `json:"total"`
	}
	if err := json.Unmarshal(found.Body.Bytes(), &hit); err != nil {
		t.Fatal(err)
	}
	if hit.Total != 1 || len(hit.Instances) != 1 {
		t.Fatalf("search %+v", hit)
	}
}
