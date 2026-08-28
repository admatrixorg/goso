// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPairing_IssueExchangeOnce(t *testing.T) {
	p := NewPairing()
	issued, err := p.Issue("view-077")
	if err != nil {
		t.Fatal(err)
	}
	if len(issued.Code) != 8 {
		t.Fatalf("code len %d", len(issued.Code))
	}
	if issued.TTLSeconds != 600 {
		t.Fatalf("ttl %d", issued.TTLSeconds)
	}
	if issued.Role != "view" {
		t.Fatalf("role %s", issued.Role)
	}
	got, err := p.Exchange(strings.ToLower(issued.Code))
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "view-077" || got.Role != "view" || got.Minted {
		t.Fatalf("exchange %+v", got)
	}
	if _, err := p.Exchange(issued.Code); err != ErrPairingInvalid {
		t.Fatalf("second exchange %v", err)
	}
}

func TestPairing_ExpiredAndUnknown(t *testing.T) {
	p := NewPairing()
	base := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return base }
	issued, err := p.Issue("view-077")
	if err != nil {
		t.Fatal(err)
	}
	p.now = func() time.Time { return base.Add(PairingTTL) }
	if _, err := p.Exchange(issued.Code); err != ErrPairingExpired {
		t.Fatalf("expired %v", err)
	}
	if _, err := p.Exchange("NOTACODE"); err != ErrPairingInvalid {
		t.Fatalf("unknown %v", err)
	}
	if _, err := p.Exchange("  "); err != ErrPairingInvalid {
		t.Fatalf("blank %v", err)
	}
}

func TestPairing_MintedGrantAccepts(t *testing.T) {
	p := NewPairing()
	issued, err := p.Issue("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Accepts("gv_notyet") {
		t.Fatal("unknown grant")
	}
	got, err := p.Exchange(issued.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Token, "gv_") || !got.Minted {
		t.Fatalf("expected minted grant, got %+v", got)
	}
	if !p.Accepts(got.Token) {
		t.Fatal("minted grant should be accepted")
	}
	if p.Accepts("view-077") {
		t.Fatal("env view token is not a minted grant")
	}
	if p.Accepts("") {
		t.Fatal("empty token")
	}
}

func TestPairing_MintedGrantExpires(t *testing.T) {
	p := NewPairing()
	base := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return base }
	issued, err := p.Issue("")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Exchange(issued.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Accepts(got.Token) {
		t.Fatal("grant live")
	}
	p.now = func() time.Time { return base.Add(PairingTTL) }
	if p.Accepts(got.Token) {
		t.Fatal("grant past TTL")
	}
}
