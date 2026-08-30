// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tts"
)

func ttsServer(t *testing.T) (*tts.Service, *auditlog.Store, http.Handler) {
	t.Helper()
	for _, k := range []string{
		"GOSO_TTS_PROVIDER", "GOSO_TTS_API_KEY", "GOSO_TTS_VOICE", "GOSO_TTS_MODEL",
		"GOSO_TTS_LANGUAGE", "GOSO_TTS_REGION", "GOSO_TTS_ENDPOINT", "GOSO_TTS_AUTO_APPLY",
		"GOSO_TTS_MAX_CHARS", "GOSO_TTS_TIMEOUT_MS", "GOSO_TTS_ENABLED",
	} {
		t.Setenv(k, "")
	}
	svc := tts.New()
	al := auditlog.New(64)
	h := NewRouter(Options{Store: store.New(), Version: "t", TTS: svc, Audit: al})
	return svc, al, h
}

func ttsJSON(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func assertNoTTSSecrets(t *testing.T, body, secret string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, n := range []string{`"api_key"`, `"authorization"`, `"secret"`, `"token"`, `"password"`} {
		if strings.Contains(lower, n) {
			t.Fatalf("secret field in body: %s", body)
		}
	}
	if secret != "" && strings.Contains(body, secret) {
		t.Fatalf("plaintext secret in body: %s", body)
	}
	if strings.Contains(body, "sk-live") || strings.Contains(body, "Bearer sk-") {
		t.Fatalf("token shape in body: %s", body)
	}
}

func TestTTS_GETOmitsKeyAndV1Alias(t *testing.T) {
	_, _, h := ttsServer(t)
	w := ttsJSON(t, h, "GET", "/api/tts", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	assertNoTTSSecrets(t, w.Body.String(), "")
	w2 := ttsJSON(t, h, "GET", "/v1/tts", "", "")
	if w2.Code != 200 || w2.Body.String() != w.Body.String() {
		t.Fatalf("v1 %d %s vs %s", w2.Code, w2.Body.String(), w.Body.String())
	}
}

func TestTTS_PutNeverEchoesKey(t *testing.T) {
	_, al, h := ttsServer(t)
	key := "sk-liveSECRET99"
	w := ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"openai","enabled":true,"api_key":"`+key+`","voice":"alloy","auto_apply":"reply","max_chars":512,"timeout_ms":8000}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"key_set":true`) || !strings.Contains(w.Body.String(), `"configured":true`) {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	assertNoTTSSecrets(t, w.Body.String(), key)
	w = ttsJSON(t, h, "GET", "/api/tts", "", "")
	assertNoTTSSecrets(t, w.Body.String(), key)
	if !strings.Contains(w.Body.String(), `"auto_apply":"reply"`) {
		t.Fatalf("policy %s", w.Body.String())
	}
	page := al.Query(auditlog.Query{Entity: "tts"})
	if len(page.Records) == 0 {
		t.Fatal("audit")
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), key) {
		t.Fatalf("audit leaked %s", raw)
	}
}

func TestTTS_EmptyPutKeepsKey(t *testing.T) {
	_, _, h := ttsServer(t)
	w := ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"openai","enabled":true,"api_key":"sk-keepKEY0001"}`, "")
	if w.Code != 200 {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"openai","enabled":true,"voice":"nova","auto_apply":"off"}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"key_set":true`) || !strings.Contains(w.Body.String(), `"voice":"nova"`) {
		t.Fatalf("keep %d %s", w.Code, w.Body.String())
	}
	assertNoTTSSecrets(t, w.Body.String(), "sk-keepKEY0001")
}

func TestTTS_TestFailureRedactsAuth(t *testing.T) {
	svc, _, h := ttsServer(t)
	leak := "sk-LEAKEDKEY99"
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusUnauthorized)
		_, _ = rw.Write([]byte(`{"authorization":"Bearer ` + leak + `","api_key":"` + leak + `"}`))
	}))
	t.Cleanup(srv.Close)
	svc.Client = srv.Client()
	body := `{"provider":"openai","enabled":true,"api_key":"` + leak + `","endpoint":` + jsonQuote(srv.URL) + `}`
	w := ttsJSON(t, h, "PUT", "/api/tts", body, "")
	if w.Code != 200 {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "POST", "/api/tts/test", `{}`, "")
	if w.Code != 400 {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	assertNoTTSSecrets(t, w.Body.String(), leak)
	if !strings.Contains(w.Body.String(), `"ok":false`) {
		t.Fatalf("test body %s", w.Body.String())
	}
}

func TestTTS_LocalTestClearConfirm(t *testing.T) {
	_, _, h := ttsServer(t)
	w := ttsJSON(t, h, "POST", "/api/tts/test", `{}`, "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "not_configured") {
		t.Fatalf("empty test %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"edge","enabled":true,"voice":"en","auto_apply":"all"}`, "")
	if w.Code != 200 {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "POST", "/v1/tts/test", `{}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"local"`) {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "POST", "/api/tts/clear", `{"confirm":"nope"}`, "")
	if w.Code != 400 {
		t.Fatalf("bad confirm %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "POST", "/api/tts/clear", `{"confirm":"tts"}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
}

func TestTTS_EnvOwnedConflict(t *testing.T) {
	for _, k := range []string{
		"GOSO_TTS_PROVIDER", "GOSO_TTS_API_KEY", "GOSO_TTS_VOICE", "GOSO_TTS_MODEL",
		"GOSO_TTS_LANGUAGE", "GOSO_TTS_REGION", "GOSO_TTS_ENDPOINT", "GOSO_TTS_AUTO_APPLY",
		"GOSO_TTS_MAX_CHARS", "GOSO_TTS_TIMEOUT_MS", "GOSO_TTS_ENABLED",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("GOSO_TTS_PROVIDER", "openai")
	t.Setenv("GOSO_TTS_API_KEY", "sk-envKEY0001")
	svc := tts.New()
	h := NewRouter(Options{Store: store.New(), Version: "t", TTS: svc})
	w := ttsJSON(t, h, "GET", "/api/tts", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"env_owned":true`) {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	assertNoTTSSecrets(t, w.Body.String(), "sk-envKEY0001")
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"openai","api_key":"sk-newKEY0001"}`, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("put env %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"openai","endpoint":"http://127.0.0.1:9"}`, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "127.0.0.1:9") {
		t.Fatalf("put endpoint ignored %d %s", w.Code, w.Body.String())
	}
}

func TestTTS_ViewTokenGETOnly(t *testing.T) {
	_, _, inner := ttsServer(t)
	h := auth.RequireTokens("admin-118", "view-118", []string{"/healthz"})(inner)
	w := ttsJSON(t, h, "GET", "/api/tts", "", "view-118")
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "GET", "/v1/tts", "", "view-118")
	if w.Code != 200 {
		t.Fatalf("view v1 %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"edge"}`, "view-118")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view put %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "POST", "/api/tts/test", `{}`, "view-118")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view test %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "POST", "/api/tts/clear", `{"confirm":"tts"}`, "view-118")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view clear %d %s", w.Code, w.Body.String())
	}
}

func TestTTS_IssuedReadWrite(t *testing.T) {
	keys := apikey.New()
	svc := tts.New()
	inner := NewRouter(Options{Store: store.New(), Version: "t", TTS: svc, APIKeys: keys})
	h := auth.Require(auth.Config{Admin: "admin-118", Keys: keys})(inner)

	w := ttsJSON(t, h, "POST", "/api/api-keys", `{"name":"reader","scopes":["read"]}`, "admin-118")
	if w.Code != 201 {
		t.Fatalf("create read %d %s", w.Code, w.Body.String())
	}
	var reader struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reader)
	w = ttsJSON(t, h, "GET", "/api/tts", "", reader.Secret)
	if w.Code != 200 {
		t.Fatalf("read GET %d %s", w.Code, w.Body.String())
	}
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"edge"}`, reader.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read put %d %s", w.Code, w.Body.String())
	}

	w = ttsJSON(t, h, "POST", "/api/api-keys", `{"name":"writer","scopes":["write"]}`, "admin-118")
	if w.Code != 201 {
		t.Fatalf("create write %d %s", w.Code, w.Body.String())
	}
	var writer struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &writer)
	w = ttsJSON(t, h, "PUT", "/api/tts", `{"provider":"edge","enabled":true,"voice":"en"}`, writer.Secret)
	if w.Code != 200 {
		t.Fatalf("write put %d %s", w.Code, w.Body.String())
	}
	assertNoTTSSecrets(t, w.Body.String(), writer.Secret)
}
