// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"

	_ "modernc.org/sqlite"
)

func TestWebhookAPI_BearerAndHMAC(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test", Provider: llm.Echo{}})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/webhooks", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, "wh_") || created.HMACKey == "" {
		t.Fatalf("created %+v", created)
	}

	body := []byte(`{"input":"hello hook","mode":"sync"}`)

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("bearer llm %d %s", w.Code, w.Body.String())
	}
	var reply map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if reply["reply"] != "echo: hello hook" {
		t.Fatalf("reply %v", reply)
	}

	sig := webhook.Sign(created.HMACKey, time.Now(), body)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", sig)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("hmac llm %d %s", w.Code, w.Body.String())
	}

	bad := webhook.Sign("not-the-key", time.Now(), body)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", bad)
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("bad hmac %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader([]byte(`{"input":"x","mode":"async"}`)))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	if accepted["id"] == nil || accepted["id"] == "" {
		t.Fatalf("async id %v", accepted)
	}
}

func TestWebhookAPI_ListPublicOnly(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test", Provider: llm.Echo{}})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/webhooks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("empty list %d %s", w.Code, w.Body.String())
	}
	var empty struct {
		Webhooks []webhook.Public `json:"webhooks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Webhooks == nil {
		t.Fatalf("want empty array not null %s", w.Body.String())
	}
	if len(empty.Webhooks) != 0 {
		t.Fatalf("want 0 got %d %s", len(empty.Webhooks), w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/webhooks", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.HMACKey == "" || created.TokenPrefix == "" {
		t.Fatalf("created %+v", created)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/webhooks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	if strings.Contains(raw, created.Token) {
		t.Fatalf("full token in list %s", raw)
	}
	if strings.Contains(raw, created.HMACKey) {
		t.Fatalf("hmac in list %s", raw)
	}
	if strings.Contains(raw, `"token":`) {
		t.Fatalf("token field in list %s", raw)
	}
	if strings.Contains(raw, `"hmac_key"`) {
		t.Fatalf("hmac_key field in list %s", raw)
	}

	var listed struct {
		Webhooks []map[string]any `json:"webhooks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Webhooks) != 1 {
		t.Fatalf("want 1 got %d %s", len(listed.Webhooks), raw)
	}
	row := listed.Webhooks[0]
	if row["id"] != created.ID {
		t.Fatalf("id %v", row)
	}
	if row["token_prefix"] != created.TokenPrefix {
		t.Fatalf("prefix %v want %s", row, created.TokenPrefix)
	}
	if !strings.HasPrefix(created.Token, created.TokenPrefix) {
		t.Fatalf("prefix %q not prefix of token", created.TokenPrefix)
	}
	if _, ok := row["token"]; ok {
		t.Fatal("token field present")
	}
	if _, ok := row["hmac_key"]; ok {
		t.Fatal("hmac_key field present")
	}
	if strings.Contains(raw, created.HMACKey) {
		t.Fatalf("hmac key in list %s", raw)
	}
}

func TestChannelsAPI_ListsSeven(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
	t.Setenv("GOSO_FEISHU_APP_SECRET", "")
	t.Setenv("GOSO_WHATSAPP_ACCESS_TOKEN", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Channels []struct {
			Name       string   `json:"name"`
			Configured bool     `json:"configured"`
			Missing    bool     `json:"missing"`
			Env        string   `json:"env"`
			EnvNames   []string `json:"env_names"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Channels) != 7 {
		t.Fatalf("want 7 got %d %s", len(body.Channels), w.Body.String())
	}
	wantEnv := map[string]string{
		"telegram":      "GOSO_TELEGRAM_BOT_TOKEN",
		"zalo-personal": "GOSO_ZALO_PERSONAL_TOKEN",
		"zalo-oa":       "GOSO_ZALO_OA_ACCESS_TOKEN",
		"discord":       "GOSO_DISCORD_BOT_TOKEN",
		"slack":         "GOSO_SLACK_BOT_TOKEN",
		"feishu":        "GOSO_FEISHU_APP_SECRET",
		"whatsapp":      "GOSO_WHATSAPP_ACCESS_TOKEN",
	}
	names := map[string]bool{}
	for _, c := range body.Channels {
		names[c.Name] = true
		if c.Configured {
			t.Fatalf("%s configured", c.Name)
		}
		if !c.Missing {
			t.Fatalf("%s missing=false with empty env", c.Name)
		}
		if c.Env != wantEnv[c.Name] {
			t.Fatalf("%s env %q want %q", c.Name, c.Env, wantEnv[c.Name])
		}
		if len(c.EnvNames) != 1 || c.EnvNames[0] != wantEnv[c.Name] {
			t.Fatalf("%s env_names %v want [%s]", c.Name, c.EnvNames, wantEnv[c.Name])
		}
	}
	for _, n := range []string{"telegram", "zalo-personal", "zalo-oa", "discord", "slack", "feishu", "whatsapp"} {
		if !names[n] {
			t.Fatalf("missing %s", n)
		}
	}
}

func TestChannelsAPI_JSONOmitsTokenValue(t *testing.T) {
	const leak = "must-not-appear-in-get-body"
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", leak)
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
	t.Setenv("GOSO_FEISHU_APP_SECRET", "")
	t.Setenv("GOSO_WHATSAPP_ACCESS_TOKEN", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	if strings.Contains(raw, leak) {
		t.Fatalf("GET body leaked token value: %s", raw)
	}
	assertNoTokenLikeValues(t, raw)
	var body struct {
		Channels []struct {
			Name       string   `json:"name"`
			Configured bool     `json:"configured"`
			Missing    bool     `json:"missing"`
			Env        string   `json:"env"`
			EnvNames   []string `json:"env_names"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var tg struct {
		Name       string
		Configured bool
		Missing    bool
		Env        string
		EnvNames   []string
	}
	for _, c := range body.Channels {
		if c.Name == "telegram" {
			tg.Name, tg.Configured, tg.Missing, tg.Env, tg.EnvNames = c.Name, c.Configured, c.Missing, c.Env, c.EnvNames
		}
	}
	if !tg.Configured {
		t.Fatal("telegram should be configured when env set")
	}
	if tg.Missing {
		t.Fatal("telegram missing=true when env set")
	}
	if tg.Env != "GOSO_TELEGRAM_BOT_TOKEN" {
		t.Fatalf("telegram env %q", tg.Env)
	}
	if len(tg.EnvNames) != 1 || tg.EnvNames[0] != "GOSO_TELEGRAM_BOT_TOKEN" {
		t.Fatalf("telegram env_names %v", tg.EnvNames)
	}
}

var tokenLikeValue = regexp.MustCompile(`(?i)(xox[bap]-|sk-[A-Za-z0-9]{8,}|ghp_[A-Za-z0-9]+|Bearer [A-Za-z0-9._-]{12,})`)

func assertNoTokenLikeValues(t *testing.T, raw string) {
	t.Helper()
	if tokenLikeValue.MatchString(raw) {
		t.Fatalf("GET body has token-like value: %s", raw)
	}
	for _, k := range []string{`"token":`, `"bot_token":`, `"access_token":`, `"api_key":`, `"app_secret":`, `"hmac_key":`} {
		if strings.Contains(raw, k) {
			t.Fatalf("GET body has secret field %s: %s", k, raw)
		}
	}
}

func TestChannelsAPI_PatchDoesNotWriteSecrets(t *testing.T) {
	const leak = "patch-must-not-store-this-token"
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_MASTER_KEY", strings.Repeat("ab", 32))
	path := filepath.Join(t.TempDir(), "goso.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := NewRouter(Options{Store: st, Version: "test"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/channels/telegram", bytes.NewBufferString(`{"token":"`+leak+`","bot_token":"`+leak+`","secret":"`+leak+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code == http.StatusOK || w.Code == http.StatusCreated {
		t.Fatalf("PATCH must not succeed with secrets: %d %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH secrets status %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), leak) {
		t.Fatalf("PATCH response echoed token: %s", w.Body.String())
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("PATCH", "/api/channels/telegram", bytes.NewBufferString(`{"enabled":true}`))
	req2.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w2, req2)
	if w2.Code == http.StatusOK {
		t.Fatalf("PATCH enabled must not persist (not in catalog): %s", w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, httptest.NewRequest("PATCH", "/api/channels/sms", bytes.NewBufferString(`{"token":"`+leak+`"}`)))
	if w3.Code != http.StatusNotFound {
		t.Fatalf("unknown channel PATCH %d %s", w3.Code, w3.Body.String())
	}

	for _, name := range []string{"telegram", "channel:telegram:token", "GOSO_TELEGRAM_BOT_TOKEN", "channels.telegram.bot_token"} {
		if _, err := st.GetSecret(name); err != store.ErrNotFound {
			t.Fatalf("secret %q written: %v", name, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("secrets rows after PATCH: %d", n)
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%channel%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		var tbl string
		_ = rows.Scan(&tbl)
		t.Fatalf("unexpected channel table %s", tbl)
	}

	gw := httptest.NewRecorder()
	h.ServeHTTP(gw, httptest.NewRequest("GET", "/api/channels", nil))
	if strings.Contains(gw.Body.String(), leak) {
		t.Fatalf("GET leaked PATCH token: %s", gw.Body.String())
	}
	assertNoTokenLikeValues(t, gw.Body.String())
}

func TestChannelsAPI_LiteFlag(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var off struct {
		Lite     bool `json:"lite"`
		Channels []struct {
			Name       string `json:"name"`
			Configured bool   `json:"configured"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &off); err != nil {
		t.Fatal(err)
	}
	if off.Lite {
		t.Fatalf("lite unset should be false: %s", w.Body.String())
	}
	if len(off.Channels) != 7 {
		t.Fatalf("want 7 channels, got %d", len(off.Channels))
	}

	t.Setenv("GOSO_LITE", "1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("lite status %d %s", w.Code, w.Body.String())
	}
	var on struct {
		Lite     bool `json:"lite"`
		Channels []struct {
			Name string `json:"name"`
			Env  string `json:"env"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &on); err != nil {
		t.Fatal(err)
	}
	if !on.Lite {
		t.Fatalf("GOSO_LITE=1 want lite true: %s", w.Body.String())
	}
	if len(on.Channels) != 7 {
		t.Fatalf("lite still lists adapters, got %d", len(on.Channels))
	}
	var hasTG bool
	for _, c := range on.Channels {
		if c.Name == "telegram" {
			hasTG = true
			if c.Env != "GOSO_TELEGRAM_BOT_TOKEN" {
				t.Fatalf("lite telegram env %q", c.Env)
			}
		}
	}
	if !hasTG {
		t.Fatal("lite missing telegram")
	}
}
