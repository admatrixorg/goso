// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"net/http/httptest"
	"testing"
)

func TestResolveDisabledIgnoresHeader(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "adm")
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set(Header, "acme")
	if got := Resolve(req); got != Default {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAdminNonDefault(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "adm")
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set(Header, "acme")
	if got := Resolve(req); got != "acme" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveViewTokenForcedDefault(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "adm")
	t.Setenv("GOSO_VIEW_TOKEN", "view")
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer view")
	req.Header.Set(Header, "acme")
	if got := Resolve(req); got != Default {
		t.Fatalf("view switched: %q", got)
	}
}
