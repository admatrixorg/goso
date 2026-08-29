// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestContacts_ObserveListOmitsPayloads(t *testing.T) {
	c := NewContacts()
	secret := "bot_token=12345:AA sk-live-ABCDEFG"
	g := c.Observe(Sighting{Channel: "telegram", Dest: "-1001", Kind: "group", AgentID: "ag1", At: time.Unix(1, 0).UTC()})
	c.Observe(Sighting{Channel: "telegram", Dest: "-1001", Kind: "group", AgentID: "ag1", At: time.Unix(5, 0).UTC()})
	if g.ID == "" || g.Count != 1 {
		t.Fatalf("first %#v", g)
	}
	list := c.List("", "", "", "")
	if len(list) != 1 || list[0].Count != 2 {
		t.Fatalf("list %#v", list)
	}
	if list[0].Channel != "telegram" || list[0].Dest != "-1001" || list[0].Kind != "group" {
		t.Fatalf("fields %#v", list[0])
	}
	if list[0].Permission != "group" {
		t.Fatalf("permission %#v", list[0])
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "token") || strings.Contains(string(raw), "content") {
		t.Fatalf("payload leak %s", raw)
	}
	for _, leak := range []string{`"token"`, `"secret"`, `"code"`, `"content"`, `"text"`, `"bot_token"`} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("leaked %s in %s", leak, raw)
		}
	}
}

func TestContacts_SearchFilterAndSenderSplit(t *testing.T) {
	c := NewContacts()
	c.Observe(Sighting{Channel: "telegram", Dest: "-9", Kind: "group", SenderID: "42", At: time.Unix(2, 0).UTC()})
	c.Observe(Sighting{Channel: "discord", Dest: "room", Kind: "user", At: time.Unix(3, 0).UTC()})
	all := c.List("", "", "", "")
	if len(all) != 3 {
		t.Fatalf("want group+sender+discord, got %#v", all)
	}
	tg := c.List("", "", "telegram", "")
	if len(tg) != 2 {
		t.Fatalf("telegram %#v", tg)
	}
	users := c.List("", "", "", "user")
	if len(users) != 2 {
		t.Fatalf("users %#v", users)
	}
	hit := c.List("", "room", "", "")
	if len(hit) != 1 || hit[0].Dest != "room" {
		t.Fatalf("search %#v", hit)
	}
}

func TestContacts_MergeKeepsIdentifiersAndUndo(t *testing.T) {
	c := NewContacts()
	a := c.Observe(Sighting{Channel: "telegram", Dest: "111", Kind: "user"})
	b := c.Observe(Sighting{Channel: "discord", Dest: "222", Kind: "user"})
	if _, err := c.Merge(a.ID, b.ID, "", ""); !errors.Is(err, ErrContactConfirmRequired) {
		t.Fatalf("empty confirm %v", err)
	}
	if _, err := c.Merge(a.ID, b.ID, "", "wrong"); !errors.Is(err, ErrContactConfirm) {
		t.Fatalf("mismatch %v", err)
	}
	if _, err := c.Merge(a.ID, a.ID, "", a.ID); !errors.Is(err, ErrContactSelfMerge) {
		t.Fatalf("self %v", err)
	}
	out, err := c.Merge(a.ID, b.ID, "", b.ID+">"+a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Identifiers) != 2 {
		t.Fatalf("idents %#v", out.Identifiers)
	}
	if !out.CanUndo || len(out.MergedFrom) != 1 || out.MergedFrom[0] != b.ID {
		t.Fatalf("provenance %#v", out)
	}
	list := c.List("", "", "", "")
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("after merge %#v", list)
	}
	if _, err := c.Get(b.ID, ""); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("merged source still visible %v", err)
	}
	got, err := c.Get(a.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	hasTG, hasDC := false, false
	for _, id := range got.Identifiers {
		if id.Channel == "telegram" && id.Dest == "111" {
			hasTG = true
		}
		if id.Channel == "discord" && id.Dest == "222" {
			hasDC = true
		}
	}
	if !hasTG || !hasDC {
		t.Fatalf("lost identifiers %#v", got.Identifiers)
	}

	if _, err := c.Undo(a.ID, "", ""); !errors.Is(err, ErrContactConfirmRequired) {
		t.Fatalf("undo empty %v", err)
	}
	if _, err := c.Undo(a.ID, "", "nope"); !errors.Is(err, ErrContactConfirm) {
		t.Fatalf("undo mismatch %v", err)
	}
	undone, err := c.Undo(a.ID, "", b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.CanUndo {
		t.Fatalf("can_undo after undo %#v", undone)
	}
	list = c.List("", "", "", "")
	if len(list) != 2 {
		t.Fatalf("after undo %#v", list)
	}
	src, err := c.Get(b.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if src.Dest != "222" || len(src.Identifiers) != 1 {
		t.Fatalf("restored %#v", src)
	}
}

func TestContacts_TenantIsolation(t *testing.T) {
	c := NewContacts()
	c.Observe(Sighting{Channel: "telegram", Dest: "1", TenantID: "alpha"})
	c.Observe(Sighting{Channel: "telegram", Dest: "2", TenantID: "beta"})
	if n := len(c.List("alpha", "", "", "")); n != 1 {
		t.Fatalf("alpha %d", n)
	}
	if n := len(c.List("beta", "", "", "")); n != 1 {
		t.Fatalf("beta %d", n)
	}
	alpha := c.List("alpha", "", "", "")[0]
	if _, err := c.Get(alpha.ID, "beta"); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("cross-tenant get %v", err)
	}
}
