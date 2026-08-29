// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package apikey

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCreateListGetNeverReturnsSecret(t *testing.T) {
	r := New()
	exp := time.Now().UTC().Add(time.Hour)
	created, err := r.Create(Input{Name: "ops", Scopes: []string{"read", "write"}, ExpiresAt: &exp})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || !strings.HasPrefix(created.Secret, "gk_") {
		t.Fatalf("secret %q", created.Secret)
	}
	if created.Prefix == "" || created.Prefix == created.Secret || !strings.HasPrefix(created.Secret, created.Prefix) {
		t.Fatalf("prefix %q secret %q", created.Prefix, created.Secret)
	}
	if created.Status != StatusActive || created.UseCount != 0 {
		t.Fatalf("created %#v", created.Public)
	}

	raw, _ := json.Marshal(r.List(""))
	if strings.Contains(string(raw), created.Secret) || strings.Contains(string(raw), `"secret"`) || strings.Contains(string(raw), `"hash"`) {
		t.Fatalf("list leaked %s", raw)
	}
	got, err := r.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(got)
	if strings.Contains(string(raw), created.Secret) || strings.Contains(string(raw), `"secret"`) {
		t.Fatalf("get leaked %s", raw)
	}
	if got.Prefix != created.Prefix || got.Name != "ops" {
		t.Fatalf("get %#v", got)
	}
}

func TestCreateRejectsSecretShapedAndUnknownScope(t *testing.T) {
	r := New()
	if _, err := r.Create(Input{Name: "", Scopes: []string{"read"}}); err != ErrName {
		t.Fatalf("empty name %v", err)
	}
	if _, err := r.Create(Input{Name: "sk-live-abcdefgh", Scopes: []string{"read"}}); err != ErrSecret {
		t.Fatalf("secret name %v", err)
	}
	if _, err := r.Create(Input{Name: "gk_" + strings.Repeat("ab", 12), Scopes: []string{"read"}}); err != ErrSecret {
		t.Fatalf("own-format secret name %v", err)
	}
	if _, err := r.Create(Input{Name: "ops", Scopes: nil}); err != ErrScope {
		t.Fatalf("no scope %v", err)
	}
	if _, err := r.Create(Input{Name: "ops", Scopes: []string{"root"}}); err != ErrUnknownScope {
		t.Fatalf("unknown %v", err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	if _, err := r.Create(Input{Name: "ops", Scopes: []string{"read"}, ExpiresAt: &past}); err != ErrExpiry {
		t.Fatalf("past %v", err)
	}
}

func TestRevokeConfirmAndAcceptUsage(t *testing.T) {
	r := New()
	created, err := r.Create(Input{Name: "ci", Scopes: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Accept("nope"); ok {
		t.Fatal("unknown accepted")
	}
	g, ok := r.Accept(created.Secret)
	if !ok || g.ID != created.ID || g.Prefix != created.Prefix || !g.Has("read") {
		t.Fatalf("accept %#v %v", g, ok)
	}
	got, _ := r.Get(created.ID)
	if got.UseCount != 1 || got.LastUsedAt == nil {
		t.Fatalf("usage %#v", got)
	}
	if _, err := r.Revoke(created.ID, ""); err != ErrConfirmRequired {
		t.Fatalf("no confirm %v", err)
	}
	if _, err := r.Revoke(created.ID, "nope"); err != ErrConfirm {
		t.Fatalf("mismatch %v", err)
	}
	rev, err := r.Revoke(created.ID, created.Prefix)
	if err != nil || rev.Status != StatusRevoked {
		t.Fatalf("revoke %v %#v", err, rev)
	}
	if _, ok := r.Accept(created.Secret); ok {
		t.Fatal("revoked accepted")
	}
	raw, _ := json.Marshal(rev)
	if strings.Contains(string(raw), created.Secret) {
		t.Fatalf("revoke leaked %s", raw)
	}
}

func TestExpiredRejectsAccept(t *testing.T) {
	r := New()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	exp := now.Add(time.Minute)
	created, err := r.Create(Input{Name: "ttl", Scopes: []string{"admin"}, ExpiresAt: &exp})
	if err != nil {
		t.Fatal(err)
	}
	r.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, ok := r.Accept(created.Secret); ok {
		t.Fatal("expired accepted")
	}
	got, _ := r.Get(created.ID)
	if got.Status != StatusExpired {
		t.Fatalf("status %s", got.Status)
	}
}

func TestListSearchAndScopeDedupe(t *testing.T) {
	r := New()
	a, err := r.Create(Input{Name: "alpha", TenantID: "acme", Scopes: []string{"read", "READ", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(a.Scopes, ",") != "read,write" {
		t.Fatalf("scopes %#v", a.Scopes)
	}
	_, err = r.Create(Input{Name: "beta", Scopes: []string{"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := r.List("acme"); len(n) != 1 || n[0].Name != "alpha" {
		t.Fatalf("search %#v", n)
	}
	if n := r.List("ADMIN"); len(n) != 1 || n[0].Name != "beta" {
		t.Fatalf("scope search %#v", n)
	}
}
