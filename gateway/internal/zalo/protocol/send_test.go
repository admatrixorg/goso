package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestSendMessageShape drives a local server so the payload encryption, the form
// encoding and the reply decryption are proven without a live Zalo account.
func TestSendMessageShape(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	secret := encodeSecretForTest(key)
	var gotParams string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotParams = r.PostFormValue("params")
		gotQuery = r.URL.Query()
		// Reply is itself an encrypted string inside data.
		reply, _ := EncodeAESCBC(key, "msg-12345", false)
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": reply})
	}))
	defer srv.Close()

	cred, err := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"x"}],"imei":"imei-qc","secret_key":"` + secret + `"}`)
	if err != nil {
		t.Fatalf("cred: %v", err)
	}
	sess, err := NewSessionWithCredential(cred)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess.Services = &ServiceMap{Chat: []string{srv.URL}}
	sess.HTTP = &http.Client{Jar: sess.Jar}

	res, err := sess.SendMessage(context.Background(), "123456789", ThreadUser, "hello from qc")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !res.OK || res.MessageID != "msg-12345" {
		t.Fatalf("result %#v", res)
	}
	if gotParams == "" {
		t.Fatal("server saw no params field")
	}
	plain, derr := DecodeAESCBC(key, gotParams)
	if derr != nil {
		t.Fatalf("payload was not AES-CBC under the secret key: %v", derr)
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["message"] != "hello from qc" || payload["toid"] != "123456789" || payload["imei"] != "imei-qc" {
		t.Fatalf("payload %#v", payload)
	}
	if gotQuery.Get("zpw_ver") == "" || gotQuery.Get("zpw_type") == "" || gotQuery.Get("nretry") != "0" {
		t.Fatalf("query %#v", gotQuery)
	}
}

func TestSendMessageRejectsBadInput(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	cred, _ := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"x"}],"secret_key":"` + encodeSecretForTest(key) + `"}`)
	sess, _ := NewSessionWithCredential(cred)
	if _, err := sess.SendMessage(context.Background(), "", ThreadUser, "hi"); err == nil {
		t.Fatal("expected a missing thread id error")
	}
	if _, err := sess.SendMessage(context.Background(), "1", ThreadUser, "  "); err == nil {
		t.Fatal("expected a missing text error")
	}
	sess.Services = &ServiceMap{}
	if _, err := sess.SendMessage(context.Background(), "1", ThreadUser, "hi"); err == nil || !strings.Contains(err.Error(), "service url") {
		t.Fatalf("expected a service url error, got %v", err)
	}
}

func TestSendMessageRejectedEnvelope(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 114, "error_message": "session expired"})
	}))
	defer srv.Close()
	cred, _ := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"x"}],"secret_key":"` + encodeSecretForTest(key) + `"}`)
	sess, _ := NewSessionWithCredential(cred)
	sess.Services = &ServiceMap{Chat: []string{srv.URL}}
	sess.HTTP = &http.Client{Jar: sess.Jar}
	if _, err := sess.SendMessage(context.Background(), "1", ThreadUser, "hi"); err == nil {
		t.Fatal("expected the rejected envelope to fail")
	}
}

func encodeSecretForTest(key []byte) string {
	return base64Of(key)
}
