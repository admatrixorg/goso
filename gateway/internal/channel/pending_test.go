// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPending_EnqueueListOmitsPayloads(t *testing.T) {
	p := NewPending()
	secret := "bot_token=12345:AA sk-live-ABCDEFG"
	g := p.Enqueue(Enqueue{Channel: "telegram", Dest: "-1001", AgentID: "ag1", At: time.Unix(1, 0).UTC()})
	p.Enqueue(Enqueue{Channel: "telegram", Dest: "-1001", AgentID: "ag1", At: time.Unix(5, 0).UTC()})
	if g.ID == "" || g.Count != 1 {
		t.Fatalf("first %#v", g)
	}
	list := p.List("", time.Unix(10, 0).UTC())
	if len(list) != 1 || list[0].Count != 2 {
		t.Fatalf("list %#v", list)
	}
	if list[0].Channel != "telegram" || list[0].Dest != "-1001" || list[0].AgentID != "ag1" {
		t.Fatalf("fields %#v", list[0])
	}
	if list[0].AgeMS != 9000 {
		t.Fatalf("age %d", list[0].AgeMS)
	}
	raw := list[0].ID + list[0].Channel + list[0].Dest + list[0].AgentID
	if strings.Contains(raw, secret) || strings.Contains(raw, "token") {
		t.Fatalf("payload leak %q", raw)
	}
}

func TestPending_CompactRequiresConfirmAndDropsCount(t *testing.T) {
	p := NewPending()
	g := p.Enqueue(Enqueue{Channel: "discord", Dest: "c9", AgentID: "a"})
	p.Enqueue(Enqueue{Channel: "discord", Dest: "c9", AgentID: "a"})
	p.Enqueue(Enqueue{Channel: "discord", Dest: "c9", AgentID: "a"})
	if _, err := p.Compact(g.ID, "", ""); !errors.Is(err, ErrPendingConfirmRequired) {
		t.Fatalf("empty confirm %v", err)
	}
	if _, err := p.Compact(g.ID, "", "wrong"); !errors.Is(err, ErrPendingConfirm) {
		t.Fatalf("mismatch %v", err)
	}
	out, err := p.Compact(g.ID, "", "discord:c9")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Compacted || out.Count != 1 || out.CompactedFrom != 3 {
		t.Fatalf("compacted %#v", out)
	}
}

func TestPending_ClearAndBusy(t *testing.T) {
	p := NewPending()
	g := p.Enqueue(Enqueue{Channel: "slack", Dest: "d1"})
	p.hook = func() {
		if err := p.Clear(g.ID, "", "d1"); !errors.Is(err, ErrPendingBusy) {
			t.Errorf("clear during compact %v", err)
		}
		if _, err := p.Compact(g.ID, "", "d1"); !errors.Is(err, ErrPendingBusy) {
			t.Errorf("second compact %v", err)
		}
		list := p.List("", time.Time{})
		if len(list) != 1 || !list[0].Compacting {
			t.Errorf("compacting flag %#v", list)
		}
	}
	if _, err := p.Compact(g.ID, "", "d1"); err != nil {
		t.Fatal(err)
	}
	if err := p.Clear(g.ID, "", "d1"); err != nil {
		t.Fatal(err)
	}
	if len(p.List("", time.Time{})) != 0 {
		t.Fatal("expected empty after clear")
	}
	if err := p.Clear(g.ID, "", "d1"); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("missing %v", err)
	}
}

func TestPending_TenantIsolation(t *testing.T) {
	p := NewPending()
	p.Enqueue(Enqueue{Channel: "telegram", Dest: "1", TenantID: "alpha"})
	p.Enqueue(Enqueue{Channel: "telegram", Dest: "2", TenantID: "beta"})
	a := p.List("alpha", time.Time{})
	if len(a) != 1 || a[0].Dest != "1" {
		t.Fatalf("alpha %#v", a)
	}
	b := p.List("beta", time.Time{})
	if len(b) != 1 || b[0].Dest != "2" {
		t.Fatalf("beta %#v", b)
	}
}

func TestBufferIfNeeded_DisabledAndExisting(t *testing.T) {
	p := NewPending()
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "tg", DisplayName: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if BufferIfNeeded(p, a, "telegram", "99") {
		t.Fatal("enabled agent without buffer should process live")
	}
	off := *a
	off.Enabled = false
	off.UpdatedAt = a.Stamp()
	if _, err := st.UpdateAgent(off); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAgent(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !BufferIfNeeded(p, got, "telegram", "99") {
		t.Fatal("disabled should buffer")
	}
	got.Enabled = true
	if !BufferIfNeeded(p, got, "telegram", "99") {
		t.Fatal("existing group should keep buffering")
	}
	if n := p.List("", time.Time{}); len(n) != 1 || n[0].Count != 2 {
		t.Fatalf("buffered %#v", n)
	}
}

func TestPending_ConcurrentEnqueue(t *testing.T) {
	p := NewPending()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Enqueue(Enqueue{Channel: "feishu", Dest: "chat"})
		}()
	}
	wg.Wait()
	list := p.List("", time.Time{})
	if len(list) != 1 || list[0].Count != 20 {
		t.Fatalf("count %#v", list)
	}
}
