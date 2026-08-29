// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package node

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRequestApproveDenyRevoke(t *testing.T) {
	n := New()
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	n.now = func() time.Time { return base }

	_, err := n.RequestAccess(Request{})
	if !errors.Is(err, ErrDisplayRequired) {
		t.Fatalf("empty display %v", err)
	}

	pending, err := n.RequestAccess(Request{Display: "ops-laptop"})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != statusPending || pending.Health != healthPending || pending.Kind != kindDashboard {
		t.Fatalf("pending %#v", pending)
	}
	if pending.ExpiresAt == nil || !pending.ExpiresAt.Equal(base.Add(pendingTTL)) {
		t.Fatalf("expiry %#v", pending.ExpiresAt)
	}

	_, err = n.Approve(pending.ID, "", "", base)
	if !errors.Is(err, ErrConfirmRequired) {
		t.Fatalf("confirm required %v", err)
	}
	_, err = n.Approve(pending.ID, "", "nope", base)
	if !errors.Is(err, ErrConfirm) {
		t.Fatalf("mismatch %v", err)
	}

	paired, err := n.Approve(pending.ID, "", pending.ID, base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if paired.Status != statusPaired || paired.Health != healthOK || paired.ApprovedAt == nil {
		t.Fatalf("paired %#v", paired)
	}
	if len(n.ListPending("", base.Add(time.Minute))) != 0 {
		t.Fatal("pending after approve")
	}
	if len(n.ListPaired("", base.Add(time.Minute))) != 1 {
		t.Fatal("paired list")
	}

	_, err = n.Approve(pending.ID, "", pending.ID, base.Add(time.Minute))
	if !errors.Is(err, ErrStatus) {
		t.Fatalf("already paired %v", err)
	}

	revoked, err := n.Revoke(pending.ID, "", "ops-laptop", base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != statusRevoked {
		t.Fatalf("revoked %#v", revoked)
	}
	if len(n.ListPaired("", base.Add(2*time.Minute))) != 0 {
		t.Fatal("paired after revoke")
	}

	other, err := n.RequestAccess(Request{Display: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := n.Deny(other.ID, "", other.Display, base.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if denied.Status != statusDenied {
		t.Fatalf("denied %#v", denied)
	}
	if len(n.ListPending("", base.Add(3*time.Minute))) != 0 {
		t.Fatal("pending after deny")
	}
}

func TestExpiredAndStaleAndTenant(t *testing.T) {
	n := New()
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	n.now = func() time.Time { return base }

	row, err := n.RequestAccess(Request{Display: "stale-box", TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	late := base.Add(pendingTTL + time.Second)
	got, err := n.Get(row.ID, "acme", late)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != healthExpired {
		t.Fatalf("expired health %#v", got)
	}
	_, err = n.Approve(row.ID, "acme", row.ID, late)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("approve expired %v", err)
	}
	if _, err := n.Deny(row.ID, "acme", row.ID, late); err != nil {
		t.Fatalf("deny expired %v", err)
	}
	if _, err := n.Get(row.ID, "other", late); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant %v", err)
	}
	if len(n.ListPending("other", late)) != 0 {
		t.Fatal("other tenant pending")
	}

	ok, err := n.RequestAccess(Request{Display: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := n.Approve(ok.ID, "", ok.Display, base)
	if err != nil {
		t.Fatal(err)
	}
	stale := n.ListPaired("", base.Add(staleAfter))
	if len(stale) != 1 || stale[0].Health != healthStale {
		t.Fatalf("stale %#v", stale)
	}
	if paired.Health != healthOK {
		t.Fatalf("fresh paired %#v", paired)
	}
}

func TestPublicJSONOmitsSecrets(t *testing.T) {
	n := New()
	row, err := n.RequestAccess(Request{Display: "desk"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, leak := range []string{`"token"`, `"code"`, `"secret"`, `"password"`, `"hmac"`, `"bot_token"`} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %s in %s", leak, body)
		}
	}
}

func TestPendingCap(t *testing.T) {
	n := New()
	for i := 0; i < maxPending; i++ {
		if _, err := n.RequestAccess(Request{Display: "d"}); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if _, err := n.RequestAccess(Request{Display: "overflow"}); !errors.Is(err, ErrCap) {
		t.Fatalf("cap %v", err)
	}
}
