// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func webhookRouter(t *testing.T) http.Handler {
	t.Helper()
	return NewRouter(Options{Store: store.New(), Version: "test", Provider: llm.Echo{}})
}

func createWebhook(t *testing.T, h http.Handler) webhook.Created {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created
}

func waitJobStatus(t *testing.T, h http.Handler, id, want string) webhook.Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last webhook.Job
	var lastCode int
	var lastBody string
	for time.Now().Before(deadline) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhooks/jobs/"+id, nil))
		lastCode = w.Code
		lastBody = w.Body.String()
		if w.Code == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &last); err == nil && last.Status == want {
				return last
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s want %s last=%d %s %+v", id, want, lastCode, lastBody, last)
	return last
}

func TestWebhookAPI_PersistSQLiteReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wh.db")
	s1, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	h1 := NewRouter(Options{Store: s1, Version: "test", Provider: llm.Echo{}})
	created := createWebhook(t, h1)
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	h2 := NewRouter(Options{Store: s2, Version: "test", Provider: llm.Echo{}})
	body := []byte(`{"input":"persist","mode":"sync"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h2.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bearer after reopen %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhooks", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(created.ID)) {
		t.Fatalf("list after reopen %d %s", w.Code, w.Body.String())
	}
}

func TestWebhookAPI_StaleHMAC401(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	body := []byte(`{"input":"stale","mode":"sync"}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", webhook.Sign(created.HMACKey, time.Now().Add(-301*time.Second), body))
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stale %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", webhook.Sign(created.HMACKey, time.Now(), body))
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("fresh %d %s", w.Code, w.Body.String())
	}
}

func TestWebhookAPI_ReplayHMAC401(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	body := []byte(`{"input":"replay","mode":"sync"}`)
	sig := webhook.Sign(created.HMACKey, time.Now(), body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", sig)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", sig)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("replay %d %s", w.Code, w.Body.String())
	}
}

func TestWebhookAPI_AsyncJobGETDone(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader([]byte(`{"input":"async job","mode":"async"}`)))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil || accepted.ID == "" {
		t.Fatalf("accepted %s", w.Body.String())
	}
	job := waitJobStatus(t, h, accepted.ID, store.WebhookDone)
	if job.Reply != "echo: async job" {
		t.Fatalf("reply %+v", job)
	}
}

func TestWebhookAPI_Callback2xxDone(t *testing.T) {
	t.Setenv("GOSO_WEBHOOK_RETRY_MS", "10,20,30")
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_SSRF", "")
	var gotID, gotUA, gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Goso-Delivery-Id")
		gotUA = r.Header.Get("User-Agent")
		gotSig = r.Header.Get("X-Goso-Signature")
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := webhookRouter(t)
	created := createWebhook(t, h)
	body, _ := json.Marshal(map[string]any{"input": "cb2xx", "mode": "async", "callback_url": srv.URL})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	waitJobStatus(t, h, accepted.ID, store.WebhookDone)
	if gotID != accepted.ID {
		t.Fatalf("delivery id %q want %s", gotID, accepted.ID)
	}
	if gotUA != "goso-webhook/1" {
		t.Fatalf("ua %q", gotUA)
	}
	if gotSig == "" {
		t.Fatal("missing X-Goso-Signature")
	}
}

func TestWebhookAPI_Callback500ThenRetry(t *testing.T) {
	t.Setenv("GOSO_WEBHOOK_RETRY_MS", "10,20,30")
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_SSRF", "")
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := webhookRouter(t)
	created := createWebhook(t, h)
	body, _ := json.Marshal(map[string]any{"input": "retry", "mode": "async", "callback_url": srv.URL})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	waitJobStatus(t, h, accepted.ID, store.WebhookDone)
	if n.Load() < 2 {
		t.Fatalf("want retry, attempts %d", n.Load())
	}
}

func TestWebhookAPI_Callback400Dead(t *testing.T) {
	t.Setenv("GOSO_WEBHOOK_RETRY_MS", "10,20,30")
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_SSRF", "")
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	h := webhookRouter(t)
	created := createWebhook(t, h)
	body, _ := json.Marshal(map[string]any{"input": "dead", "mode": "async", "callback_url": srv.URL})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	job := waitJobStatus(t, h, accepted.ID, store.WebhookDead)
	if job.Attempts != 1 {
		t.Fatalf("attempts %+v", job)
	}
	if n.Load() != 1 {
		t.Fatalf("callback count %d", n.Load())
	}
}

func TestWebhookAPI_Idempotency(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	body := []byte(`{"input":"idem","mode":"async"}`)
	post := func(raw []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+created.Token)
		req.Header.Set("Idempotency-Key", "k-1")
		h.ServeHTTP(w, req)
		return w
	}
	w1 := post(body)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first %d %s", w1.Code, w1.Body.String())
	}
	var a1, a2 struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(w1.Body.Bytes(), &a1)
	w2 := post(body)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("replay %d %s", w2.Code, w2.Body.String())
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &a2)
	if a1.ID == "" || a1.ID != a2.ID {
		t.Fatalf("ids %q %q", a1.ID, a2.ID)
	}
	w3 := post([]byte(`{"input":"other","mode":"async"}`))
	if w3.Code != http.StatusConflict {
		t.Fatalf("conflict %d %s", w3.Code, w3.Body.String())
	}
}

func TestWebhookAPI_RotateInvalidatesBearer(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks/"+created.ID+"/rotate", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("rotate %d %s", w.Code, w.Body.String())
	}
	var rotated webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"input":"rot","mode":"sync"}`)
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("old bearer %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rotated.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("new bearer %d %s", w.Code, w.Body.String())
	}
}

func TestWebhookAPI_DeleteRevokes(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/webhooks/"+created.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	body := []byte(`{"input":"rev","mode":"sync"}`)
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked %d %s", w.Code, w.Body.String())
	}
}
