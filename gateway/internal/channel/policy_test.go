// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPolicy_Defaults(t *testing.T) {
	t.Setenv("GOSO_ENV", "")
	tg := DefaultPolicy("telegram")
	if tg.DMPolicy != "pairing" || tg.GroupPolicy != "allowlist" || !tg.RequireMention {
		t.Fatalf("telegram %+v", tg)
	}
	t.Setenv("GOSO_ENV", "demo")
	tg = DefaultPolicy("telegram")
	if tg.DMPolicy != "open" {
		t.Fatalf("demo telegram dm %s", tg.DMPolicy)
	}
	pp := DefaultPolicy("zalo-personal")
	if pp.DMPolicy != "allowlist" || pp.GroupPolicy != "allowlist" {
		t.Fatalf("personal never open by default %+v", pp)
	}
	oa := DefaultPolicy("zalo-oa")
	if oa.DMPolicy != "pairing" || oa.GroupPolicy != "disabled" {
		t.Fatalf("oa %+v", oa)
	}
}

func TestPolicy_DMMatrix(t *testing.T) {
	in := Inbound{Channel: "telegram", SenderID: "u1", ChatID: "u1", PeerKind: "direct", Text: "hi"}
	cases := []struct {
		dm     string
		allow  []string
		paired bool
		want   PolicyAction
	}{
		{"open", nil, false, PolicyAccept},
		{"disabled", nil, false, PolicyReject},
		{"allowlist", nil, false, PolicyReject},
		{"allowlist", []string{"u1"}, false, PolicyAccept},
		{"pairing", nil, false, PolicyNeedPairing},
		{"pairing", nil, true, PolicyAccept},
		{"pairing", []string{"u1"}, false, PolicyAccept},
	}
	for _, c := range cases {
		got := CheckPolicy("telegram", Policy{DMPolicy: c.dm, AllowFrom: c.allow}, in, c.paired)
		if got != c.want {
			t.Fatalf("dm=%s allow=%v paired=%v got %s want %s", c.dm, c.allow, c.paired, got, c.want)
		}
	}
}

func TestPolicy_GroupMention(t *testing.T) {
	in := Inbound{Channel: "telegram", SenderID: "u1", ChatID: "-9", PeerKind: "group", Text: "hi", Mention: false}
	p := Policy{GroupPolicy: "open", RequireMention: true}
	if got := CheckPolicy("telegram", p, in, false); got != PolicyNeedMention {
		t.Fatalf("no mention %s", got)
	}
	in.Mention = true
	if got := CheckPolicy("telegram", p, in, false); got != PolicyAccept {
		t.Fatalf("mention %s", got)
	}
	p.GroupPolicy = "allowlist"
	in.Mention = true
	if got := CheckPolicy("telegram", p, in, false); got != PolicyReject {
		t.Fatalf("group not allowed %s", got)
	}
	p.AllowFrom = []string{"-9"}
	if got := CheckPolicy("telegram", p, in, false); got != PolicyAccept {
		t.Fatalf("group allow %s", got)
	}
	p.GroupPolicy = "disabled"
	if got := CheckPolicy("zalo-oa", p, in, false); got != PolicyReject {
		t.Fatalf("oa group %s", got)
	}
}

func TestPolicy_MergePersonalNeverOpenDefault(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	p := MergePolicy("zalo-personal", nil)
	if p.DMPolicy != "allowlist" {
		t.Fatalf("demo personal %s", p.DMPolicy)
	}
	cfg := &store.ChannelConfig{Name: "zalo-personal", DMPolicy: "open"}
	p = MergePolicy("zalo-personal", cfg)
	if p.DMPolicy != "open" {
		t.Fatalf("explicit patch open %s", p.DMPolicy)
	}
}

func TestPolicy_Debounce(t *testing.T) {
	d := NewPairingDebounce(60 * time.Second)
	now := time.Now().UTC()
	if !d.ShouldSend("telegram", "u1", now) {
		t.Fatal("first")
	}
	if d.ShouldSend("telegram", "u1", now.Add(30*time.Second)) {
		t.Fatal("debounced")
	}
	if !d.ShouldSend("telegram", "u1", now.Add(61*time.Second)) {
		t.Fatal("after ttl")
	}
}
