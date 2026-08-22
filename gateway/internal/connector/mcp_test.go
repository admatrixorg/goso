// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestMCP_HTTPFakeServer(t *testing.T) {
	tools := []Tool{sampleTool("contact_search", false)}
	srv := httptest.NewServer(ServeFakeMCP(tools, func(name string, args map[string]any) (any, error) {
		return map[string]any{"contacts": []any{map[string]any{"name": args["query"]}}}, nil
	}))
	defer srv.Close()

	c, err := Build(Config{
		Name:      "crm",
		Transport: TransportMCPHTTP,
		Endpoint:  srv.URL,
		TimeoutMS: 2000,
		Client:    srv.Client(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	list, err := c.ListTools(context.Background())
	if err != nil || len(list) != 1 || list[0].Name != "contact_search" {
		t.Fatalf("ListTools: %v %v", err, list)
	}
	res, err := c.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m, _ := res.Content.(map[string]any)
	if m["contacts"] == nil {
		t.Fatalf("content %v", res.Content)
	}
}

func TestMCP_StdioInProcess(t *testing.T) {
	tools := []Tool{sampleTool("contact_search", false)}
	pipes, stop := StartFakeMCPStdio(tools, func(name string, args map[string]any) (any, error) {
		return map[string]any{"ok": true, "q": args["query"]}, nil
	})
	defer stop()

	c, err := Build(Config{
		Name:      "crm-stdio",
		Transport: TransportMCPStdio,
		Stdio:     pipes,
		ManifestJSON: []byte(`{
			"schema_version":"1.0",
			"tools":[{"name":"contact_search","description":"s","input_schema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}]
		}`),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	res, err := c.Invoke(context.Background(), "contact_search", map[string]any{"query": "Ada"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m, _ := res.Content.(map[string]any)
	if m["q"] != "Ada" {
		t.Fatalf("content %v", res.Content)
	}
}
