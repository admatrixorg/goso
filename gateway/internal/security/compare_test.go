// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"crypto/subtle"
	"testing"
)

func TestEqual_ConstantTimeCompare(t *testing.T) {
	if !Equal("secret-041", "secret-041") {
		t.Fatal("matching tokens")
	}
	if Equal("secret-041", "secret-040") {
		t.Fatal("mismatch must fail")
	}
	if Equal("short", "longer-token") {
		t.Fatal("length mismatch must fail")
	}
	if Equal("", "x") || Equal("x", "") || !Equal("", "") {
		t.Fatal("empty cases")
	}
	a := []byte("Bearer-token-value")
	b := []byte("Bearer-token-value")
	if subtle.ConstantTimeCompare(a, b) != 1 {
		t.Fatal("stdlib ConstantTimeCompare")
	}
	if subtle.ConstantTimeCompare(a, []byte("other-token-value")) == 1 {
		t.Fatal("stdlib mismatch")
	}
}
