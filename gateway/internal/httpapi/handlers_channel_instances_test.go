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
