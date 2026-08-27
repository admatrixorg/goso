// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestParseWikilinks(t *testing.T) {
	got := ParseWikilinks("see [[Beta]] and [[Alpha|alias]] and [[Beta]] again [[  id-1  ]]")
	if len(got) != 3 || got[0] != "Beta" || got[1] != "Alpha" || got[2] != "id-1" {
		t.Fatalf("got %#v", got)
	}
	if n := ParseWikilinks("no links"); n != nil && len(n) != 0 {
		t.Fatalf("empty %#v", n)
	}
}

func TestSlug(t *testing.T) {
	if Slug("Other Note") != "other-note" {
		t.Fatalf("slug %q", Slug("Other Note"))
	}
	if Slug("  ") != "note" {
		t.Fatalf("blank slug %q", Slug("  "))
	}
}

func TestSync_BidirectionalLinksAndSkipDelete(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.md"), []byte("# Alpha\n\nSee [[Beta]] pineapple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.txt"), []byte("# Beta\n\nSee [[Alpha]] mango\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.New()
	svc := New(st, root)
	res, err := svc.Sync()
	if err != nil || res.Upserted != 2 {
		t.Fatalf("sync %v %#v", err, res)
	}
	docs := svc.List()
	if len(docs) != 2 {
		t.Fatalf("docs %d", len(docs))
	}
	var alpha, beta *store.VaultDoc
	for _, d := range docs {
		switch d.Title {
		case "Alpha":
			alpha = d
		case "Beta":
			beta = d
		}
	}
	if alpha == nil || beta == nil {
		t.Fatalf("titles %#v", docs)
	}
	ob, ib, err := svc.Links(alpha.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != beta.ID || len(ib) != 1 || ib[0].FromID != beta.ID {
		t.Fatalf("alpha edges %v %#v %#v", err, ob, ib)
	}
	obB, ibB, err := svc.Links(beta.ID)
	if err != nil || len(obB) != 1 || obB[0].ToID != alpha.ID || len(ibB) != 1 {
		t.Fatalf("beta edges %v %#v %#v", err, obB, ibB)
	}
	hits, err := svc.Search("pineapple")
	if err != nil || len(hits) == 0 {
		t.Fatalf("search %v %#v", err, hits)
	}

	res2, err := svc.Sync()
	if err != nil || res2.Skipped != 2 || res2.Upserted != 0 {
		t.Fatalf("skip %v %#v", err, res2)
	}

	if err := os.Remove(filepath.Join(root, "beta.txt")); err != nil {
		t.Fatal(err)
	}
	res3, err := svc.Sync()
	if err != nil || res3.Deleted != 1 {
		t.Fatalf("delete %v %#v", err, res3)
	}
	if len(svc.List()) != 1 {
		t.Fatalf("remaining %d", len(svc.List()))
	}
	ob, _, _ = svc.Links(alpha.ID)
	if len(ob) != 1 || ob[0].ToID != "" {
		t.Fatalf("unresolved after vanish %#v", ob)
	}
}

func TestSync_SkipSymlinkOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ext := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(ext, []byte("# Secret\nleak\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "secret.md")
	if err := os.Symlink(ext, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.md"), []byte("# Ok\ninside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(store.New(), root)
	res, err := svc.Sync()
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	docs := svc.List()
	if len(docs) != 1 || docs[0].Title != "Ok" {
		t.Fatalf("docs %#v res %#v", docs, res)
	}
}

func TestPut_WritesSlugAndResolves(t *testing.T) {
	root := t.TempDir()
	svc := New(store.New(), root)
	a, err := svc.Put("Alpha", "hello [[Other]]")
	if err != nil || a.Path != "alpha.md" {
		t.Fatalf("put alpha %v %#v", err, a)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha.md")); err != nil {
		t.Fatalf("file: %v", err)
	}
	ob, _, err := svc.Links(a.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != "" {
		t.Fatalf("unresolved %v %#v", err, ob)
	}
	b, err := svc.Put("Other", "back [[Alpha]]")
	if err != nil {
		t.Fatalf("put other: %v", err)
	}
	ob, ib, err := svc.Links(a.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != b.ID || len(ib) != 1 {
		t.Fatalf("resolved %v %#v %#v", err, ob, ib)
	}
}

func TestSync_SQLiteFTSHit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.md"), []byte("# Other\n\nhello pineapple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.md"), []byte("# Note\n\nSee [[Other]] mango\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	svc := New(db, root)
	if _, err := svc.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var other, note *store.VaultDoc
	for _, d := range svc.List() {
		switch d.Title {
		case "Other":
			other = d
		case "Note":
			note = d
		}
	}
	if other == nil || note == nil {
		t.Fatalf("docs %#v", svc.List())
	}
	ob, _, err := svc.Links(note.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != other.ID {
		t.Fatalf("note outbound %v %#v", err, ob)
	}
	_, ib, err := svc.Links(other.ID)
	if err != nil || len(ib) != 1 || ib[0].FromID != note.ID {
		t.Fatalf("other inbound %v %#v", err, ib)
	}
	hits, err := svc.Search("pineapple")
	if err != nil || len(hits) == 0 {
		t.Fatalf("fts %v %#v", err, hits)
	}
}

func TestPut_WorkspaceAndDotDot(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	svc := New(store.New(), outside)
	if _, err := svc.Put("Nope", "body"); err == nil {
		t.Fatal("expected outside workspace")
	}
	inside := New(store.New(), filepath.Join(ws, "vault"))
	if _, err := inside.Put("Ok", "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := inside.absPath("../escape.md"); err == nil {
		t.Fatal("expected path escape")
	}
}

func TestUnderRootRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if underRoot(root, filepath.Join(root, "..", "nope.md")) {
		t.Fatal("expected escape reject")
	}
	if !underRoot(root, filepath.Join(root, "ok.md")) {
		t.Fatal("expected inside")
	}
}

func TestParseWikilinksUnclosed(t *testing.T) {
	if got := ParseWikilinks("see [[Nope"); len(got) != 0 {
		t.Fatalf("unclosed %#v", got)
	}
	if !strings.Contains(Slug("Foo_Bar Baz"), "foo") {
		t.Fatalf("slug %q", Slug("Foo_Bar Baz"))
	}
}
