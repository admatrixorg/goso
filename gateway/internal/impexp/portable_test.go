// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package impexp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripArchiveDropsTokens(t *testing.T) {
	raw := []byte(`{
		"schema":"goso.portable/v1","schema_version":1,"include_secrets":true,
		"agents":[{"agent_key":"bot","display_name":"Bot","instructions":"call sk-live-fixture-not-vendor-zzzz"}],
		"mcp":[{"name":"crm","transport":"http","token":"sk-live-fixture-not-vendor-zzzz","credential_ref":"secret:connector/crm/token"}]
	}`)
	a, err := DecodeArchive(raw)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateSchema(a); len(errs) != 0 {
		t.Fatalf("schema %v", errs)
	}
	StripArchive(a)
	if a.IncludeSecrets {
		t.Fatal("include_secrets must stay false")
	}
	if strings.Contains(a.Agents[0].Instructions, "sk-") {
		t.Fatalf("instructions still have token: %s", a.Agents[0].Instructions)
	}
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "sk-live") || strings.Contains(string(b), `"token"`) {
		t.Fatalf("archive leaked token: %s", b)
	}
	if ContainsSecrets(a) {
		t.Fatal("stripped archive still flagged")
	}
}

func TestValidateSchema(t *testing.T) {
	if errs := ValidateSchema(&Archive{Schema: Schema, SchemaVersion: SchemaVersion}); len(errs) != 0 {
		t.Fatalf("v1 ok %v", errs)
	}
	if errs := ValidateSchema(&Archive{Schema: "other", SchemaVersion: 1}); len(errs) == 0 {
		t.Fatal("bad schema")
	}
	if errs := ValidateSchema(&Archive{Schema: Schema, SchemaVersion: 99}); len(errs) == 0 {
		t.Fatal("bad version")
	}
}

func TestDecodeDoesNotInventSchema(t *testing.T) {
	a, err := DecodeArchive([]byte(`{"agents":[{"agent_key":"bot","display_name":"Bot"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateSchema(a); len(errs) == 0 {
		t.Fatal("missing schema must fail")
	}
	a2, err := DecodeArchive([]byte(`{"schema":"goso.portable/v1","schema_version":0,"agents":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if errs := ValidateSchema(a2); len(errs) == 0 {
		t.Fatal("version 0 must fail")
	}
}

func TestNormalizeConflict(t *testing.T) {
	c, err := NormalizeConflict("")
	if err != nil || c != ConflictSkip {
		t.Fatalf("default %s %v", c, err)
	}
	if _, err := NormalizeConflict("merge"); err == nil {
		t.Fatal("unknown must fail")
	}
}

func TestContainsSecrets(t *testing.T) {
	if ContainsSecrets(map[string]any{"name": "crm", "token_set": true}) {
		t.Fatal("token_set bool is not a secret")
	}
	if !ContainsSecrets(map[string]any{"token": "abc"}) {
		t.Fatal("token string is a secret")
	}
}
