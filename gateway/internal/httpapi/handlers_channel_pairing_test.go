// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func channelPairingServer(t *testing.T) (http.Handler, store.StoreIface) {
	t.Helper()
	t.Setenv("GOSO_VIEW_TOKEN", "view-084")
	st := store.New()
	p := auth.NewPairing()
	inner := NewRouter(Options{Store: st, Version: "t", Pairing: p})
	h := auth.Require(auth.Config{Admin: "admin-084", View: "view-084", Pairing: p, Bypass: []string{"/healthz"}})(inner)
	return h, st
}

func TestChannelPairing_HTTPApproveDeny(t *testing.T) {
	h, st := channelPairingServer(t)
	now := time.Now().UTC()
	issued, err := channel.IssuePairing(st, "telegram", "u1", now)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/channel-pairing", nil)
	req.Header.Set("Authorization", "Bearer admin-084")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), issued.Code) || strings.Contains(strings.ToLower(w.Body.String()), "code_hash") {
		t.Fatalf("leaked code %s", w.Body.String())
	}
	var list struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Items) != 1 {
		t.Fatalf("list parse %v %+v", err, list)
	}

	ap := httptest.NewRequest("POST", "/api/channel-pairing/"+issued.ID+"/approve", nil)
	ap.Header.Set("Authorization", "Bearer admin-084")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, ap)
	if w.Code != 200 {
		t.Fatalf("approve %d %s", w.Code, w.Body.String())
	}
	if !channel.SenderPaired(st, "telegram", "u1", now) {
		t.Fatal("not paired")
	}

	den := httptest.NewRequest("POST", "/v1/channel-pairing/nope/deny", nil)
	den.Header.Set("Authorization", "Bearer admin-084")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, den)
	if w.Code != 404 {
		t.Fatalf("deny missing %d", w.Code)
	}
}

func TestChannelPairing_HTTPExpiredConflict(t *testing.T) {
	h, st := channelPairingServer(t)
	now := time.Now().UTC()
	row, err := st.CreateChannelPairing(store.ChannelPairing{
		Channel: "telegram", SenderID: "u9", CodeHash: "x", Status: "pending",
		ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/channel-pairing/"+row.ID+"/approve", nil)
	req.Header.Set("Authorization", "Bearer admin-084")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatalf("expired %d %s", w.Code, w.Body.String())
	}
}

func TestChannelPairing_HTTPViewForbidden(t *testing.T) {
	h, _ := channelPairingServer(t)
	req := httptest.NewRequest("GET", "/api/channel-pairing", nil)
	req.Header.Set("Authorization", "Bearer view-084")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("view GET %d", w.Code)
	}
	ap := httptest.NewRequest("POST", "/api/channel-pairing/x/approve", nil)
	ap.Header.Set("Authorization", "Bearer view-084")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, ap)
	if w.Code != 403 {
		t.Fatalf("view approve %d", w.Code)
	}
}

func TestTelegramWebhook_CreatesPendingOnGET(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	t.Setenv("GOSO_LITE", "")
	st := store.New()
	tg := &channel.Telegram{Store: st, Sender: func(context.Context, int64, string) error { return nil }}
	h := NewRouter(Options{Store: st, Version: "t", TG: tg.HandleUpdate})

	body := `{"message":{"chat":{"id":42},"from":{"id":777},"text":"hello"}}`
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("webhook %d %s", w.Code, w.Body.String())
	}

	list := httptest.NewRequest("GET", "/api/channel-pairing", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, list)
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "code_hash") || strings.Contains(w.Body.String(), `"code":`) {
		t.Fatalf("leaked %s", w.Body.String())
	}
	var out struct {
		Items []struct {
			Channel  string `json:"channel"`
			SenderID string `json:"sender_id"`
			Status   string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Channel != "telegram" || out.Items[0].SenderID != "777" || out.Items[0].Status != "pending" {
		t.Fatalf("items %+v %s", out.Items, w.Body.String())
	}
}

func TestListChannels_MergedDefaultPolicy(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Channels []struct {
			Name        string `json:"name"`
			DMPolicy    string `json:"dm_policy"`
			GroupPolicy string `json:"group_policy"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var tg, oa struct{ DM, Group string }
	for _, c := range body.Channels {
		switch c.Name {
		case "telegram":
			tg.DM, tg.Group = c.DMPolicy, c.GroupPolicy
		case "zalo-oa":
			oa.DM, oa.Group = c.DMPolicy, c.GroupPolicy
		}
	}
	if tg.DM != "open" || tg.Group != "allowlist" {
		t.Fatalf("demo telegram %+v", tg)
	}
	if oa.DM != "pairing" {
		t.Fatalf("oa %+v", oa)
	}
}

func TestChannelPairing_ExchangeDoesNotApprove(t *testing.T) {
	h, st := channelPairingServer(t)
	now := time.Now().UTC()
	issued, err := channel.IssuePairing(st, "telegram", "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/pairing/exchange", bytes.NewBufferString(`{"code":"`+issued.Code+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatalf("077 exchange must not accept channel pairing code: %s", w.Body.String())
	}
	if channel.SenderPaired(st, "telegram", "u1", now) {
		t.Fatal("channel sender must not be approved via 077 exchange")
	}
}
