// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func invokeFetch(t *testing.T, args map[string]any) (status string, content map[string]any) {
	t.Helper()
	res, err := Invoke(context.Background(), ToolWebFetch, args, false)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if _, err := json.Marshal(res); err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	m, ok := res.Content.(map[string]any)
	if !ok {
		t.Fatalf("content %T %+v", res.Content, res)
	}
	return res.Status, m
}

func TestConfigured_WebFetchAlways(t *testing.T) {
	if !Configured(ToolWebFetch) {
		t.Fatal("web_fetch must stay advertised configured")
	}
}

func TestInvoke_WebFetchEmptyURL(t *testing.T) {
	for _, args := range []map[string]any{nil, {}, {"url": ""}, {"url": "  "}} {
		status, content := invokeFetch(t, args)
		if status != "error" {
			t.Fatalf("status %s args %+v", status, args)
		}
		errStr, _ := content["error"].(string)
		if errStr != "url is required" {
			t.Fatalf("error %q args %+v", errStr, args)
		}
	}
}

func TestInvoke_WebFetchHttptest200(t *testing.T) {
	t.Setenv("GOSO_SSRF", "")
	t.Setenv("GOSO_ENV", "demo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello fetch"))
	}))
	defer srv.Close()

	status, content := invokeFetch(t, map[string]any{"url": srv.URL})
	if status != "ok" {
		t.Fatalf("status %s %+v", status, content)
	}
	if content["status"] != 200 {
		t.Fatalf("http status %+v", content["status"])
	}
	if content["content_type"] != "text/plain; charset=utf-8" {
		t.Fatalf("content_type %v", content["content_type"])
	}
	if content["body"] != "hello fetch" {
		t.Fatalf("body %v", content["body"])
	}
	res, err := Invoke(context.Background(), ToolWebFetch, map[string]any{"url": srv.URL}, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("text/plain must marshal: %v", err)
	}
	var round struct {
		Status  string          `json:"status"`
		Content map[string]any  `json:"content"`
		Raw     json.RawMessage `json:"raw"`
	}
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if round.Status != "ok" || round.Content["body"] != "hello fetch" {
		t.Fatalf("round-trip %+v", round)
	}
	if len(round.Raw) > 0 {
		t.Fatalf("raw must be omitted, got %s", round.Raw)
	}
}

func TestInvoke_WebFetchNon2xxStillReturns(t *testing.T) {
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_ENV", "demo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer srv.Close()

	status, content := invokeFetch(t, map[string]any{"url": srv.URL})
	if status != "ok" {
		t.Fatalf("non-2xx must not panic or error, got %s %+v", status, content)
	}
	if content["status"] != 404 {
		t.Fatalf("http status %+v", content["status"])
	}
	if content["body"] != "missing" {
		t.Fatalf("body %v", content["body"])
	}
}

func TestInvoke_WebFetchBodyCap(t *testing.T) {
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_ENV", "demo")
	payload := strings.Repeat("a", maxFetchBody+64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	status, content := invokeFetch(t, map[string]any{"url": srv.URL})
	if status != "ok" {
		t.Fatalf("status %s %+v", status, content)
	}
	body, _ := content["body"].(string)
	if len(body) != maxFetchBody {
		t.Fatalf("body len %d", len(body))
	}
}

func TestInvoke_WebFetchSSRFNoDial(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		t.Error("must not dial when SSRF blocks loopback")
	}))
	defer srv.Close()

	status, content := invokeFetch(t, map[string]any{"url": srv.URL})
	if status != "error" {
		t.Fatalf("status %s %+v", status, content)
	}
	errStr, _ := content["error"].(string)
	if !strings.Contains(strings.ToLower(errStr), "ssrf") {
		t.Fatalf("error must contain ssrf, got %q", errStr)
	}
	if hit != 0 {
		t.Fatalf("dialed %d", hit)
	}

	status, content = invokeFetch(t, map[string]any{"url": "http://127.0.0.1/"})
	if status != "error" {
		t.Fatalf("literal loopback status %s %+v", status, content)
	}
	errStr, _ = content["error"].(string)
	if !strings.Contains(strings.ToLower(errStr), "ssrf") {
		t.Fatalf("literal loopback error %q", errStr)
	}
}

func TestInvoke_WebFetchRedirectLoopbackBlocked(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	secretHit := 0
	secret := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHit++
		_, _ = w.Write([]byte("secret"))
	}))
	defer secret.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, secret.URL, http.StatusFound)
	}))
	defer first.Close()

	parsed, err := url.Parse(first.URL)
	if err != nil {
		t.Fatal(err)
	}
	publicURL := "http://1.1.1.1:" + parsed.Port() + "/"
	prev := WebFetchClient
	WebFetchClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				if host == "1.1.1.1" {
					addr = net.JoinHostPort("127.0.0.1", port)
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}
	t.Cleanup(func() { WebFetchClient = prev })

	status, content := invokeFetch(t, map[string]any{"url": publicURL})
	if status != "error" {
		t.Fatalf("status %s %+v", status, content)
	}
	errStr, _ := content["error"].(string)
	if !strings.Contains(strings.ToLower(errStr), "ssrf") {
		t.Fatalf("redirect error %q", errStr)
	}
	if secretHit != 0 {
		t.Fatalf("redirect dialed secret %d", secretHit)
	}
}
