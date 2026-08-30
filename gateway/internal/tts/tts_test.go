// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clearTTSEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GOSO_TTS_PROVIDER", "GOSO_TTS_API_KEY", "GOSO_TTS_VOICE", "GOSO_TTS_MODEL",
		"GOSO_TTS_LANGUAGE", "GOSO_TTS_REGION", "GOSO_TTS_ENDPOINT", "GOSO_TTS_AUTO_APPLY",
		"GOSO_TTS_MAX_CHARS", "GOSO_TTS_TIMEOUT_MS", "GOSO_TTS_ENABLED",
	} {
		t.Setenv(k, "")
	}
}

func TestPublicOmitsKey(t *testing.T) {
	clearTTSEnv(t)
	s := New()
	on := true
	pub, err := s.Put(Write{Provider: ProviderOpenAI, Enabled: &on, APIKey: "sk-liveSECRET99", Voice: "alloy", AutoApply: ApplyReply, MaxChars: 512, TimeoutMS: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if !pub.KeySet || !pub.Configured || pub.Provider != ProviderOpenAI || pub.AutoApply != ApplyReply {
		t.Fatalf("pub %+v", pub)
	}
	raw, _ := json.Marshal(pub)
	if strings.Contains(string(raw), "sk-live") || strings.Contains(string(raw), "api_key") {
		t.Fatalf("leaked %s", raw)
	}
	if _, ok := AsPublicJSON(pub); !ok {
		t.Fatal("public refused")
	}
}

func TestEmptyPutKeepsKey(t *testing.T) {
	clearTTSEnv(t)
	s := New()
	on := true
	if _, err := s.Put(Write{Provider: ProviderOpenAI, Enabled: &on, APIKey: "sk-keepKEY0001"}); err != nil {
		t.Fatal(err)
	}
	pub, err := s.Put(Write{Provider: ProviderOpenAI, Enabled: &on, Voice: "nova", AutoApply: ApplyOff})
	if err != nil {
		t.Fatal(err)
	}
	if !pub.KeySet || pub.Voice != "nova" {
		t.Fatalf("keep %+v", pub)
	}
}

func TestEnvOwnedRefusesKeyPut(t *testing.T) {
	clearTTSEnv(t)
	t.Setenv("GOSO_TTS_PROVIDER", "openai")
	t.Setenv("GOSO_TTS_API_KEY", "sk-envKEY0001")
	s := New()
	pub := s.Public()
	if !pub.EnvOwned || !pub.KeySet || pub.Source != "env" {
		t.Fatalf("env pub %+v", pub)
	}
	_, err := s.Put(Write{Provider: ProviderOpenAI, APIKey: "sk-newKEY0001"})
	if err != ErrEnvOwned {
		t.Fatalf("want env overlay, got %v", err)
	}
	if _, err := s.Clear("tts"); err != ErrEnvOwned {
		t.Fatalf("clear %v", err)
	}
}

func TestClearConfirmAndLocalTest(t *testing.T) {
	clearTTSEnv(t)
	s := New()
	on := true
	if _, err := s.Put(Write{Provider: ProviderEdge, Enabled: &on, Voice: "en"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Clear("nope"); err != ErrConfirm {
		t.Fatalf("confirm %v", err)
	}
	res := s.Test()
	if !res.OK || res.Kind != "local" || res.Error != "" {
		t.Fatalf("edge test %+v", res)
	}
	pub, err := s.Clear("edge")
	if err != nil {
		t.Fatal(err)
	}
	if pub.Configured || pub.KeySet || pub.Provider != ProviderNone {
		t.Fatalf("cleared %+v", pub)
	}
	res = s.Test()
	if res.OK || res.Error != ErrNotConfigured.Error() {
		t.Fatalf("none test %+v", res)
	}
}

func TestDisabledAndMissingKey(t *testing.T) {
	clearTTSEnv(t)
	s := New()
	off := false
	if _, err := s.Put(Write{Provider: ProviderOpenAI, Enabled: &off, APIKey: "sk-disabled01"}); err != nil {
		t.Fatal(err)
	}
	res := s.Test()
	if res.OK || res.Error != ErrDisabled.Error() {
		t.Fatalf("disabled %+v", res)
	}
	if _, err := s.Clear("tts"); err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := s.Put(Write{Provider: ProviderElevenLabs, Enabled: &on}); err != nil {
		t.Fatal(err)
	}
	res = s.Test()
	if res.OK || res.Error != ErrNotConfigured.Error() {
		t.Fatalf("missing key %+v", res)
	}
}

func TestHTTPProbeRedactsAuthorization(t *testing.T) {
	clearTTSEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad","authorization":"Bearer sk-LEAKEDKEY99","api_key":"sk-LEAKEDKEY99"}`))
	}))
	t.Cleanup(srv.Close)
	s := New()
	s.Client = srv.Client()
	on := true
	if _, err := s.Put(Write{Provider: ProviderOpenAI, Enabled: &on, APIKey: "sk-LEAKEDKEY99", Endpoint: srv.URL}); err != nil {
		t.Fatal(err)
	}
	res := s.Test()
	if res.OK {
		t.Fatalf("want fail %+v", res)
	}
	if strings.Contains(res.Error, "sk-LEAKED") || strings.Contains(strings.ToLower(res.Error), "bearer sk-") {
		t.Fatalf("leaked auth %q", res.Error)
	}
	if res.Error != "unauthorized" {
		t.Fatalf("want unauthorized, got %q", res.Error)
	}
}

func TestHTTPProbeOK(t *testing.T) {
	clearTTSEnv(t)
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "" {
			sawAuth = true
		}
		if strings.Contains(strings.ToLower(r.URL.RawQuery), "key") {
			t.Errorf("key query")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	s := New()
	s.Client = srv.Client()
	on := true
	if _, err := s.Put(Write{Provider: ProviderElevenLabs, Enabled: &on, APIKey: "xi-testKEY01", Endpoint: srv.URL, AutoApply: ApplyAll, MaxChars: 99, TimeoutMS: 5000}); err != nil {
		t.Fatal(err)
	}
	res := s.Test()
	if !res.OK || res.Kind != "http" || !sawAuth {
		t.Fatalf("http test %+v saw=%v", res, sawAuth)
	}
}

func TestRedactAndAsPublicJSON(t *testing.T) {
	if got := Redact(`Bearer sk-abcdefghijk authorization: secret123`); strings.Contains(got, "sk-abcdef") || strings.Contains(got, "secret123") {
		t.Fatalf("redact %q", got)
	}
	if _, ok := AsPublicJSON(map[string]any{"api_key": "x"}); ok {
		t.Fatal("secret json accepted")
	}
	if ContainsSecrets(map[string]any{"provider": "openai", "key_set": true}) {
		t.Fatal("status looks secret")
	}
}

func TestUnknownProviderAndApply(t *testing.T) {
	clearTTSEnv(t)
	s := New()
	if _, err := s.Put(Write{Provider: "paid-vendor"}); err != ErrProvider {
		t.Fatalf("provider %v", err)
	}
	if _, err := s.Put(Write{Provider: ProviderOpenAI, AutoApply: "always"}); err != ErrApply {
		t.Fatalf("apply %v", err)
	}
}

func TestClampAndConfirm(t *testing.T) {
	if ClampMaxChars(0) != DefaultMaxChars || ClampMaxChars(99999) != MaxMaxChars {
		t.Fatal("chars")
	}
	if ClampTimeout(0) != DefaultTimeoutMS || ClampTimeout(10) != MinTimeoutMS {
		t.Fatal("timeout")
	}
	if !ConfirmMatch("TTS", "openai") || !ConfirmMatch("openai", "openai") || ConfirmMatch("", "openai") {
		t.Fatal("confirm")
	}
}
