// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLookupEnvWinsOverlay(t *testing.T) {
	t.Cleanup(ResetOverlay)
	ResetOverlay()
	t.Setenv("GOSO_QUOTA_DAY", "9")
	SetOverlay(map[string]string{"quota_day": "3"})
	if Lookup("GOSO_QUOTA_DAY") != "9" {
		t.Fatalf("env must win, got %q", Lookup("GOSO_QUOTA_DAY"))
	}
	t.Setenv("GOSO_QUOTA_DAY", "")
	if Lookup("GOSO_QUOTA_DAY") != "3" {
		t.Fatalf("overlay fallback got %q", Lookup("GOSO_QUOTA_DAY"))
	}
}

func TestApplyPatchValidationAndEnvOwned(t *testing.T) {
	t.Cleanup(ResetOverlay)
	ResetOverlay()
	t.Setenv("GOSO_INJECTION", "block")
	_, err := ApplyPatch(nil, map[string]string{"injection": "log"})
	pe, ok := err.(*PatchError)
	if !ok || pe.Status != 409 || !strings.Contains(pe.Message, "env-owned") {
		t.Fatalf("env-owned want 409, got %v", err)
	}
	t.Setenv("GOSO_INJECTION", "")
	out, err := ApplyPatch(nil, map[string]string{"injection": "log", "quota_day": "12"})
	if err != nil {
		t.Fatal(err)
	}
	if out["injection"] != "log" || out["quota_day"] != "12" {
		t.Fatalf("merged %+v", out)
	}
	_, err = ApplyPatch(nil, map[string]string{"quota_day": "-1"})
	if err == nil {
		t.Fatal("negative quota must fail")
	}
	_, err = ApplyPatch(nil, map[string]string{"token": "secret"})
	if err == nil {
		t.Fatal("secret key must fail")
	}
	_, err = ApplyPatch(nil, map[string]string{"unknown": "x"})
	if err == nil {
		t.Fatal("unknown field must fail")
	}
}

func TestSnapshotNeverReturnsAuthToken(t *testing.T) {
	t.Cleanup(ResetOverlay)
	ResetOverlay()
	secret := "goso-test-admin-token-xyz"
	t.Setenv("GOSO_ADMIN_TOKEN", secret)
	t.Setenv("GOSO_VIEW_TOKEN", "view-token-abcdef")
	t.Setenv("GOSO_MASTER_KEY", "master-key-abcdef")
	t.Setenv("GOSO_DATABASE_URL", "postgres://user:pass@localhost/goso")
	t.Setenv("GOSO_OTEL_ENDPOINT", "http://127.0.0.1:4318")
	snap := BuildSnapshot(time.Time{})
	if snap.Auth.TokenSet.Value != true || snap.Auth.TokenSet.Editable {
		t.Fatalf("token_set %+v", snap.Auth.TokenSet)
	}
	b, err := MarshalPublic(snap)
	if err != nil {
		t.Fatal(err)
	}
	if ContainsSecret(b) {
		t.Fatalf("snapshot leaked secret: %s", b)
	}
	if strings.Contains(string(b), secret) || strings.Contains(string(b), "view-token-abcdef") {
		t.Fatalf("token value in GET: %s", b)
	}
	if strings.Contains(string(b), "postgres://") || strings.Contains(string(b), "user:pass") {
		t.Fatalf("dsn in GET: %s", b)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	auth := decoded["auth"].(map[string]any)
	tok := auth["token_set"].(map[string]any)
	if _, ok := tok["value"].(bool); !ok {
		t.Fatalf("token_set.value must be bool, got %#v", tok["value"])
	}
}
