// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"strings"
	"testing"
)

func TestRegistryMasterAlwaysPresent(t *testing.T) {
	r := New()
	list := r.List("")
	if len(list) != 1 || !list[0].Master || list[0].ID != Default || list[0].Status != StatusActive {
		t.Fatalf("master %#v", list)
	}
	if !r.Writable(Default) {
		t.Fatal("master should be writable")
	}
}

func TestRegistryCreateListSearch(t *testing.T) {
	r := New()
	p, err := r.Create("acme", "Acme Corp")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "acme" || p.Name != "Acme Corp" || p.Status != StatusActive || p.Master {
		t.Fatalf("created %#v", p)
	}
	if _, err := r.Create("acme", "Dup"); err != ErrExists {
		t.Fatalf("dup %v", err)
	}
	if _, err := r.Create("not valid!", "X"); err != ErrSlug {
		t.Fatalf("slug %v", err)
	}
	if _, err := r.Create("beta", ""); err != ErrName {
		t.Fatalf("name %v", err)
	}
	if _, err := r.Create("gamma", "sk-live-abcdefgh"); err != ErrSecret {
		t.Fatalf("secret name %v", err)
	}
	got := r.List("acme")
	if len(got) != 1 || got[0].ID != "acme" {
		t.Fatalf("search %#v", got)
	}
	if len(r.List("nope")) != 0 {
		t.Fatalf("empty search %#v", r.List("nope"))
	}
}

func TestRegistryDeactivateConfirmAndMasterGuard(t *testing.T) {
	r := New()
	if _, err := r.Create("acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetStatus(Default, StatusDeactivated, Default); err != ErrMaster {
		t.Fatalf("master %v", err)
	}
	if _, err := r.SetStatus("acme", StatusDeactivated, ""); err != ErrConfirmRequired {
		t.Fatalf("confirm req %v", err)
	}
	if _, err := r.SetStatus("acme", StatusDeactivated, "nope"); err != ErrConfirm {
		t.Fatalf("mismatch %v", err)
	}
	p, err := r.SetStatus("acme", StatusDeactivated, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusDeactivated {
		t.Fatalf("status %#v", p)
	}
	if r.Writable("acme") {
		t.Fatal("deactivated still writable")
	}
	if !r.Writable("ghost") {
		t.Fatal("unregistered should stay writable")
	}
	p, err = r.SetStatus("acme", StatusActive, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusActive || !r.Writable("acme") {
		t.Fatalf("reactivate %#v", p)
	}
}

func TestRegistryMembersRolesNoSecrets(t *testing.T) {
	r := New()
	if _, err := r.Create("acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.AddMember("acme", "Bearer abcdefghijk", RoleAdmin); err != ErrSecret {
		t.Fatalf("secret subject %v", err)
	}
	row, mem, err := r.AddMember("acme", "ops@acme.test", RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if mem.Role != RoleAdmin || mem.Subject != "ops@acme.test" || mem.ID == "" {
		t.Fatalf("member %#v", mem)
	}
	if len(row.Members) != 1 {
		t.Fatalf("members %#v", row.Members)
	}
	if _, _, err := r.AddMember("acme", "ops@acme.test", RoleViewer); err != ErrMemberExists {
		t.Fatalf("dup %v", err)
	}
	row, mem, err = r.SetMemberRole("acme", mem.ID, RoleViewer)
	if err != nil || mem.Role != RoleViewer || row.Members[0].Role != RoleViewer {
		t.Fatalf("role %v %#v", err, mem)
	}
	if _, _, err := r.RemoveMember("acme", mem.ID, ""); err != ErrConfirmRequired {
		t.Fatalf("confirm %v", err)
	}
	if _, _, err := r.RemoveMember("acme", mem.ID, "nope"); err != ErrConfirm {
		t.Fatalf("mismatch %v", err)
	}
	row, _, err = r.RemoveMember("acme", mem.ID, "ops@acme.test")
	if err != nil || len(row.Members) != 0 {
		t.Fatalf("remove %v %#v", err, row.Members)
	}
}

func TestRegistryContextUnregistered(t *testing.T) {
	r := New()
	ctx := r.Context("acme")
	if ctx.ID != "acme" || ctx.Status != StatusActive || ctx.Master {
		t.Fatalf("ctx %#v", ctx)
	}
	if _, err := r.Get("acme"); err != ErrNotFound {
		t.Fatalf("get unregistered %v", err)
	}
	master := r.Master()
	if master.ID != Default || !master.Master {
		t.Fatalf("master %#v", master)
	}
}

func TestPublicHasNoSecretFields(t *testing.T) {
	r := New()
	p, _ := r.Create("acme", "Acme")
	_, mem, _ := r.AddMember("acme", "ops@acme.test", RoleOwner)
	got, _ := r.Get("acme")
	blob := p.ID + p.Name + p.Status + got.Members[0].Subject + mem.Role
	if strings.Contains(strings.ToLower(blob), "token") || strings.Contains(blob, "sk-") {
		t.Fatalf("secret-shaped public %q", blob)
	}
}
