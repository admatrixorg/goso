// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextfile

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestEnabled_DefaultOff(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "")
	if Enabled() {
		t.Fatal("unset flag must be off")
	}
	for _, v := range []string{"0", "off", "false", "no"} {
		t.Setenv("GOSO_CONTEXT_FILES", v)
		if Enabled() {
			t.Fatalf("%q must be off", v)
		}
	}
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("GOSO_CONTEXT_FILES", v)
		if !Enabled() {
			t.Fatalf("%q must be on", v)
		}
	}
}

func TestLogFailure_ClassOnly(t *testing.T) {
	var buf bytes.Buffer
	old := logPrintf
	logPrintf = func(format string, args ...any) {
		fmt.Fprintf(&buf, format, args...)
	}
	t.Cleanup(func() { logPrintf = old })

	secret := "Chiết khấu tối đa của Minh An là 9 phần trăm. lan@example"
	LogFailure("put", errors.New("disk I/O "+secret))
	got := buf.String()
	if got != "goso context-files: op=put class=store" {
		t.Fatalf("log %q", got)
	}
	if strings.Contains(got, "Chiết") || strings.Contains(got, "lan") || strings.Contains(got, "disk") {
		t.Fatalf("log leaked: %q", got)
	}
	if Class(store.ErrContextInvalid) != "invalid" || Class(store.ErrContextTooLarge) != "too_large" || Class(store.ErrNotFound) != "not_found" {
		t.Fatalf("classes invalid=%s large=%s missing=%s", Class(store.ErrContextInvalid), Class(store.ErrContextTooLarge), Class(store.ErrNotFound))
	}
}

func TestResolveUserKey(t *testing.T) {
	key, err := ResolveUserKey("", "")
	if err != nil || key != "" {
		t.Fatalf("empty: %q %v", key, err)
	}
	key, err = ResolveUserKey("lan", "")
	if err != nil || key != "lan" {
		t.Fatalf("header: %q %v", key, err)
	}
	key, err = ResolveUserKey("", "lan")
	if err != nil || key != "lan" {
		t.Fatalf("body: %q %v", key, err)
	}
	if _, err := ResolveUserKey("lan", "hoa"); !errors.Is(err, store.ErrContextInvalid) {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := ResolveUserKey("lan@shop", ""); !errors.Is(err, store.ErrContextInvalid) {
		t.Fatalf("email: %v", err)
	}
}
