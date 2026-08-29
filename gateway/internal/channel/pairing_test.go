// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPairing_AlphabetAndTTL(t *testing.T) {
	st := store.New()
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	issued, err := IssuePairing(st, "telegram", "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(issued.Code) != 8 {
		t.Fatalf("len %d", len(issued.Code))
	}
	for _, r := range issued.Code {
		if !strings.ContainsRune(PairingAlphabet, r) {
			t.Fatalf("rune %c not in alphabet", r)
		}
	}
	if strings.ContainsAny(issued.Code, "0O1IL") {
		t.Fatalf("ambiguous %s", issued.Code)
	}
	if issued.ExpiresAt.Sub(now) != 60*time.Minute {
		t.Fatalf("ttl %v", issued.ExpiresAt.Sub(now))
	}
	row, err := st.GetChannelPairing(issued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.CodeHash == "" || row.CodeHash == issued.Code {
		t.Fatalf("must store hash not plaintext: %q", row.CodeHash)
	}
}

func TestPairing_MaxThreePending(t *testing.T) {
	st := store.New()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if _, err := IssuePairing(st, "telegram", "u1", now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := IssuePairing(st, "telegram", "u1", now); err == nil {
		t.Fatal("want cap error")
	}
	if _, err := IssuePairing(st, "telegram", "u2", now); err != nil {
		t.Fatal(err)
	}
}

func TestPairing_ApproveAndLookup(t *testing.T) {
	st := store.New()
	now := time.Now().UTC()
	issued, err := IssuePairing(st, "zalo-oa", "s1", now)
	if err != nil {
		t.Fatal(err)
	}
	if SenderPaired(st, "zalo-oa", "s1", now) {
		t.Fatal("not approved yet")
	}
	if err := ApprovePairing(st, issued.ID, now); err != nil {
		t.Fatal(err)
	}
	if !SenderPaired(st, "zalo-oa", "s1", now) {
		t.Fatal("want paired")
	}
	if err := DenyPairing(st, issued.ID, now); err == nil {
		t.Fatal("already approved")
	}
}
