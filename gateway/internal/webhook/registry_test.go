// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package webhook

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
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

func TestRegistry_PersistAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webhooks.db")
	s1, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	r1 := NewWithStore(s1)
	c, err := r1.CreateOpts(CreateOpts{Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	r2 := NewWithStore(s2)
	if err := r2.Authenticate("Bearer "+c.Token, "", nil); err != nil {
		t.Fatalf("bearer after reopen: %v", err)
	}
	pubs := r2.List()
	if len(pubs) != 1 || pubs[0].ID != c.ID {
		t.Fatalf("list after reopen %+v", pubs)
	}
}

func TestRegistry_HMACEncryptedPersists(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	dir := t.TempDir()
	path := filepath.Join(dir, "enc.db")
	s1, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	r1 := NewWithStore(s1)
	c, err := r1.Create()
	if err != nil {
		t.Fatal(err)
	}
	row, err := s1.GetWebhook(c.ID)
	if err != nil || row.HMACEnc == "" {
		t.Fatalf("hmac_enc not stored: %#v %v", row, err)
	}
	if strings.Contains(row.HMACEnc, c.HMACKey) {
		t.Fatal("hmac key stored in plaintext")
	}
	_ = s1.Close()

	s2, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	r2 := NewWithStore(s2)
	body := []byte(`{"input":"enc","mode":"sync"}`)
	sig := Sign(c.HMACKey, time.Now(), body)
	if err := r2.Authenticate("", sig, body); err != nil {
		t.Fatalf("hmac after reopen: %v", err)
	}
}

func TestRegistry_StaleHMAC(t *testing.T) {
	r := New()
	c, err := r.Create()
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"input":"stale","mode":"sync"}`)
	old := Sign(c.HMACKey, time.Now().Add(-301*time.Second), body)
	if err := r.Authenticate("", old, body); err != ErrStaleHMAC {
		t.Fatalf("want stale got %v", err)
	}
	fresh := Sign(c.HMACKey, time.Now(), body)
	if err := r.Authenticate("", fresh, body); err != nil {
		t.Fatalf("fresh: %v", err)
	}
}

func TestRegistry_ReplayHMAC(t *testing.T) {
	r := New()
	c, err := r.Create()
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"input":"replay","mode":"sync"}`)
	sig := Sign(c.HMACKey, time.Now(), body)
	if err := r.Authenticate("", sig, body); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.Authenticate("", sig, body); err != ErrReplay {
		t.Fatalf("want replay got %v", err)
	}
}

func TestRegistry_RotateInvalidatesBearer(t *testing.T) {
	r := New()
	c, err := r.Create()
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := r.Rotate(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Token == c.Token || rotated.HMACKey == c.HMACKey {
		t.Fatal("rotate should mint new secrets")
	}
	if err := r.Authenticate("Bearer "+c.Token, "", nil); err != ErrUnauthorized {
		t.Fatalf("old bearer %v", err)
	}
	if err := r.Authenticate("Bearer "+rotated.Token, "", nil); err != nil {
		t.Fatalf("new bearer %v", err)
	}
}

func TestRegistry_Revoke(t *testing.T) {
	r := New()
	c, err := r.Create()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Authenticate("Bearer "+c.Token, "", nil); err != ErrUnauthorized {
		t.Fatalf("revoked %v", err)
	}
	if n := len(r.List()); n != 0 {
		t.Fatalf("list after revoke %d", n)
	}
}
