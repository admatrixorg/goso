// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPersonalQR_UnconfiguredAndLogout(t *testing.T) {
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "t"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels/zalo-personal/qr", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "imei") || strings.Contains(w.Body.String(), "cookie") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Status != "unconfigured" {
		t.Fatalf("%v %+v", err, body)
	}

	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	if err := secrets.Put(st, channel.SecretName("zalo-personal", "session"), []byte("sess")); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels/zalo-personal/qr", nil))
	if !strings.Contains(w.Body.String(), "confirmed") || strings.Contains(w.Body.String(), "sess") {
		t.Fatalf("confirmed leak? %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/channels/zalo-personal/logout", nil))
	if w.Code != 200 {
		t.Fatalf("logout %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels/zalo-personal/qr", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Status != "unconfigured" {
		t.Fatalf("after logout %s", w.Body.String())
	}
}
