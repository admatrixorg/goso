// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestChannelConfig_PutGet(t *testing.T) {
	t.Parallel()
	for _, s := range []StoreIface{New(), mustSQLiteChannels(t)} {
		if err := s.PutChannelConfig(ChannelConfig{
			Name: "telegram", Enabled: true, AgentID: "a1",
			DMPolicy: "pairing", GroupPolicy: "allowlist", RequireMention: true,
			AllowFrom: []string{"111"},
		}); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetChannelConfig("telegram")
		if err != nil || !got.Enabled || got.AgentID != "a1" || got.DMPolicy != "pairing" || !got.RequireMention {
			t.Fatalf("get %v %+v", err, got)
		}
		if len(got.AllowFrom) != 1 || got.AllowFrom[0] != "111" {
			t.Fatalf("allow_from %+v", got.AllowFrom)
		}
		list := s.ListChannelConfigs()
		if len(list) != 1 {
			t.Fatalf("list %d", len(list))
		}
		if _, err := s.GetChannelConfig("missing"); err != ErrNotFound {
			t.Fatalf("missing %v", err)
		}
	}
}

func TestChannelPairing_PendingCap(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	for _, s := range []StoreIface{New(), mustSQLiteChannels(t)} {
		for i := 0; i < 3; i++ {
			if _, err := s.CreateChannelPairing(ChannelPairing{
				Channel: "telegram", SenderID: "u1", CodeHash: "h", Status: "pending",
				ExpiresAt: now.Add(time.Hour),
			}); err != nil {
				t.Fatal(err)
			}
		}
		if n := s.CountPendingChannelPairings("telegram", "u1", now); n != 3 {
			t.Fatalf("pending %d", n)
		}
		expired, err := s.CreateChannelPairing(ChannelPairing{
			Channel: "telegram", SenderID: "u1", CodeHash: "x", Status: "pending",
			ExpiresAt: now.Add(-time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := s.CountPendingChannelPairings("telegram", "u1", now); n != 3 {
			t.Fatalf("expired not counted want 3 got %d", n)
		}
		got, err := s.GetChannelPairing(expired.ID)
		if err != nil || got.CodeHash != "x" {
			t.Fatalf("get %v %+v", err, got)
		}
		got.Status = "approved"
		got.ApprovedAt = now
		if _, err := s.UpdateChannelPairing(*got); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeleteSecret(t *testing.T) {
	t.Parallel()
	for _, s := range []StoreIface{New(), mustSQLiteChannels(t)} {
		if err := s.PutSecret(SecretRow{Name: "channel:zalo-personal:session", Nonce: []byte("n"), CT: []byte("c")}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetSecret("channel:zalo-personal:session"); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteSecret("channel:zalo-personal:session"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetSecret("channel:zalo-personal:session"); err != ErrNotFound {
			t.Fatalf("after delete %v", err)
		}
		if err := s.DeleteSecret("channel:zalo-personal:session"); err != nil {
			t.Fatalf("idempotent delete %v", err)
		}
	}
}

func mustSQLiteChannels(t *testing.T) StoreIface {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
