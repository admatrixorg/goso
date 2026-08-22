// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import "testing"

func TestManifest_ParseAndValidate(t *testing.T) {
	raw := []byte(`{
		"schema_version": "1.0",
		"name": "zalocrm",
		"tools": [
			{
				"name": "contact_search",
				"description": "Search contacts",
				"requires_approval": false,
				"input_schema": {
					"type": "object",
					"properties": {"query": {"type": "string"}},
					"required": ["query"]
				}
			},
			{
				"name": "message_send",
				"description": "Send a message",
				"requires_approval": true,
				"inputSchema": {
					"type": "object",
					"properties": {"text": {"type": "string"}},
					"required": ["text"]
				}
			}
		]
	}`)
	m, err := ParseManifest(raw)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(m.Tools) != 2 {
		t.Fatalf("tools %d", len(m.Tools))
	}
	t2, ok := m.Tool("message_send")
	if !ok || !t2.RequiresApproval {
		t.Fatalf("message_send approval %v %v", ok, t2)
	}
	if err := ValidateArgs(m.Tools[0].InputSchema, map[string]any{"query": "A"}); err != nil {
		t.Fatalf("ValidateArgs: %v", err)
	}
	if err := ValidateArgs(m.Tools[0].InputSchema, map[string]any{}); err == nil {
		t.Fatal("expected missing required")
	}
}

func TestManifest_RejectsBadSchema(t *testing.T) {
	cases := []string{
		`{}`,
		`{"tools":[]}`,
		`{"tools":[{"name":"x"}]}`,
		`{"tools":[{"name":"x","input_schema":"string"}]}`,
		`{"tools":[{"name":"x","input_schema":{"type":"string"}}]}`,
		`not-json`,
	}
	for _, c := range cases {
		if _, err := ParseManifest([]byte(c)); err == nil {
			t.Fatalf("expected error for %s", c)
		}
	}
}
