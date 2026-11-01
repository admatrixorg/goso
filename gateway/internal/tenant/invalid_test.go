// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"net/http/httptest"
	"testing"
)

func TestInvalid(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	req := httptest.NewRequest("GET", "/", nil)
	if Invalid(req) {
		t.Fatal("no header → not invalid")
	}
	req.Header.Set(Header, "acme-1")
	if Invalid(req) {
		t.Fatal("valid id → not invalid")
	}
	req.Header.Set(Header, "bad tenant!")
	if !Invalid(req) {
		t.Fatal("bad id → invalid")
	}
	t.Setenv("GOSO_MULTI_TENANT", "")
	if Invalid(req) {
		t.Fatal("multi-tenant off → never invalid")
	}
}
