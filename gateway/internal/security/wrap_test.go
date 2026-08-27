// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"strings"
	"testing"
)

func TestWrapUntrusted(t *testing.T) {
	out := WrapUntrusted(`{"contacts":["A"]}`)
	if !strings.HasPrefix(out, UntrustedBegin) || !strings.HasSuffix(out, UntrustedEnd) {
		t.Fatalf("markers: %q", out)
	}
	if !strings.Contains(out, `{"contacts":["A"]}`) {
		t.Fatalf("payload: %q", out)
	}
}
