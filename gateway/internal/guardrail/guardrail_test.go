package guardrail

import (
	"strings"
	"testing"
)

func TestAppend(t *testing.T) {
	if got := Append(""); got != Reply {
		t.Fatalf("empty: %q", got)
	}
	got := Append("agent rules")
	if !strings.HasPrefix(got, "agent rules\n\n") || !strings.HasSuffix(got, Reply) {
		t.Fatalf("order: %q", got)
	}
	if Append(got) != got {
		t.Fatal("rule added twice")
	}
	if len(Reply) > 900 {
		t.Fatalf("guardrail too long: %d bytes", len(Reply))
	}
}
