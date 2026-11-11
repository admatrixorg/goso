// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// fileBodyRaw is the on-disk bytes for a document that has no version row.
// The provenance sum is sha256 of these bytes, including the CR.
var fileBodyRaw = []byte("Dong 1 Minh An\r\nDong 2\r\n")

func TestFileBodyMinhAn(t *testing.T) {
	for name, open := range importBackends(t) {
		t.Run(name, func(t *testing.T) {
			raw, vaultRoot := open(t)
			memID := seedMinhAnMemory(t, raw)
			cs := &countingStore{StoreIface: raw}
			logBuf := &bytes.Buffer{}
			opt := Options{
				SourceTenant: sourceTenant,
				Root:         tenantRoot,
				DestTenant:   destTenant,
				VaultRoot:    vaultRoot,
				Log:          bufLog{logBuf},
			}

			src := t.TempDir()
			writeSrc(t, src, "tenants/minh-an/notes/co-ban.md", []byte("FILEKHACMARKER"))
			writeSrc(t, src, "tenants/minh-an/notes/trung.md", []byte("TEAMFILEMARKER"))
			writeSrc(t, src, "tenants/minh-an/notes/tu-file.md", fileBodyRaw)
			writeSrc(t, src, "tenants/minh-an/notes/lon.md", bytes.Repeat([]byte("LARGEMARKER"), (maxFileBodyBytes/11)+2))
			writeSrc(t, src, "tenants/minh-an/notes/nhi.md", []byte("BINMARKER\xff"))
			writeSrc(t, src, "tenants/minh-an/notes/nul.md", []byte("NULMARKER\x00"))
			writeSrc(t, src, "tenants/minh-an/notes/rong.md", nil)
			if info, err := os.Stat(filepath.Join(src, "tenants/minh-an/notes/lon.md")); err != nil || info.Size() <= maxFileBodyBytes {
				t.Fatalf("large file size %v %v", info, err)
			}

			// A kept document with no version fails the run before any write,
			// including a sibling that does have a version.
			need := &Export{Documents: []Document{
				{ID: "doc-ver", TenantID: sourceTenant, Scope: "team", Path: "tenants/minh-an/notes/moi-ver.md", Title: "Co ban Minh An", UpdatedAt: "2026-03-01T04:05:06Z"},
				{ID: "doc-need", TenantID: sourceTenant, Scope: "team", Path: "tenants/minh-an/notes/moi-file.md", Title: "Can file Minh An", UpdatedAt: "2026-03-01T04:05:06Z"},
			}, Versions: []Version{
				{DocID: "doc-ver", Version: 1, Content: "CANFILEMARKER", CreatedAt: "2026-03-01T04:05:06Z"},
			}}
			_, err := Import(cs, need, opt)
			var cfg *ConfigError
			if !errors.As(err, &cfg) || !strings.Contains(err.Error(), "source dir") {
				t.Fatalf("missing src-dir: %v", err)
			}
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatal("missing src-dir wrote")
			}
			if _, err := raw.FindVaultDocByPath(destTenant, "notes/moi-ver.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("version sibling was written")
			}
			if _, err := raw.FindVaultDocByPath(destTenant, "notes/moi-file.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("file doc was written without src-dir")
			}
			if strings.Contains(err.Error(), "CANFILEMARKER") || strings.Contains(logBuf.String(), "CANFILEMARKER") {
				t.Fatal("missing src-dir leaked a body")
			}

			opt.SrcDir = src
			opt.DryRun = true
			cs.reset()
			logBuf.Reset()
			ex := fileBodyExport()
			dry, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertFileBodyReport(t, dry, 3, 0, 1, 9)
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatal("dry-run wrote")
			}

			opt.DryRun = false
			cs.reset()
			first, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertFileBodyReport(t, first, 3, 0, 1, 9)
			if cs.puts != 3 || cs.links != 1 || cs.resolves != 1 {
				t.Fatalf("writes puts=%d links=%d resolves=%d", cs.puts, cs.links, cs.resolves)
			}
			ver := mustDoc(t, raw, "notes/co-ban.md")
			fileDoc := mustDoc(t, raw, "notes/tu-file.md")
			team := mustDoc(t, raw, "notes/trung.md")
			assertVersionBody(t, ver)
			assertFileBody(t, fileDoc)
			if !strings.Contains(team.Body, "TEAMBODYMARKER") || strings.Contains(team.Body, "PERSONALBODYMARKER") || strings.Contains(team.Body, "TEAMFILEMARKER") {
				t.Fatal("team collision lost")
			}
			for _, path := range []string{
				"notes/thieu.md", "notes/lon.md", "notes/nhi.md", "notes/nul.md", "notes/rong.md", "notes/rieng.md", "notes/khac.md",
			} {
				if _, err := raw.FindVaultDocByPath(destTenant, path); !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("rejected path stored %s", path)
				}
			}
			onDisk, err := os.ReadFile(filepath.Join(vaultRoot, "t", destTenant, "notes", "tu-file.md"))
			if err != nil || string(onDisk) != fileDoc.Body {
				t.Fatal("published file mismatch")
			}
			assertNoBodyLeak(t, logBuf.String(), first)
			snap := snapshot(t, raw)
			_ = sqlSnapshot(t, raw)

			cs.reset()
			second, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertFileBodyReport(t, second, 0, 0, 4, 9)
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatal("second run wrote")
			}
			if snapshot(t, raw) != snap {
				t.Fatal("second run changed rows")
			}
			fileAgain := mustDoc(t, raw, "notes/tu-file.md")
			if _, _, sum, ok := splitImportMarker(fileAgain.Body); !ok || sum != hashBody(string(fileBodyRaw)) {
				t.Fatal("second run changed the file sum")
			}

			edited := strings.Replace(fileAgain.Body, "Dong 1", "Dong 9", 1)
			if _, err := raw.PutVaultDoc(store.VaultDoc{
				ID: fileAgain.ID, Title: fileAgain.Title, Path: fileAgain.Path, Body: edited,
				SHA256: hashBody(edited), Mtime: fileAgain.Mtime, TenantID: destTenant, OwnerExplicit: true,
			}); err != nil {
				t.Fatal(err)
			}
			cs.reset()
			held, err := Import(cs, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			if !hasReason(held, "doc", "tenants/minh-an/notes/tu-file.md", "path-taken") {
				t.Fatalf("hand edit %+v", held.Rejections)
			}
			if cs.puts != 0 || cs.links != 0 || cs.resolves != 0 {
				t.Fatal("hand edit was overwritten")
			}
			got := mustDoc(t, raw, "notes/tu-file.md")
			if !strings.Contains(got.Body, "Dong 9") || strings.Contains(got.Body, "Dong 1 Minh An") {
				t.Fatal("hand edit was lost")
			}
			mem, err := raw.GetMemory(memID)
			if err != nil || mem.Body != memoryMarker {
				t.Fatalf("memory changed %v", err)
			}
			assertNoBodyLeak(t, logBuf.String(), held)
		})
	}
}

func fileBodyExport() *Export {
	at := "2026-03-01T04:05:06Z"
	doc := func(id, tenant, scope, path, title string) Document {
		return Document{ID: id, TenantID: tenant, Scope: scope, Path: path, Title: title, UpdatedAt: at}
	}
	return &Export{
		Documents: []Document{
			doc("doc-ver", sourceTenant, "team", "tenants/minh-an/notes/co-ban.md", "Quy trinh Minh An"),
			doc("doc-file", sourceTenant, "team", "tenants/minh-an/notes/tu-file.md", "Tu file Minh An"),
			doc("doc-miss", sourceTenant, "team", "tenants/minh-an/notes/thieu.md", "Thieu file Minh An"),
			doc("doc-big", sourceTenant, "team", "tenants/minh-an/notes/lon.md", "Lon Minh An"),
			doc("doc-bin", sourceTenant, "team", "tenants/minh-an/notes/nhi.md", "Nhi phan Minh An"),
			doc("doc-nul", sourceTenant, "team", "tenants/minh-an/notes/nul.md", "Nul Minh An"),
			doc("doc-empty", sourceTenant, "team", "tenants/minh-an/notes/rong.md", "Rong Minh An"),
			doc("doc-other", otherTenant, "team", "tenants/minh-an/notes/khac.md", "Khac tenant"),
			doc("doc-personal", sourceTenant, "personal", "tenants/minh-an/notes/rieng.md", "Rieng Minh An"),
			doc("doc-team", sourceTenant, "team", "tenants/minh-an/notes/trung.md", "Team Minh An"),
			doc("doc-per2", sourceTenant, "personal", "tenants/minh-an/notes/trung.md", "Trung ca nhan"),
		},
		Versions: []Version{
			{DocID: "doc-ver", Version: 1, Content: "NOIDUNGBAN", CreatedAt: at},
			{DocID: "doc-personal", Version: 1, Content: "PERSONALBODYMARKER", CreatedAt: at},
			{DocID: "doc-team", Version: 1, Content: "TEAMBODYMARKER", CreatedAt: at},
			{DocID: "doc-per2", Version: 1, Content: "PERSONALBODYMARKER", CreatedAt: at},
		},
		Links: []Link{
			{ID: "l1", FromDoc: "doc-file", ToDoc: "doc-ver", LinkType: "wikilink", Context: contextPhrase},
			{ID: "l2", FromDoc: "doc-ver", ToDoc: "doc-miss", LinkType: "wikilink"},
			{ID: "l3", FromDoc: "doc-other", ToDoc: "doc-ver", LinkType: "wikilink"},
		},
	}
}

func assertFileBodyReport(t *testing.T, rep *Report, inserted, updated, skipped, rejected int) {
	t.Helper()
	assertCounts(t, rep, inserted, updated, skipped, rejected)
	docs, links := 0, 0
	for _, s := range rep.Docs {
		docs += s.Input
		if !s.balanced() {
			t.Fatalf("doc section %+v", s)
		}
	}
	for _, s := range rep.Links {
		links += s.Input
		if !s.balanced() {
			t.Fatalf("link section %+v", s)
		}
	}
	if docs != 11 || links != 3 {
		t.Fatalf("inputs docs=%d links=%d", docs, links)
	}
	srcDocs := sectionBy(t, rep.Docs, sourceTenant)
	otherDocs := sectionBy(t, rep.Docs, otherTenant)
	srcLinks := sectionBy(t, rep.Links, sourceTenant)
	otherLinks := sectionBy(t, rep.Links, otherTenant)
	if srcDocs.Input != srcDocs.Inserted+srcDocs.Updated+srcDocs.Skipped+srcDocs.Rejected {
		t.Fatalf("source docs %+v", srcDocs)
	}
	if otherDocs.Input != otherDocs.Skipped || otherDocs.Reasons[reasonTenantMismatch] != otherDocs.Skipped {
		t.Fatalf("other docs %+v", otherDocs)
	}
	if srcLinks.Input != srcLinks.Inserted+srcLinks.Updated+srcLinks.Skipped+srcLinks.Rejected {
		t.Fatalf("source links %+v", srcLinks)
	}
	if otherLinks.Input != otherLinks.Rejected || otherLinks.Reasons["source-rejected"] != 1 {
		t.Fatalf("other links %+v", otherLinks)
	}
	if inserted == 3 {
		if srcDocs.Inserted != 3 || srcDocs.Rejected != 7 || srcDocs.Reasons["path-missing"] != 1 ||
			srcDocs.Reasons[reasonTooLarge] != 1 || srcDocs.Reasons[reasonBinarySkipped] != 2 ||
			srcDocs.Reasons[reasonEmptyBody] != 1 || srcDocs.Reasons["scope-personal-held"] != 2 ||
			srcDocs.Reasons["scope-conflict"] != 0 {
			t.Fatalf("first source docs %+v", srcDocs)
		}
		if srcLinks.Inserted != 1 || srcLinks.Rejected != 1 || srcLinks.Reasons["outside-tenant"] != 1 {
			t.Fatalf("first source links %+v", srcLinks)
		}
	}
	if skipped == 4 {
		if srcDocs.Skipped != 3 || srcDocs.Reasons[reasonUnchanged] != 3 || srcLinks.Skipped != 1 {
			t.Fatalf("second source docs %+v links %+v", srcDocs, srcLinks)
		}
	}
}

func assertVersionBody(t *testing.T, doc *store.VaultDoc) {
	t.Helper()
	if strings.Contains(doc.Body, "FILEKHACMARKER") || !strings.Contains(doc.Body, "NOIDUNGBAN") {
		t.Fatal("version doc used the file")
	}
	prefix, src, sum, ok := splitImportMarker(doc.Body)
	if !ok || src != "doc-ver" || sum != hashBody(doc.Title+"\n"+prefix) {
		t.Fatal("version provenance sum")
	}
	if sum == hashBody("FILEKHACMARKER") {
		t.Fatal("version sum is the file hash")
	}
	if doc.SHA256 != hashBody(doc.Body) {
		t.Fatal("version row hash")
	}
}

func assertFileBody(t *testing.T, doc *store.VaultDoc) {
	t.Helper()
	if !strings.Contains(doc.Body, "Dong 1 Minh An\nDong 2") || strings.Contains(doc.Body, "\r") {
		t.Fatal("file text was not stored as normalized text")
	}
	prefix, src, sum, ok := splitImportMarker(doc.Body)
	if !ok || src != "doc-file" || sum != hashBody(string(fileBodyRaw)) {
		t.Fatal("file provenance sum")
	}
	if sum == hashBody(doc.Title+"\n"+prefix) {
		t.Fatal("file sum used the version checksum")
	}
	if !fileTextIntact(prefix, normalizeNewlines(string(fileBodyRaw))) {
		t.Fatal("stored prefix lost the file text")
	}
	if doc.SHA256 != hashBody(doc.Body) {
		t.Fatal("file row hash")
	}
}

func assertNoBodyLeak(t *testing.T, logText string, rep *Report) {
	t.Helper()
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"NOIDUNGBAN", "FILEKHACMARKER", "Dong 1 Minh An", "PERSONALBODYMARKER",
		"TEAMBODYMARKER", "TEAMFILEMARKER", "LARGEMARKER", "BINMARKER", "NULMARKER", "CANFILEMARKER",
	} {
		if strings.Contains(logText, marker) || bytes.Contains(raw, []byte(marker)) {
			t.Fatalf("report or log contains %s", marker)
		}
	}
}

func sectionBy(t *testing.T, sections []TenantSection, tenant string) TenantSection {
	t.Helper()
	for _, s := range sections {
		if s.SourceTenant == tenant {
			return s
		}
	}
	t.Fatalf("missing section %s", tenant)
	return TenantSection{}
}

func mustDoc(t *testing.T, st store.StoreIface, path string) *store.VaultDoc {
	t.Helper()
	doc, err := st.FindVaultDocByPath(destTenant, path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func writeSrc(t *testing.T, root, rel string, body []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedMinhAnMemory(t *testing.T, st store.StoreIface) string {
	t.Helper()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-sales", DisplayName: "Co van Minh An", TenantID: destTenant})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: destTenant, Label: "seed"})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := st.PutMemory(store.Memory{SessionID: sess.ID, TenantID: destTenant, Body: memoryMarker})
	if err != nil {
		t.Fatal(err)
	}
	return mem.ID
}
