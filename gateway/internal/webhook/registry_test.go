// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package webhook

import (
	"strings"
	"testing"
	"time"
)

func TestRegistry_CreateHashedAndAuth(t *testing.T) {
	r := New()
	c, err := r.Create()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.Token, "wh_") || c.HMACKey == "" || c.ID == "" {
		t.Fatalf("created %+v", c)
	}
	if c.TokenPrefix == c.Token {
		t.Fatal("prefix should be truncated")
	}
	pubs := r.List()
	if len(pubs) != 1 || pubs[0].TokenPrefix != c.TokenPrefix {
		t.Fatalf("list %+v", pubs)
	}
	if pubs[0].ID != c.ID {
		t.Fatal("id")
	}

	if err := r.Authenticate("Bearer "+c.Token, "", nil); err != nil {
		t.Fatalf("bearer: %v", err)
	}
	if err := r.Authenticate("Bearer wh_deadbeef", "", nil); err != ErrUnauthorized {
		t.Fatalf("bad bearer %v", err)
	}

	body := []byte(`{"input":"hi","mode":"sync"}`)
	sig := Sign(c.HMACKey, time.Now(), body)
	if err := r.Authenticate("", sig, body); err != nil {
		t.Fatalf("hmac: %v", err)
	}
	bad := Sign("wrong-key-not-the-secret", time.Now(), body)
	if err := r.Authenticate("", bad, body); err != ErrUnauthorized {
		t.Fatalf("bad hmac %v", err)
	}
	if err := r.Authenticate("", "", body); err != ErrUnauthorized {
		t.Fatalf("empty %v", err)
	}
}

func TestRegistry_SignFormat(t *testing.T) {
	sig := Sign("k", time.Unix(1700000000, 0), []byte("body"))
	if !strings.HasPrefix(sig, "t=1700000000,v1=") {
		t.Fatalf("sig %s", sig)
	}
}
