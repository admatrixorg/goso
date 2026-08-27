// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestWhatsApp_CloudAPIHandleUpdate(t *testing.T) {
	st := store.New()
	var sentDest, sentText string
	wa := &WhatsApp{
		Store: st, LLM: llm.Echo{},
		Sender: func(_ context.Context, to, text string) error {
			sentDest, sentText = to, text
			return nil
		},
	}
	// Fixture: WhatsApp Cloud API webhook (Business, not native — DI-01).
	body, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{
			map[string]any{
				"changes": []any{
					map[string]any{
						"value": map[string]any{
							"messaging_product": "whatsapp",
							"metadata":          map[string]any{"phone_number_id": "pn1"},
							"messages": []any{
								map[string]any{
									"from": "15551234567",
									"type": "text",
									"text": map[string]any{"body": "hello wa"},
								},
							},
						},
					},
				},
			},
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/whatsapp/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	wa.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentDest != "15551234567" || sentText != "echo: hello wa" {
		t.Fatalf("sent %q %q", sentDest, sentText)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sessions[0].Label != "whatsapp:15551234567" {
		t.Fatalf("sessions %v", sessions)
	}
}

func TestWhatsApp_IgnoreEmpty(t *testing.T) {
	st := store.New()
	wa := &WhatsApp{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	req := httptest.NewRequest("POST", "/api/channels/whatsapp/webhook", bytes.NewReader([]byte(`{"object":"whatsapp_business_account","entry":[]}`)))
	w := httptest.NewRecorder()
	wa.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 0 {
		t.Fatalf("expected ignore")
	}
}

func TestWhatsApp_SendHttptest(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.1"}]}`))
	}))
	t.Cleanup(srv.Close)
	wa := &WhatsApp{
		AccessToken: "test-placeholder", PhoneNumberID: "pn1",
		APIBase: srv.URL, HTTPClient: srv.Client(),
	}
	if err := wa.sendMessage(context.Background(), "1555", "hi"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer test-placeholder" || gotPath != "/pn1/messages" {
		t.Fatalf("auth %q path %q", gotAuth, gotPath)
	}
	if !bytes.Contains([]byte(gotBody), []byte(`"messaging_product":"whatsapp"`)) {
		t.Fatalf("body %s", gotBody)
	}
}
