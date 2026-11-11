// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

const (
	sourceTenant = "11111111-1111-4111-8111-111111111111"
	otherTenant  = "22222222-2222-4222-8222-222222222222"
	destTenant   = "minh-an"
	otherDest    = "tenant-b"
	tenantRoot   = "tenants/minh-an"

	bodyMarker    = "BODYMARKERminhanphapche"
	summaryMarker = "SUMMARYMARKERminhan"
	secretMarker  = "SECRETMARKERdonotimport"
	embedMarker   = "EMBEDMARKERvector"
	tsvMarker     = "TSVMARKERdonotimport"
	oldMarker     = "OLDVERSIONMARKER"
	changedMarker = "CHANGEDBYMARKER"
	agentMarker   = "AGENTIDMARKER"
	foreignMarker = "FOREIGNBODYMARKER"
	lienHeMarker  = "LIENHEBODY"
	tenantBMarker = "TENANTBONLY"
	memoryMarker  = "MEMORYMARKERuntouched"
	contextPhrase = "tham chieu mau"
)

// minhAnExport is a synthetic GoClaw-shaped dump. The mq-capital paths are
// rejection fixtures, not customer records.
const minhAnExport = `{
  "vault_documents": [
    {
      "id": "doc-a",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "agent_id": "AGENTIDMARKER",
      "scope": "personal",
      "path": "tenants/minh-an/notes/quy-trinh.md",
      "title": "Quy trinh Minh An",
      "doc_type": "note",
      "content_hash": "HASHMARKERnotstored",
      "embedding": "EMBEDMARKERvector",
      "metadata": {
        "kind": "sample",
        "api_token": "SECRETMARKERdonotimport",
        "embedding": "EMBEDMARKERvector",
        "channel": {"bot_token": "SECRETMARKERdonotimport"},
        "headers": {"Authorization": "bearer SECRETMARKERdonotimport"}
      },
      "created_at": "2026-03-01T04:05:06Z",
      "updated_at": "2026-03-01T04:05:06Z",
      "summary": "SUMMARYMARKERminhan",
      "tsv": "TSVMARKERdonotimport"
    },
    {
      "id": "doc-b",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "tenants/minh-an/notes/lien-he.md",
      "title": "Lien he mau",
      "summary": "",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-c",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "tenants/mq-capital/bien-ban.md",
      "title": "Khong duoc nhap",
      "summary": "SECRETMARKERdonotimport",
      "embedding": [0.987654321, 0.123456789],
      "tsv": "TSVMARKERdonotimport",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-d",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "tenants/minh-an/../../tenants/mq-capital/leak.md",
      "title": "Traversal",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-e",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "/etc/passwd",
      "title": "Absolute",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-f",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "tenants/minh-an/notes/alias -> tenants/mq-capital/x.md",
      "title": "Symlink like",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-g",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "tenants/minh-an/%2e%2e/tenants/mq-capital/enc.md",
      "title": "Encoded",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-h",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "tenants/minh-an/tenants/mq-capital/nested.md",
      "title": "Nested other tenant",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-i",
      "tenant_id": "22222222-2222-4222-8222-222222222222",
      "path": "tenants/minh-an/notes/foreign.md",
      "title": "Foreign tenant",
      "summary": "FOREIGNBODYMARKER",
      "updated_at": "2026-03-01T04:05:06Z"
    },
    {
      "id": "doc-j",
      "tenant_id": "11111111-1111-4111-8111-111111111111",
      "path": "notes/outside.md",
      "title": "Outside root",
      "updated_at": "2026-03-01T04:05:06Z"
    }
  ],
  "vault_versions": [
    {"id": "ver-a1", "doc_id": "doc-a", "version": 1, "content": "OLDVERSIONMARKER", "changed_by": "CHANGEDBYMARKER", "created_at": "2026-02-01T00:00:00Z"},
    {"id": "ver-a2", "doc_id": "doc-a", "version": 2, "content": "BODYMARKERminhanphapche", "changed_by": "CHANGEDBYMARKER", "created_at": "2026-03-01T04:05:06Z"},
    {"id": "ver-b1", "doc_id": "doc-b", "version": 1, "content": "LIENHEBODY", "created_at": "2026-03-01T04:05:06Z"},
    {"id": "ver-c1", "doc_id": "doc-c", "version": 1, "content": "SECRETMARKERdonotimport", "created_at": "2026-03-01T04:05:06Z"}
  ],
  "vault_links": [
    {"id": "lnk-1", "from_doc_id": "doc-a", "to_doc_id": "doc-b", "link_type": "wikilink", "context": "tham chieu mau bearer SECRETMARKERdonotimport", "metadata": {"token": "SECRETMARKERdonotimport"}},
    {"id": "lnk-2", "from_doc_id": "doc-a", "to_doc_id": "doc-c", "link_type": "wikilink", "context": "SECRETMARKERdonotimport"},
    {"id": "lnk-3", "from_doc_id": "doc-a", "to_doc_id": "doc-d", "link_type": "wikilink", "context": ""},
    {"id": "lnk-4", "from_doc_id": "doc-a", "to_doc_id": "doc-missing", "link_type": "wikilink", "context": ""},
    {"id": "lnk-5", "from_doc_id": "doc-c", "to_doc_id": "doc-a", "link_type": "wikilink", "context": ""},
    {"id": "lnk-6", "from_doc_id": "doc-i", "to_doc_id": "doc-a", "link_type": "wikilink", "context": "FOREIGNBODYMARKER"}
  ]
}`

func TestClassifyPath(t *testing.T) {
	root, err := cleanRoot(tenantRoot)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path   string
		reason string
		logic  string
	}{
		{"tenants/minh-an/notes/a.md", "", "notes/a.md"},
		{"tenants/minh-an/./notes/a.md", "", "notes/a.md"},
		{"tenants/mq-capital/bien-ban.md", reasonOtherTenant, ""},
		{"tenants/minh-an/../../tenants/mq-capital/leak.md", reasonTraversal, ""},
		{"tenants\\minh-an\\..\\..\\tenants\\mq-capital\\x.md", reasonTraversal, ""},
		{"/etc/passwd", reasonAbsolute, ""},
		{"C:/Windows/note.md", reasonAbsolute, ""},
		{"file:/tmp/note.md", reasonAbsolute, ""},
		{"tenants/minh-an/notes/alias -> tenants/mq-capital/x.md", reasonSymlink, ""},
		{"tenants/minh-an/%2e%2e/tenants/mq-capital/enc.md", reasonEncoded, ""},
		{"tenants/minh-an/tenants/mq-capital/nested.md", reasonOtherTenant, ""},
		{"notes/outside.md", reasonOutside, ""},
		{"tenants/minh-an-evil/notes/a.md", reasonOtherTenant, ""},
		{"tenants/minh-an/notes/a\ninjected.md", reasonIllegal, ""},
	}
	for _, tc := range cases {
		got := classifyPath(tc.path, root)
		if got.Reason != tc.reason || got.Logical != tc.logic {
			t.Fatalf("path %q got logical %q reason %q", tc.path, got.Logical, got.Reason)
		}
	}
}

func TestImportMinhAn(t *testing.T) {
	ex := readMinhAn(t)
	for name, open := range importBackends(t) {
		t.Run(name, func(t *testing.T) {
			raw, vaultRoot := open(t)
			seedOtherTenant(t, raw)
			cs := &countingStore{StoreIface: raw}
			logBuf := &bytes.Buffer{}
			opt := Options{
				SourceTenant: sourceTenant,
				Root:         tenantRoot,
				DestTenant:   destTenant,
				VaultRoot:    vaultRoot,
				DryRun:       true,
				Log:          bufLog{logBuf},
			}
			before := snapshot(t, raw)
			dry, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, dry, 2, 0, 1, 12)
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatalf("dry-run wrote puts=%d links=%d resolves=%d", cs.puts, cs.links, cs.resolves)
			}
			if snapshot(t, raw) != before {
				t.Fatal("dry-run changed the store")
			}
			if _, err := os.Stat(filepath.Join(vaultRoot, "t")); !os.IsNotExist(err) {
				t.Fatal("dry-run created a vault directory")
			}
			assertRejectionLogged(t, logBuf.String(), dry)

			opt.DryRun = false
			cs.reset()
			first, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, first, 2, 0, 1, 12)
			if cs.puts != 2 || cs.links != 1 || cs.resolves != 1 {
				t.Fatalf("first run puts=%d links=%d resolves=%d", cs.puts, cs.links, cs.resolves)
			}
			assertImported(t, raw, vaultRoot)
			assertNoLeak(t, raw, logBuf.String(), first)
			snap := snapshot(t, raw)
			sqlSnap := sqlSnapshot(t, raw)
			fileTime := mtime(t, filepath.Join(vaultRoot, "t", destTenant, "notes", "quy-trinh.md"))

			cs.reset()
			second, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, second, 0, 0, 3, 12)
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatalf("second run wrote puts=%d links=%d resolves=%d", cs.puts, cs.links, cs.resolves)
			}
			if snapshot(t, raw) != snap {
				t.Fatal("second run changed vault rows")
			}
			if got := sqlSnapshot(t, raw); got != sqlSnap {
				t.Fatal("second run changed sql vault rows")
			}
			if got := mtime(t, filepath.Join(vaultRoot, "t", destTenant, "notes", "quy-trinh.md")); !got.Equal(fileTime) {
				t.Fatalf("second run changed file mtime %s -> %s", fileTime, got)
			}

			cs.reset()
			opt.DryRun = true
			again, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertCounts(t, again, 0, 0, 3, 12)
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatal("dry-run after import wrote")
			}
			if snapshot(t, raw) != snap {
				t.Fatal("dry-run after import changed rows")
			}
		})
	}
}

func TestImportUpdateThenNoop(t *testing.T) {
	ex := readMinhAn(t)
	raw := store.New()
	opt := Options{SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, Log: discardLog{}}
	if _, err := Import(raw, ex, opt); err != nil {
		t.Fatal(err)
	}
	ex.Documents[0].Summary = summaryMarker + "v2"
	cs := &countingStore{StoreIface: raw}
	updated, err := Import(cs, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Updated != 1 || updated.Inserted != 0 {
		t.Fatalf("update counts %+v", updated)
	}
	if cs.puts != 1 || cs.links != 0 {
		t.Fatalf("update writes puts=%d links=%d", cs.puts, cs.links)
	}
	doc, err := raw.FindVaultDocByPath(destTenant, "notes/quy-trinh.md")
	if err != nil || !strings.Contains(doc.Body, summaryMarker+"v2") || strings.Contains(doc.Body, secretMarker) {
		t.Fatalf("updated body err=%v", err)
	}
	cs.reset()
	snap := snapshot(t, raw)
	noop, err := Import(cs, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if noop.Updated != 0 || noop.Inserted != 0 || cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
		t.Fatalf("noop %+v puts=%d links=%d resolves=%d", noop, cs.puts, cs.links, cs.resolves)
	}
	if snapshot(t, raw) != snap {
		t.Fatal("noop changed rows")
	}
}

func TestImportLeavesUnclearOwner(t *testing.T) {
	st := store.New()
	legacy, err := st.PutVaultDoc(store.VaultDoc{
		Title: "Legacy", Path: "notes/legacy.md", Body: "LEGACYMARKER", SHA256: "legacy",
		TenantID: destTenant, OwnerExplicit: false, Mtime: time.Unix(10, 0).UTC(),
	})
	if err != nil || legacy.OwnerExplicit {
		t.Fatal(err)
	}
	ex := &Export{Documents: []Document{{
		ID: "leg", TenantID: sourceTenant, Path: "tenants/minh-an/notes/legacy.md",
		Title: "Replacement", Summary: "NEWBODYMARKER", UpdatedAt: "2026-04-01T00:00:00Z",
	}}}
	rep, err := Import(st, ex, Options{SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Rejected != 1 || rep.Inserted != 0 || rep.Updated != 0 {
		t.Fatalf("counts %+v", rep)
	}
	got, err := st.FindVaultDocByPath(destTenant, "notes/legacy.md")
	if err != nil || got.Body != "LEGACYMARKER" || got.OwnerExplicit || got.ID != legacy.ID {
		t.Fatalf("legacy changed %#v %v", got, err)
	}
}

func TestImportDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("SENTINELOUTSIDE"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(root, "t", destTenant, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(notes, "linked.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	otherDir := filepath.Join(t.TempDir(), "other-tenant")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "note.md"), []byte("SENTINELOUTSIDE"), 0o644); err != nil {
		t.Fatal(err)
	}
	viaParent := filepath.Join(root, "t", destTenant)
	if err := os.Symlink(otherDir, filepath.Join(viaParent, "via")); err != nil {
		t.Fatal(err)
	}
	ex := &Export{
		Documents: []Document{
			{ID: "ok", TenantID: sourceTenant, Path: "tenants/minh-an/notes/ok.md", Title: "Ok", UpdatedAt: "2026-03-01T04:05:06Z"},
			{ID: "sy", TenantID: sourceTenant, Path: "tenants/minh-an/notes/linked.md", Title: "Linked", UpdatedAt: "2026-03-01T04:05:06Z"},
			{ID: "via", TenantID: sourceTenant, Path: "tenants/minh-an/via/note.md", Title: "Via", UpdatedAt: "2026-03-01T04:05:06Z"},
		},
		Versions: []Version{
			{DocID: "ok", Version: 1, Content: "OKBODY", CreatedAt: "2026-03-01T04:05:06Z"},
			{DocID: "sy", Version: 1, Content: "SHOULDNOTWRITE", CreatedAt: "2026-03-01T04:05:06Z"},
			{DocID: "via", Version: 1, Content: "SHOULDNOTWRITE", CreatedAt: "2026-03-01T04:05:06Z"},
		},
	}
	logBuf := &bytes.Buffer{}
	st := store.New()
	rep, err := Import(st, ex, Options{
		SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant,
		VaultRoot: root, Log: bufLog{logBuf},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inserted != 1 || rep.Rejected != 2 {
		t.Fatalf("counts %+v", rep)
	}
	if _, err := st.FindVaultDocByPath(destTenant, "notes/ok.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindVaultDocByPath(destTenant, "notes/linked.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.FindVaultDocByPath(destTenant, "via/note.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "SENTINELOUTSIDE" {
		t.Fatalf("symlink target changed %q %v", got, err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	got, err = os.ReadFile(filepath.Join(otherDir, "note.md"))
	if err != nil || string(got) != "SENTINELOUTSIDE" {
		t.Fatalf("parent symlink target changed %q %v", got, err)
	}
	if !strings.Contains(logBuf.String(), "notes/linked.md") || !strings.Contains(logBuf.String(), "via/note.md") {
		t.Fatalf("log %s", logBuf.String())
	}
	if strings.Contains(logBuf.String(), "SHOULDNOTWRITE") || strings.Contains(logBuf.String(), "SENTINELOUTSIDE") {
		t.Fatalf("log leaked file bytes %s", logBuf.String())
	}
}

func TestImportConfig(t *testing.T) {
	ex := &Export{}
	if _, err := Import(nil, ex, Options{}); err == nil {
		t.Fatal("nil store")
	}
	if _, err := Import(store.New(), ex, Options{SourceTenant: sourceTenant, DestTenant: destTenant}); err == nil {
		t.Fatal("missing root")
	}
	if _, err := Import(store.New(), ex, Options{SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: "bad id"}); err == nil {
		t.Fatal("bad dest")
	}
	if _, err := Import(store.New(), ex, Options{Root: "../tenants/minh-an", SourceTenant: sourceTenant, DestTenant: destTenant}); err == nil {
		t.Fatal("bad root")
	}
}

func readMinhAn(t *testing.T) *Export {
	t.Helper()
	ex, err := ReadExport(strings.NewReader(minhAnExport))
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func assertCounts(t *testing.T, rep *Report, inserted, updated, skipped, rejected int) {
	t.Helper()
	if rep.Inserted != inserted || rep.Updated != updated || rep.Skipped != skipped || rep.Rejected != rejected {
		t.Fatalf("counts inserted=%d updated=%d skipped=%d rejected=%d rejections=%v",
			rep.Inserted, rep.Updated, rep.Skipped, rep.Rejected, rep.Rejections)
	}
}

func assertRejectionLogged(t *testing.T, logText string, rep *Report) {
	t.Helper()
	for _, path := range []string{
		"tenants/mq-capital/bien-ban.md",
		"tenants/minh-an/../../tenants/mq-capital/leak.md",
		"tenants/minh-an/tenants/mq-capital/nested.md",
	} {
		if !strings.Contains(logText, path) {
			t.Fatalf("log missing %s\n%s", path, logText)
		}
		if !hasRejection(rep, "doc", path) {
			t.Fatalf("report missing %s", path)
		}
	}
	if !strings.Contains(logText, "../") {
		t.Fatalf("log missing traversal\n%s", logText)
	}
	if !hasRejection(rep, "link", "tenants/mq-capital/bien-ban.md") && !hasLinkTarget(rep, "tenants/mq-capital/bien-ban.md") {
		t.Fatalf("out-of-tenant link not rejected: %+v", rep.Rejections)
	}
}

func hasRejection(rep *Report, kind, path string) bool {
	for _, r := range rep.Rejections {
		if r.Kind == kind && r.Path == path {
			return true
		}
	}
	return false
}

func hasLinkTarget(rep *Report, target string) bool {
	for _, r := range rep.Rejections {
		if r.Kind == "link" && r.Target == target {
			return true
		}
	}
	return false
}

func assertImported(t *testing.T, st store.StoreIface, vaultRoot string) {
	t.Helper()
	a, err := st.FindVaultDocByPath(destTenant, "notes/quy-trinh.md")
	if err != nil {
		t.Fatal(err)
	}
	if !a.OwnerExplicit || a.TenantID != destTenant {
		t.Fatalf("owner %#v", a)
	}
	if !strings.Contains(a.Body, bodyMarker) || !strings.Contains(a.Body, summaryMarker) || !strings.Contains(a.Body, `"kind":"sample"`) {
		t.Fatal("imported body missing latest text, summary, or safe metadata")
	}
	b, err := st.FindVaultDocByPath(destTenant, "notes/lien-he.md")
	if err != nil || b.Body != lienHeMarker {
		t.Fatalf("lien he %#v %v", b, err)
	}
	for _, path := range []string{"tenants/mq-capital/bien-ban.md", "notes/foreign.md", "notes/outside.md", "via/note.md"} {
		if _, err := st.FindVaultDocByPath(destTenant, path); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("path %s visible err=%v", path, err)
		}
	}
	for _, doc := range st.ListVaultDocs() {
		if strings.Contains(doc.Path, "mq-capital") || strings.Contains(doc.Body, secretMarker) || strings.Contains(doc.Body, embedMarker) || strings.Contains(doc.Body, tsvMarker) || strings.Contains(doc.Body, oldMarker) || strings.Contains(doc.Body, changedMarker) || strings.Contains(doc.Body, agentMarker) || strings.Contains(doc.Body, foreignMarker) {
			t.Fatalf("leaked doc path=%s", doc.Path)
		}
	}
	ob, _, err := st.ListVaultDocLinks(a.ID)
	if err != nil || len(ob) != 1 || ob[0].ToID != b.ID {
		t.Fatalf("links %v %#v", err, ob)
	}
	if !strings.Contains(ob[0].Raw, "|wikilink: "+contextPhrase) || strings.Contains(ob[0].Raw, secretMarker) || strings.Contains(ob[0].Raw, "mq-capital") {
		t.Fatalf("raw %q", ob[0].Raw)
	}
	hits, err := st.SearchVault(otherDest, bodyMarker)
	if err != nil || len(hits) != 0 {
		t.Fatalf("tenant B saw Minh An %v %#v", err, hits)
	}
	hits, err = st.SearchVault(destTenant, bodyMarker)
	if err != nil || len(hits) != 1 || hits[0].Path != "notes/quy-trinh.md" {
		t.Fatalf("tenant A search %v %#v", err, hits)
	}
	hits, err = st.SearchVault(destTenant, tenantBMarker)
	if err != nil || len(hits) != 0 {
		t.Fatalf("tenant A saw tenant B %v %#v", err, hits)
	}
	other, err := st.FindVaultDocByPath(otherDest, "notes/quy-trinh.md")
	if err != nil || other.Body != tenantBMarker || other.TenantID != otherDest {
		t.Fatalf("tenant B row %#v %v", other, err)
	}
	if _, err := st.FindVaultDocByPath(destTenant, "notes/quy-trinh.md"); err != nil {
		t.Fatal(err)
	}
	svc := vault.New(st, vaultRoot)
	if _, err := svc.Get(otherDest, a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("vault get tenant B %v", err)
	}
	got, err := svc.Get(destTenant, a.ID)
	if err != nil || got.BodyStatus != "current" || !strings.Contains(got.Body, bodyMarker) {
		t.Fatalf("vault get %#v %v", got, err)
	}
	mem, err := st.GetMemory(memoryID(t, st))
	if err != nil || mem.Body != memoryMarker {
		t.Fatalf("memory changed %#v %v", mem, err)
	}
	onDisk, err := os.ReadFile(filepath.Join(vaultRoot, "t", destTenant, "notes", "quy-trinh.md"))
	if err != nil || string(onDisk) != a.Body {
		t.Fatalf("file mismatch %v", err)
	}
}

func assertNoLeak(t *testing.T, st store.StoreIface, logText string, rep *Report) {
	t.Helper()
	stored := storedText(t, st)
	report := mustJSON(rep)
	for _, marker := range []string{secretMarker, embedMarker, tsvMarker, oldMarker, changedMarker, agentMarker, foreignMarker, "0.987654321"} {
		if strings.Contains(stored, marker) {
			t.Fatalf("store contains %s", marker)
		}
		if strings.Contains(logText, marker) || strings.Contains(report, marker) {
			t.Fatalf("report or log contains %s", marker)
		}
	}
	for _, marker := range []string{bodyMarker, summaryMarker, contextPhrase, lienHeMarker, foreignMarker} {
		if strings.Contains(logText, marker) {
			t.Fatalf("log contains %s", marker)
		}
	}
}

func storedText(t *testing.T, st store.StoreIface) string {
	t.Helper()
	var b strings.Builder
	for _, doc := range st.ListVaultDocs() {
		b.WriteString(doc.Body)
		b.WriteString(doc.Title)
		b.WriteString(doc.Path)
		ob, ib, err := st.ListVaultDocLinks(doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range ob {
			b.WriteString(l.Raw)
		}
		for _, l := range ib {
			b.WriteString(l.Raw)
		}
	}
	if db, ok := sqlDB(st); ok {
		b.WriteString(sqlText(t, db))
	}
	return b.String()
}

func seedOtherTenant(t *testing.T, st store.StoreIface) {
	t.Helper()
	if _, err := st.PutVaultDoc(store.VaultDoc{
		Title: "Tenant B", Path: "notes/quy-trinh.md", Body: tenantBMarker, SHA256: "b",
		TenantID: otherDest, OwnerExplicit: true, Mtime: time.Unix(20, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(store.Agent{AgentKey: "tenant-b-agent", DisplayName: "Tenant B", TenantID: otherDest})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: otherDest, Label: "seed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutMemory(store.Memory{SessionID: sess.ID, TenantID: otherDest, Body: memoryMarker}); err != nil {
		t.Fatal(err)
	}
}

func memoryID(t *testing.T, st store.StoreIface) string {
	t.Helper()
	// List sessions then memories. The seed creates one session.
	sessions := st.ListSessions()
	for _, sess := range sessions {
		mems, err := st.ListMemories(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range mems {
			if m.Body == memoryMarker {
				return m.ID
			}
		}
	}
	t.Fatal("seed memory missing")
	return ""
}

func snapshot(t *testing.T, st store.StoreIface) string {
	t.Helper()
	docs := st.ListVaultDocs()
	var b strings.Builder
	for _, d := range docs {
		b.WriteString(d.ID)
		b.WriteByte('|')
		b.WriteString(d.TenantID)
		b.WriteByte('|')
		b.WriteString(d.Path)
		b.WriteByte('|')
		b.WriteString(d.Title)
		b.WriteByte('|')
		b.WriteString(d.SHA256)
		b.WriteByte('|')
		b.WriteString(d.Mtime.UTC().Format(time.RFC3339Nano))
		b.WriteByte('|')
		if d.OwnerExplicit {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
		b.WriteByte('|')
		b.WriteString(d.Body)
		b.WriteByte('\n')
		ob, ib, err := st.ListVaultDocLinks(d.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range ob {
			b.WriteString("O ")
			b.WriteString(l.FromID)
			b.WriteByte(' ')
			b.WriteString(l.ToID)
			b.WriteByte(' ')
			b.WriteString(l.Raw)
			b.WriteByte('\n')
		}
		for _, l := range ib {
			b.WriteString("I ")
			b.WriteString(l.FromID)
			b.WriteByte(' ')
			b.WriteString(l.ToID)
			b.WriteByte(' ')
			b.WriteString(l.Raw)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sqlSnapshot(t *testing.T, st store.StoreIface) string {
	t.Helper()
	db, ok := sqlDB(st)
	if !ok {
		return ""
	}
	return sqlText(t, db)
}

func sqlDB(st store.StoreIface) (*sql.DB, bool) {
	q, ok := st.(interface{ SQL() (*sql.DB, bool) })
	if !ok {
		return nil, false
	}
	db, _ := q.SQL()
	if db == nil {
		return nil, false
	}
	return db, true
}

func sqlText(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	rows, err := db.Query(`SELECT id, title, path, sha256, mtime, body, tenant_id, owner_explicit FROM vault_docs ORDER BY tenant_id, path, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, title, path, sum, mtime, body, tenant string
		var explicit int
		if err := rows.Scan(&id, &title, &path, &sum, &mtime, &body, &tenant, &explicit); err != nil {
			t.Fatal(err)
		}
		b.WriteString(strings.Join([]string{id, title, path, sum, mtime, body, tenant, strconv.Itoa(explicit)}, "|"))
		b.WriteByte('\n')
	}
	links, err := db.Query(`SELECT from_id, to_id, raw FROM vault_links ORDER BY from_id, raw`)
	if err != nil {
		t.Fatal(err)
	}
	defer links.Close()
	for links.Next() {
		var from, to, raw string
		if err := links.Scan(&from, &to, &raw); err != nil {
			t.Fatal(err)
		}
		b.WriteString(strings.Join([]string{from, to, raw}, "|"))
		b.WriteByte('\n')
	}
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('vault_versions', 'vault_documents')`).Scan(&versions); err != nil {
		// Postgres has no sqlite_master. Limit the lookup to the importer schema.
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name IN ('vault_versions', 'vault_documents')`).Scan(&versions); err != nil {
			t.Fatal(err)
		}
	}
	if versions != 0 {
		t.Fatalf("parallel vault tables present: %d", versions)
	}
	return b.String()
}

func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

func mustJSON(rep *Report) string {
	var b strings.Builder
	for _, r := range rep.Rejections {
		b.WriteString(r.Kind)
		b.WriteByte(' ')
		b.WriteString(r.Path)
		b.WriteByte(' ')
		b.WriteString(r.Target)
		b.WriteByte(' ')
		b.WriteString(r.Reason)
		b.WriteByte('\n')
	}
	return b.String()
}

type bufLog struct{ b *bytes.Buffer }

func (l bufLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.b, format+"\n", args...)
}

type countingStore struct {
	store.StoreIface
	puts, links, resolves int
}

func (c *countingStore) reset() { c.puts, c.links, c.resolves = 0, 0, 0 }

func (c *countingStore) PutVaultDoc(d store.VaultDoc) (*store.VaultDoc, error) {
	c.puts++
	return c.StoreIface.PutVaultDoc(d)
}

func (c *countingStore) ReplaceVaultLinks(id string, raws []string) error {
	c.links++
	return c.StoreIface.ReplaceVaultLinks(id, raws)
}

func (c *countingStore) ReResolveVaultLinks() error {
	c.resolves++
	return c.StoreIface.ReResolveVaultLinks()
}

func importBackends(t *testing.T) map[string]func(t *testing.T) (store.StoreIface, string) {
	t.Helper()
	out := map[string]func(t *testing.T) (store.StoreIface, string){
		"memory": func(t *testing.T) (store.StoreIface, string) {
			return store.New(), t.TempDir()
		},
		"sqlite": func(t *testing.T) (store.StoreIface, string) {
			s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "vault.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s, t.TempDir()
		},
	}
	if pg := openPostgres(t); pg != nil {
		out["postgres"] = func(t *testing.T) (store.StoreIface, string) {
			return pg, t.TempDir()
		}
	}
	return out
}

func openPostgres(t *testing.T) store.StoreIface {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		return nil
	}
	schema := "vaultimport31"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	pg, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	return pg
}
