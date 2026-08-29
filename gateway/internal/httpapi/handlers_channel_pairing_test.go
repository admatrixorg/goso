// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
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
