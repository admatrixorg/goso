// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package impexp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestSkillPortable_TenantScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	if _, err := skill.Create("", "tu-van", "shared counsel"); err != nil {
		t.Fatal(err)
	}
	st := store.New()
	svc := New(st)

	raw := mustJSON(t, Archive{
		Schema: Schema, SchemaVersion: SchemaVersion,
		Skills: []SkillItem{{Name: "bao-gia", Body: "Cố vấn Minh An bao gia"}},
	})
	imp, err := svc.Import("minh-an-sales", raw, ConflictSkip, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Report.Created) != 1 || imp.Report.Created[0].Name != "bao-gia" {
		t.Fatalf("import %+v", imp.Report)
	}
	if _, err := os.Stat(filepath.Join(root, "bao-gia")); !os.IsNotExist(err) {
		t.Fatalf("import wrote the shared root: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "t", "minh-an-sales", "bao-gia", "SKILL.md"))
	if err != nil || !strings.Contains(string(got), "bao gia") {
		t.Fatalf("tenant file %v %q", err, got)
	}

	cat := svc.Catalog("minh-an-sales")
	if !catalogHas(cat, "bao-gia", "t/minh-an-sales/bao-gia/SKILL.md") || !catalogHas(cat, "tu-van", "tu-van/SKILL.md") {
		t.Fatalf("sales catalog %+v", cat.Skills)
	}
	def := svc.Catalog("")
	if catalogHas(def, "bao-gia", "") || !catalogHas(def, "tu-van", "tu-van/SKILL.md") {
		t.Fatalf("default catalog %+v", def.Skills)
	}
	legal := svc.Catalog("minh-an-legal")
	if catalogHas(legal, "bao-gia", "") {
		t.Fatalf("legal catalog leaked %+v", legal.Skills)
	}

	leaked, err := svc.Export("minh-an-legal", Selection{SkillNames: []string{"bao-gia"}})
	if err != ErrEmptySelection {
		t.Fatalf("legal export %v", err)
	}
	blob, _ := json.Marshal(leaked)
	if strings.Contains(string(blob), "bao gia") {
		t.Fatalf("export leaked the other tenant: %s", blob)
	}
	exp, err := svc.Export("minh-an-sales", Selection{SkillNames: []string{"bao-gia", "tu-van"}})
	if err != nil {
		t.Fatal(err)
	}
	foundSales, foundShared := false, false
	for _, sk := range exp.Archive.Skills {
		if sk.Name == "bao-gia" && strings.Contains(sk.Body, "bao gia") {
			foundSales = true
		}
		if sk.Name == "tu-van" && strings.Contains(sk.Body, "shared counsel") {
			foundShared = true
		}
	}
	if !foundSales || !foundShared {
		t.Fatalf("export %+v", exp.Archive.Skills)
	}

	rb, err := svc.Rollback(imp.ID)
	if err != nil || rb.Status != StatusDone {
		t.Fatalf("rollback %v %+v", err, rb)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an-sales", "bao-gia")); !os.IsNotExist(err) {
		t.Fatalf("rollback left the tenant skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tu-van", "SKILL.md")); err != nil {
		t.Fatalf("rollback touched the shared skill: %v", err)
	}

	overRaw := mustJSON(t, Archive{
		Schema: Schema, SchemaVersion: SchemaVersion,
		Skills: []SkillItem{{Name: "tu-van", Body: "Cố vấn Minh An override"}},
	})
	over, err := svc.Import("minh-an-sales", overRaw, ConflictOverwrite, false)
	if err != nil {
		t.Fatal(err)
	}
	rootBody, err := os.ReadFile(filepath.Join(root, "tu-van", "SKILL.md"))
	if err != nil || string(rootBody) != "shared counsel" {
		t.Fatalf("overwrite rewrote the shared root %v %q", err, rootBody)
	}
	own, err := skill.Load("minh-an-sales", "tu-van")
	if err != nil || !strings.Contains(own.Body, "override") {
		t.Fatalf("override missing %+v %v", own, err)
	}
	if _, err := svc.Rollback(over.ID); err != nil {
		t.Fatal(err)
	}
	own, err = skill.Load("minh-an-sales", "tu-van")
	if err != nil || own.Body != "shared counsel" || own.Path != "tu-van/SKILL.md" {
		t.Fatalf("rollback kept the override %+v %v", own, err)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an-sales", "tu-van")); !os.IsNotExist(err) {
		t.Fatalf("override file remained: %v", err)
	}
}

func catalogHas(cat Catalog, name, path string) bool {
	for _, sk := range cat.Skills {
		if sk.Name != name {
			continue
		}
		if path == "" || sk.Path == path {
			return true
		}
	}
	return false
}
