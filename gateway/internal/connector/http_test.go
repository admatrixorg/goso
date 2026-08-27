// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransport_ManifestURLAndInvoke(t *testing.T) {
	manifest := `{
		"schema_version":"1.0",
		"tools":[
			{"name":"contact_search","description":"search","requires_approval":false,
			 "input_schema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}
		]
	}`
	var sawAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("POST /tools/contact_search", func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		var args map[string]any
		_ = json.Unmarshal(b, &args)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []string{args["query"].(string)}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := Build(Config{
		Name:        "zalocrm",
		Transport:   TransportHTTP,
		Endpoint:    srv.URL,
		BearerToken: "test-token",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	tools, err := c.ListTools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("ListTools: %v %v", err, tools)
	}
	res, err := c.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if sawAuth != "Bearer test-token" {
		t.Fatalf("auth %q", sawAuth)
	}
	if res.Connector != "zalocrm" {
		t.Fatalf("connector name %q", res.Connector)
	}
}

func TestHTTPTransport_SSRFBlocksLocalhost(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	c, err := Build(Config{
		Name:         "ssrf",
		Transport:    TransportHTTP,
		Endpoint:     "http://127.0.0.1:9",
		ManifestJSON: json.RawMessage(`{"schema_version":"1.0","tools":[{"name":"x","input_schema":{"type":"object"}}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Invoke(context.Background(), "x", map[string]any{}); err == nil {
		t.Fatal("expected SSRF block")
	}
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected SSRF block on health")
	}
}

func TestHTTPTransport_InlineManifest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	mux.HandleFunc("POST /tools/order_lookup", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"order_id": "1", "total": 9})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	inline := []byte(`{"schema_version":"1.0","tools":[{"name":"order_lookup","description":"pos","input_schema":{"type":"object","properties":{"order_id":{"type":"string"}},"required":["order_id"]}}]}`)
	c, err := Build(Config{
		Name:         "pos",
		Transport:    TransportHTTP,
		Endpoint:     srv.URL,
		ManifestJSON: inline,
		Client:       srv.Client(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res, err := c.Invoke(context.Background(), "order_lookup", map[string]any{"order_id": "1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m, _ := res.Content.(map[string]any)
	if m["total"] == nil {
		t.Fatalf("content %v", res.Content)
	}
}

func TestHTTPTransport_OfflineUnavailable(t *testing.T) {
	c, err := Build(Config{
		Name:         "down",
		Transport:    TransportHTTP,
		Endpoint:     "http://127.0.0.1:1",
		TimeoutMS:    200,
		Retries:      0,
		ManifestJSON: []byte(`{"tools":[{"name":"x","input_schema":{"type":"object","properties":{}}}]}`),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	err = c.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "connector_unavailable") {
		t.Fatalf("expected unavailable, got %v", err)
	}
}
