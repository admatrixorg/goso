// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTenant_ReservedNameAndTraversal(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	if err := os.MkdirAll(filepath.Join(root, "t"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "t", "SKILL.md"), []byte("namespace is not a skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tenant := range []string{"", "minh-an-sales"} {
		if _, err := Create(tenant, "t", "nope"); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("create t tenant %q: %v", tenant, err)
		}
		if _, err := Load(tenant, "t"); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("load t tenant %q: %v", tenant, err)
		}
	}
	list, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range list {
		if info.Name == "t" {
			t.Fatalf("listed reserved name %+v", list)
		}
	}

	parent := filepath.Dir(root)
	for _, tenant := range []string{"..", "../x", `..\x`, "foo..bar", "a/b", `a\b`, ".", "not a tenant"} {
		if _, err := Create(tenant, "pwned", "nope"); !errors.Is(err, ErrPathEscape) {
			t.Fatalf("create tenant %q: %v", tenant, err)
		}
		if _, err := Load(tenant, "pwned"); !errors.Is(err, ErrPathEscape) {
			t.Fatalf("load tenant %q: %v", tenant, err)
		}
		if _, err := List(tenant); !errors.Is(err, ErrPathEscape) {
			t.Fatalf("list tenant %q: %v", tenant, err)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("traversal wrote outside the skills dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("rejected tenant still wrote a root skill: %v", err)
	}
}

func TestTenant_SymlinkNamespaceRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := []byte("outside-minh-an")
	if err := os.MkdirAll(filepath.Join(outside, "bao-gia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "bao-gia", "SKILL.md"), secret, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "t")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")

	if _, err := List("minh-an-sales"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("list symlink t: %v", err)
	}
	if _, err := Load("minh-an-sales", "bao-gia"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("load symlink t: %v", err)
	}
	if _, err := Create("minh-an-sales", "bao-gia", "nope"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("create symlink t: %v", err)
	}
	if _, err := Search("minh-an-sales", "outside"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("search symlink t: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outside, "bao-gia", "SKILL.md"))
	if err != nil || string(got) != string(secret) {
		t.Fatalf("symlink write changed outside file %v %q", err, got)
	}

	if err := os.Remove(filepath.Join(root, "t")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "t"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "t", "minh-an-sales")); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("minh-an-sales", "bao-gia", "nope"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("create symlink tenant: %v", err)
	}
	if err := Delete("minh-an-sales", "bao-gia"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("delete symlink tenant: %v", err)
	}
	got, err = os.ReadFile(filepath.Join(outside, "bao-gia", "SKILL.md"))
	if err != nil || string(got) != string(secret) {
		t.Fatalf("tenant symlink changed outside file %v %q", err, got)
	}
}

func TestTenant_SharedReadOnlyOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	invalidateIndex()

	shared := "---\ndescription: shared counsel\n---\n# Shared\n"
	doc, err := Create("", "tu-van", shared)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "tu-van/SKILL.md" {
		t.Fatalf("default path %s", doc.Path)
	}
	salesBody := "---\ndescription: riengsales\n---\nCố vấn Minh An\n"
	doc, err = Create("minh-an-sales", "tu-van", salesBody)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "t/minh-an-sales/tu-van/SKILL.md" {
		t.Fatalf("sales path %s", doc.Path)
	}
	rootBytes, err := os.ReadFile(filepath.Join(root, "tu-van", "SKILL.md"))
	if err != nil || string(rootBytes) != shared {
		t.Fatalf("root skill changed %v %q", err, rootBytes)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "default")); !os.IsNotExist(err) {
		t.Fatalf("default tenant must not grow t/default: %v", err)
	}

	got, err := Load("", "tu-van")
	if err != nil || got.Body != shared || got.Path != "tu-van/SKILL.md" {
		t.Fatalf("default load %+v %v", got, err)
	}
	got, err = Load("minh-an-sales", "tu-van")
	if err != nil || got.Body != salesBody || !TenantOwned("minh-an-sales", got.Path) {
		t.Fatalf("sales load %+v %v", got, err)
	}
	got, err = Load("minh-an-legal", "tu-van")
	if err != nil || got.Body != shared {
		t.Fatalf("legal should see shared %+v %v", got, err)
	}

	if _, err := Create("minh-an-sales", "bao-gia", "Cố vấn Minh An bao gia"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "bao-gia")); !os.IsNotExist(err) {
		t.Fatalf("non-default create wrote the shared root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an-sales", "bao-gia", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(filepath.Join(root, "t", "minh-an-sales", "bao-gia"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}

	if err := Delete("minh-an-legal", "tu-van"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legal delete of shared skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tu-van", "SKILL.md")); err != nil {
		t.Fatalf("shared skill removed: %v", err)
	}
	if err := Delete("", "bao-gia"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("default delete of a tenant skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an-sales", "bao-gia", "SKILL.md")); err != nil {
		t.Fatalf("default delete removed the tenant skill: %v", err)
	}

	if err := Delete("minh-an-sales", "tu-van"); err != nil {
		t.Fatal(err)
	}
	got, err = Load("minh-an-sales", "tu-van")
	if err != nil || got.Body != shared {
		t.Fatalf("after deleting the override, shared should show through %+v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "tu-van", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestTenant_SearchIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	invalidateIndex()

	if _, err := Create("", "tu-van", "---\ndescription: shared counsel\n---\nCố vấn Minh An\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("minh-an-sales", "bao-gia", "---\ndescription: baogia minh an sales\n---\nCố vấn Minh An\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("minh-an-legal", "hop-dong", "---\ndescription: phapche minh an legal\n---\nCố vấn Minh An\n"); err != nil {
		t.Fatal(err)
	}

	sales, err := Search("minh-an-sales", "baogia")
	if err != nil || len(sales) != 1 || sales[0].Name != "bao-gia" {
		t.Fatalf("sales search %+v %v", sales, err)
	}
	legal, err := Search("minh-an-legal", "baogia")
	if err != nil || len(legal) != 0 {
		t.Fatalf("legal must not see the sales skill %+v %v", legal, err)
	}
	legal, err = Search("minh-an-legal", "phapche")
	if err != nil || len(legal) != 1 || legal[0].Name != "hop-dong" {
		t.Fatalf("legal search %+v %v", legal, err)
	}
	sales, err = Search("minh-an-sales", "phapche")
	if err != nil || len(sales) != 0 {
		t.Fatalf("sales must not see the legal skill %+v %v", sales, err)
	}
	for _, tenant := range []string{"", "minh-an-sales", "minh-an-legal"} {
		hits, err := Search(tenant, "counsel")
		if err != nil || len(hits) != 1 || hits[0].Name != "tu-van" {
			t.Fatalf("tenant %s shared search %+v %v", tenant, hits, err)
		}
	}

	if _, err := Create("minh-an-sales", "tu-van", "---\ndescription: riengsales\n---\nCố vấn Minh An\n"); err != nil {
		t.Fatal(err)
	}
	sales, err = Search("minh-an-sales", "riengsales")
	if err != nil || len(sales) != 1 || sales[0].Name != "tu-van" || !strings.Contains(sales[0].Snippet, "riengsales") {
		t.Fatalf("override search %+v %v", sales, err)
	}
	legal, err = Search("minh-an-legal", "riengsales")
	if err != nil || len(legal) != 0 {
		t.Fatalf("other tenant indexed the override %+v %v", legal, err)
	}
	legal, err = Search("minh-an-legal", "counsel")
	if err != nil || len(legal) != 1 || !strings.Contains(legal[0].Snippet, "shared counsel") {
		t.Fatalf("legal shared snippet %+v %v", legal, err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			hits, err := Search("minh-an-sales", "baogia")
			if err != nil || len(hits) != 1 || hits[0].Name != "bao-gia" {
				t.Errorf("parallel sales %+v %v", hits, err)
			}
		}()
		go func() {
			defer wg.Done()
			hits, err := Search("minh-an-legal", "phapche")
			if err != nil || len(hits) != 1 || hits[0].Name != "hop-dong" {
				t.Errorf("parallel legal %+v %v", hits, err)
			}
		}()
	}
	wg.Wait()
}
