// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const sharedAt = "2026-03-01T04:05:06Z"

// localBackends is memory and sqlite: each test gets an empty store.
func localBackends() map[string]func(t *testing.T) store.StoreIface {
	return map[string]func(t *testing.T) store.StoreIface{
		"memory": func(t *testing.T) store.StoreIface { return store.New() },
		"sqlite": func(t *testing.T) store.StoreIface {
			s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "vault.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
	}
}

func versioned(docs ...Document) *Export {
	ex := &Export{Documents: docs}
	for _, d := range docs {
		ex.Versions = append(ex.Versions, Version{DocID: d.ID, Version: 1, Content: "NOIDUNG-" + d.ID, CreatedAt: sharedAt})
	}
	return ex
}

func gdoc(id, scope, agent, team, path, title string) Document {
	return Document{ID: id, TenantID: sourceTenant, Scope: scope, AgentID: agent, TeamID: team, Path: path, Title: title, UpdatedAt: sharedAt}
}

func noDoc(t *testing.T, st store.StoreIface, path string) {
	t.Helper()
	if _, err := st.FindVaultDocByPath(destTenant, path); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("%s should not be stored (%v)", path, err)
	}
}

func assertAccess(t *testing.T, d *store.VaultDoc, scope, agent, team string) {
	t.Helper()
	got := d.Scope
	if got == "" {
		got = store.VaultScopeTenant
	}
	if got != scope || d.AgentID != agent || d.TeamID != team {
		t.Fatalf("%s access %q/%q/%q, want %q/%q/%q", d.Path, got, d.AgentID, d.TeamID, scope, agent, team)
	}
}

// GoClaw keeps a shared row, a team row and personal rows of two agents at
// one path. Every row is imported with its own audience: the shared row
// keeps the path, the others move to qualified siblings.
func TestSamePathKeepsEveryScope(t *testing.T) {
	for name, open := range localBackends() {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			vaultRoot := t.TempDir()
			an := seedAgent(t, st, "minh-an")
			binh := seedAgent(t, st, "tro-ly-binh")
			team, err := st.CreateTeam(store.Team{Name: "Ban hang", LeadAgentID: an, TenantID: destTenant})
			if err != nil {
				t.Fatal(err)
			}
			path := "tenants/minh-an/notes/quy-trinh.md"
			ex := versioned(
				gdoc("doc-shared", "shared", "", "", path, "Quy trinh chung"),
				gdoc("doc-team", "team", "", "gc-team", path, "Quy trinh team"),
				gdoc("doc-an", "personal", "gc-an", "", path, "Quy trinh Minh An"),
				gdoc("doc-binh", "personal", "gc-binh", "", path, "Quy trinh Binh"),
			)
			opt := Options{
				SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, VaultRoot: vaultRoot,
				AgentMap: map[string]string{"gc-an": an, "gc-binh": binh},
				TeamMap:  map[string]string{"gc-team": team.ID},
				Log:      discardLog{},
			}
			cs := &countingStore{StoreIface: st}
			opt.DryRun = true
			dry, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, dry, 4, 0, 0, 0)
			if cs.puts != 0 {
				t.Fatal("dry-run wrote")
			}
			opt.DryRun = false
			rep, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, rep, 4, 0, 0, 0)

			shared := mustDoc(t, st, "notes/quy-trinh.md")
			assertAccess(t, shared, store.VaultScopeTenant, "", "")
			teamDoc := mustDoc(t, st, "notes/quy-trinh~team-"+pathToken(team.ID)+".md")
			assertAccess(t, teamDoc, store.VaultScopeTeam, "", team.ID)
			anDoc := mustDoc(t, st, "notes/quy-trinh~personal-minh-an.md")
			assertAccess(t, anDoc, store.VaultScopeAgent, an, "")
			binhDoc := mustDoc(t, st, "notes/quy-trinh~personal-tro-ly-binh.md")
			assertAccess(t, binhDoc, store.VaultScopeAgent, binh, "")
			for doc, marker := range map[*store.VaultDoc]string{
				shared: "NOIDUNG-doc-shared", teamDoc: "NOIDUNG-doc-team", anDoc: "NOIDUNG-doc-an", binhDoc: "NOIDUNG-doc-binh",
			} {
				if !strings.Contains(doc.Body, marker) {
					t.Fatalf("%s lost %s", doc.Path, marker)
				}
				onDisk, err := os.ReadFile(filepath.Join(vaultRoot, "t", destTenant, filepath.FromSlash(doc.Path)))
				if err != nil || string(onDisk) != doc.Body {
					t.Fatalf("%s file %v", doc.Path, err)
				}
			}
			for _, other := range []*store.VaultDoc{shared, teamDoc, binhDoc} {
				if strings.Contains(other.Body, "NOIDUNG-doc-an") {
					t.Fatalf("%s holds Minh An's personal text", other.Path)
				}
			}

			cs.reset()
			again, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, again, 0, 0, 4, 0)
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatalf("re-run wrote puts=%d", cs.puts)
			}
		})
	}
}

func TestPersonalScopeOptions(t *testing.T) {
	path := "tenants/minh-an/notes/rieng.md"
	ex := func() *Export {
		return versioned(gdoc("doc-personal", "personal", "gc-an", "", path, "Ghi chu rieng Minh An"))
	}
	st := store.New()
	an := seedAgent(t, st, "minh-an")
	logBuf := &bytes.Buffer{}
	opt := Options{
		SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant,
		AgentMap: map[string]string{"gc-an": an}, HoldPersonal: true, Log: bufLog{logBuf},
	}
	cs := &countingStore{StoreIface: st}
	held, err := Import(cs, ex(), opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, held, 0, 0, 0, 1)
	if !hasReason(held, "doc", path, "scope-personal-held") || cs.puts != 0 {
		t.Fatalf("hold %+v puts=%d", held.Rejections, cs.puts)
	}
	if strings.Contains(logBuf.String(), "NOIDUNG") {
		t.Fatal("log leaked a body")
	}
	noDoc(t, st, "notes/rieng.md")

	// Default: agent scope, the same audience GoClaw gives a personal row.
	opt.HoldPersonal = false
	rep, err := Import(st, ex(), opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 1, 0, 0, 0)
	first := mustDoc(t, st, "notes/rieng.md")
	assertAccess(t, first, store.VaultScopeAgent, an, "")

	// Strict: goso personal scope. The audience narrows, so the row is
	// written again rather than edited in place.
	opt.PersonalScope = PersonalScopeStrict
	rep, err = Import(st, ex(), opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 0, 1, 0, 0)
	strict := mustDoc(t, st, "notes/rieng.md")
	assertAccess(t, strict, store.VaultScopePersonal, an, "")
	if strict.ID == first.ID {
		t.Fatal("narrowed row kept its id")
	}

	// Back to agent scope: wider for the same agent, updated in place.
	opt.PersonalScope = PersonalScopeAgent
	rep, err = Import(st, ex(), opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 0, 1, 0, 0)
	wide := mustDoc(t, st, "notes/rieng.md")
	assertAccess(t, wide, store.VaultScopeAgent, an, "")
	if wide.ID != strict.ID {
		t.Fatal("widened row changed id")
	}

	opt.PersonalScope = "everyone"
	var cfg *ConfigError
	if _, err := Import(st, ex(), opt); !errors.As(err, &cfg) {
		t.Fatalf("bad personal scope: %v", err)
	}
}

// A GoClaw scope change on a re-run moves the row to the new audience. A
// narrower audience removes the old file and row first.
func TestRescopeOnRerun(t *testing.T) {
	for name, open := range localBackends() {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			vaultRoot := t.TempDir()
			an := seedAgent(t, st, "minh-an")
			path := "tenants/minh-an/notes/bang-gia.md"
			opt := Options{
				SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, VaultRoot: vaultRoot,
				AgentMap: map[string]string{"gc-an": an}, Log: discardLog{},
			}
			if _, err := Import(st, versioned(gdoc("doc-gia", "shared", "", "", path, "Bang gia")), opt); err != nil {
				t.Fatal(err)
			}
			wide := mustDoc(t, st, "notes/bang-gia.md")
			assertAccess(t, wide, store.VaultScopeTenant, "", "")

			narrow := versioned(gdoc("doc-gia", "personal", "gc-an", "", path, "Bang gia"))
			cs := &countingStore{StoreIface: st}
			opt.DryRun = true
			dry, err := Import(cs, narrow, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, dry, 0, 1, 0, 0)
			if cs.puts != 0 {
				t.Fatal("dry-run wrote")
			}
			assertAccess(t, mustDoc(t, st, "notes/bang-gia.md"), store.VaultScopeTenant, "", "")

			opt.DryRun = false
			rep, err := Import(st, narrow, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, rep, 0, 1, 0, 0)
			got := mustDoc(t, st, "notes/bang-gia.md")
			assertAccess(t, got, store.VaultScopeAgent, an, "")
			if got.ID == wide.ID {
				t.Fatal("narrowed row kept the tenant-wide id")
			}
			if _, err := st.GetVaultDoc(wide.ID); err == nil {
				t.Fatal("tenant-wide row still exists")
			}
			onDisk, err := os.ReadFile(filepath.Join(vaultRoot, "t", destTenant, "notes", "bang-gia.md"))
			if err != nil || string(onDisk) != got.Body {
				t.Fatalf("file after narrow %v", err)
			}

			again, err := Import(st, narrow, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, again, 0, 0, 1, 0)

			// Widening back is an in-place update.
			rep, err = Import(st, versioned(gdoc("doc-gia", "shared", "", "", path, "Bang gia")), opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, rep, 0, 1, 0, 0)
			back := mustDoc(t, st, "notes/bang-gia.md")
			assertAccess(t, back, store.VaultScopeTenant, "", "")
			if back.ID != got.ID {
				t.Fatal("widened row changed id")
			}
		})
	}
}

// A path that held one GoClaw row gets a wider sibling later. The wider row
// takes the plain path; the earlier row moves to its qualified path.
func TestWiderSiblingTakesPlainPath(t *testing.T) {
	st := store.New()
	vaultRoot := t.TempDir()
	an := seedAgent(t, st, "minh-an")
	path := "tenants/minh-an/notes/ke-hoach.md"
	personal := gdoc("doc-an", "personal", "gc-an", "", path, "Ke hoach Minh An")
	shared := gdoc("doc-chung", "shared", "", "", path, "Ke hoach chung")
	opt := Options{
		SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, VaultRoot: vaultRoot,
		AgentMap: map[string]string{"gc-an": an}, Log: discardLog{},
	}
	if _, err := Import(st, versioned(personal), opt); err != nil {
		t.Fatal(err)
	}
	assertAccess(t, mustDoc(t, st, "notes/ke-hoach.md"), store.VaultScopeAgent, an, "")

	rep, err := Import(st, versioned(personal, shared), opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 1, 1, 0, 0)
	plain := mustDoc(t, st, "notes/ke-hoach.md")
	assertAccess(t, plain, store.VaultScopeTenant, "", "")
	if !strings.Contains(plain.Body, "NOIDUNG-doc-chung") || strings.Contains(plain.Body, "NOIDUNG-doc-an") {
		t.Fatal("plain path body")
	}
	moved := mustDoc(t, st, "notes/ke-hoach~personal-minh-an.md")
	assertAccess(t, moved, store.VaultScopeAgent, an, "")
	if !strings.Contains(moved.Body, "NOIDUNG-doc-an") {
		t.Fatal("personal body lost")
	}
	again, err := Import(st, versioned(personal, shared), opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, again, 0, 0, 2, 0)

	// The shared doc is deleted in GoClaw. Without AdoptOrphans its row is
	// not reused: the export may be partial.
	alone := versioned(personal)
	rep, err = Import(st, alone, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(rep, "doc", path, reasonPathTaken) {
		t.Fatalf("orphan without adopt %+v", rep.Rejections)
	}
	// With AdoptOrphans the personal doc takes the plain path back. The
	// tenant-wide row is replaced, and its own qualified copy is removed.
	opt.AdoptOrphans = true
	rep, err = Import(st, alone, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 0, 1, 0, 0)
	back := mustDoc(t, st, "notes/ke-hoach.md")
	assertAccess(t, back, store.VaultScopeAgent, an, "")
	if !strings.Contains(back.Body, "NOIDUNG-doc-an") || strings.Contains(back.Body, "NOIDUNG-doc-chung") {
		t.Fatal("adopted body")
	}
	noDoc(t, st, "notes/ke-hoach~personal-minh-an.md")
	if _, err := os.Stat(filepath.Join(vaultRoot, "t", destTenant, "notes", "ke-hoach~personal-minh-an.md")); !os.IsNotExist(err) {
		t.Fatal("stale qualified file left behind")
	}
	again, err = Import(st, alone, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, again, 0, 0, 1, 0)
}

// A file-sourced row stays the importer's after its GoClaw file changes, so
// the new text is imported instead of path-taken.
func TestChangedSourceFileUpdatesRow(t *testing.T) {
	st := store.New()
	src := t.TempDir()
	rel := "tenants/minh-an/notes/huong-dan.md"
	writeSrc(t, src, rel, []byte("Huong dan ban dau\r\nDong 2\r\n"))
	ex := &Export{Documents: []Document{{
		ID: "doc-hd", TenantID: sourceTenant, Scope: "shared", Path: rel, Title: "Huong dan",
		Summary: "tom tat", UpdatedAt: sharedAt,
	}}}
	opt := Options{SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, SrcDir: src, Log: discardLog{}}
	if _, err := Import(st, ex, opt); err != nil {
		t.Fatal(err)
	}
	writeSrc(t, src, rel, []byte("Huong dan moi\n"))
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 0, 1, 0, 0)
	got := mustDoc(t, st, "notes/huong-dan.md")
	if !strings.Contains(got.Body, "Huong dan moi") || strings.Contains(got.Body, "ban dau") {
		t.Fatal("changed file not imported")
	}
	rep, err = Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rep, 0, 0, 1, 0)

	// A hand edit in goso is still path-taken.
	edited := strings.Replace(got.Body, "Huong dan moi", "Sua tay", 1)
	if _, err := st.PutVaultDoc(store.VaultDoc{
		ID: got.ID, Title: got.Title, Path: got.Path, Body: edited, SHA256: hashBody(edited),
		Mtime: got.Mtime, TenantID: destTenant, OwnerExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}
	writeSrc(t, src, rel, []byte("Huong dan lan ba\n"))
	rep, err = Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(rep, "doc", rel, reasonPathTaken) {
		t.Fatalf("hand edit %+v", rep.Rejections)
	}
	if !strings.Contains(mustDoc(t, st, "notes/huong-dan.md").Body, "Sua tay") {
		t.Fatal("hand edit overwritten")
	}
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{0xff, 0x00, 0x7f}, 64)...)

func TestBinaryAttachment(t *testing.T) {
	for name, open := range localBackends() {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			vaultRoot := t.TempDir()
			src := t.TempDir()
			an := seedAgent(t, st, "minh-an")
			writeSrc(t, src, "tenants/minh-an/media/logo.png", pngBytes)
			writeSrc(t, src, "tenants/minh-an/docs/hop-dong.pdf", append([]byte("%PDF-1.7\n\x00"), bytes.Repeat([]byte{0xe2, 0x00}, 32)...))
			writeSrc(t, src, "tenants/minh-an/docs/to-lon.pdf", append([]byte("%PDF-1.7\n\x00"), bytes.Repeat([]byte{0x01, 0x00}, 600)...))
			writeSrc(t, src, "tenants/minh-an/docs/ghi-chu.csv", []byte("ten,gia\nMinh An,1\n"))
			ex := &Export{Documents: []Document{
				{ID: "doc-logo", TenantID: sourceTenant, Scope: "shared", Path: "tenants/minh-an/media/logo.png", Title: "logo.png", DocType: "media", Summary: "Logo Minh An", UpdatedAt: sharedAt},
				{ID: "doc-logo-an", TenantID: sourceTenant, Scope: "personal", AgentID: "gc-an", Path: "tenants/minh-an/media/logo.png", Title: "logo.png", DocType: "media", UpdatedAt: sharedAt},
				{ID: "doc-hd", TenantID: sourceTenant, Scope: "shared", Path: "tenants/minh-an/docs/hop-dong.pdf", Title: "hop-dong.pdf", DocType: "document", UpdatedAt: sharedAt},
				{ID: "doc-lon", TenantID: sourceTenant, Scope: "shared", Path: "tenants/minh-an/docs/to-lon.pdf", Title: "to-lon.pdf", DocType: "document", UpdatedAt: sharedAt},
				{ID: "doc-csv", TenantID: sourceTenant, Scope: "shared", Path: "tenants/minh-an/docs/ghi-chu.csv", Title: "ghi-chu.csv", UpdatedAt: sharedAt},
			}}
			opt := Options{
				SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, VaultRoot: vaultRoot, SrcDir: src,
				AgentMap: map[string]string{"gc-an": an}, MaxBinaryBytes: 1024, Log: discardLog{},
			}
			cs := &countingStore{StoreIface: st}
			opt.DryRun = true
			dry, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, dry, 4, 0, 0, 1)
			if !hasReason(dry, "doc", "tenants/minh-an/docs/to-lon.pdf", reasonTooLarge) {
				t.Fatalf("over cap %+v", dry.Rejections)
			}
			if cs.puts != 0 {
				t.Fatal("dry-run wrote")
			}
			if _, err := os.Stat(filepath.Join(vaultRoot, "t")); !os.IsNotExist(err) {
				t.Fatal("dry-run wrote files")
			}

			opt.DryRun = false
			rep, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, rep, 4, 0, 0, 1)
			note := mustDoc(t, st, "media/logo.png.md")
			assertAccess(t, note, store.VaultScopeTenant, "", "")
			for _, want := range []string{"Attachment: logo.png", "File: media/logo.png", "Type: media (png)", "Size: 208 bytes", "Logo Minh An"} {
				if !strings.Contains(note.Body, want) {
					t.Fatalf("note lacks %q:\n%s", want, note.Body)
				}
			}
			if strings.Contains(note.Body, "PNG") || strings.ContainsRune(note.Body, 0) {
				t.Fatal("note holds file bytes")
			}
			stored, err := os.ReadFile(filepath.Join(vaultRoot, "t", destTenant, "media", "logo.png"))
			if err != nil || !bytes.Equal(stored, pngBytes) {
				t.Fatalf("stored bytes %v", err)
			}
			mine := mustDoc(t, st, "media/logo~personal-minh-an.png.md")
			assertAccess(t, mine, store.VaultScopeAgent, an, "")
			if _, err := os.Stat(filepath.Join(vaultRoot, "t", destTenant, "media", "logo~personal-minh-an.png")); err != nil {
				t.Fatal("personal attachment bytes missing")
			}
			if pdf := mustDoc(t, st, "docs/hop-dong.pdf.md"); !strings.Contains(pdf.Body, "Type: document (pdf)") {
				t.Fatal("pdf note")
			}
			if csv := mustDoc(t, st, "docs/ghi-chu.csv.md"); !strings.Contains(csv.Body, "Minh An,1") {
				t.Fatal("text file under another extension is still a text body")
			}
			noDoc(t, st, "docs/to-lon.pdf.md")
			if _, err := os.Stat(filepath.Join(vaultRoot, "t", destTenant, "docs", "to-lon.pdf")); !os.IsNotExist(err) {
				t.Fatal("over-cap bytes stored")
			}

			cs.reset()
			again, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, again, 0, 0, 4, 1)
			if cs.puts != 0 {
				t.Fatal("re-run wrote")
			}

			// Changed bytes refresh the note and the stored file.
			changed := append(append([]byte(nil), pngBytes...), 0x00, 0x01)
			writeSrc(t, src, "tenants/minh-an/media/logo.png", changed)
			rep, err = Import(st, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, rep, 0, 2, 2, 1) // both rows of logo.png share the source file
			stored, err = os.ReadFile(filepath.Join(vaultRoot, "t", destTenant, "media", "logo.png"))
			if err != nil || !bytes.Equal(stored, changed) {
				t.Fatal("changed bytes not stored")
			}
			if !strings.Contains(mustDoc(t, st, "media/logo.png.md").Body, "Size: 210 bytes") {
				t.Fatal("note not refreshed")
			}
		})
	}
}

func TestLinkReasons(t *testing.T) {
	st := store.New()
	src := t.TempDir()
	writeSrc(t, src, "tenants/minh-an/notes/a.md", []byte("A"))
	writeSrc(t, src, "tenants/minh-an/notes/b.md", []byte("B"))
	ex := &Export{
		Documents: []Document{
			gdoc("doc-a", "shared", "", "", "tenants/minh-an/notes/a.md", "A"),
			gdoc("doc-b", "shared", "", "", "tenants/minh-an/notes/b.md", "B"),
			gdoc("doc-miss", "shared", "", "", "tenants/minh-an/notes/thieu.md", "Thieu"),
			gdoc("doc-foreign", "shared", "", "", "tenants/mq-demo/x.md", "Foreign"),
			{ID: "doc-other", TenantID: otherTenant, Scope: "shared", Path: "tenants/minh-an/notes/o.md", Title: "O", UpdatedAt: sharedAt},
		},
		Links: []Link{
			{ID: "l-ok", FromDoc: "doc-a", ToDoc: "doc-b", LinkType: "wikilink"},
			{ID: "l-from-foreign", FromDoc: "doc-foreign", ToDoc: "doc-a", LinkType: "wikilink"},
			{ID: "l-from-other", FromDoc: "doc-other", ToDoc: "doc-a", LinkType: "wikilink"},
			{ID: "l-from-missing", FromDoc: "doc-miss", ToDoc: "doc-a", LinkType: "wikilink"},
			{ID: "l-to-missing", FromDoc: "doc-a", ToDoc: "doc-miss", LinkType: "wikilink"},
			{ID: "l-to-foreign", FromDoc: "doc-a", ToDoc: "doc-foreign", LinkType: "wikilink"},
			{ID: "l-to-unknown", FromDoc: "doc-a", ToDoc: "doc-nowhere", LinkType: "wikilink"},
			{ID: "l-from-unknown", FromDoc: "doc-nowhere", ToDoc: "doc-a", LinkType: "wikilink"},
		},
	}
	opt := Options{SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, SrcDir: src, Log: discardLog{}}
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]int{}
	actions := map[string]int{}
	for _, s := range rep.Links {
		for r, n := range s.Reasons {
			reasons[r] += n
		}
		actions["inserted"] += s.Inserted
		actions["skipped"] += s.Skipped
		actions["rejected"] += s.Rejected
	}
	want := map[string]int{
		"foreign-tenant": 1, reasonTenantMismatch: 1, "source-rejected": 1,
		"target-rejected": 1, "outside-tenant": 1, "target-missing": 1, "source-missing": 1,
	}
	for r, n := range want {
		if reasons[r] != n {
			t.Fatalf("link reasons %v, want %v", reasons, want)
		}
	}
	if actions["inserted"] != 1 || actions["skipped"] != 2 || actions["rejected"] != 5 {
		t.Fatalf("link actions %v", actions)
	}
}
