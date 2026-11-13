// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestVaultPathShapes(t *testing.T) {
	mem, kind, ok := classify("memory/notes.md")
	if !ok || kind != kindMemory || mem != "memory/notes.md" {
		t.Fatalf("memory path %q %q %v", mem, kind, ok)
	}
	got, err := VaultPath("ag", mem)
	if err != nil || got != "memory/goclaw/ag/memory/notes.md" || strings.Contains(got, "memory/memory") {
		t.Fatalf("vault memory path %q %v", got, err)
	}
	root, kind, ok := classify("SOUL.md")
	if !ok || kind != kindRoot {
		t.Fatalf("root %q %v", kind, ok)
	}
	got, err = VaultPath("ag", root)
	if err != nil || got != "memory/goclaw/ag/SOUL.md" || strings.Contains(got, "/memory/SOUL") {
		t.Fatalf("vault root path %q %v", got, err)
	}
	sys, kind, ok := classify("_system/SOUL.md")
	if !ok || kind != kindSystem {
		t.Fatalf("system %q %v", kind, ok)
	}
	got, err = VaultPath("ag", sys)
	if err != nil || got != "memory/goclaw/ag/_system/SOUL.md" || strings.Contains(got, "/memory/_system") {
		t.Fatalf("system mapping %q %v", got, err)
	}
	if _, err := VaultPath("..", "memory/notes.md"); err == nil {
		t.Fatal("agent .. was accepted")
	}
	if _, err := VaultPath("ag/../x", "notes.md"); err == nil {
		t.Fatal("agent with .. was accepted")
	}
	for _, bad := range []string{"", "../x", "memory/../../etc/passwd", "/etc/passwd", `memory\x.md`, "other/x.md", "memory", "_system", "file://x", "C:/x"} {
		if _, _, ok := classify(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestMinhAnImportAndRemoval(t *testing.T) {
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st, root := open(t)
			runMinhAn(t, st, root)
		})
	}
}

func runMinhAn(t *testing.T, st Store, root string) {
	t.Helper()
	minh, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "co-van-minh", DisplayName: "Co van Minh An", Model: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	phap, err := st.CreateAgent(store.Agent{TenantID: "phap-che", AgentKey: "co-van-phap", DisplayName: "Co van phap che", Model: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	svc := vault.New(st, root)
	taken := vp(t, minh.ID, "memory/taken.md")
	liveVault, err := svc.PutTenantAt("minh-an", "wiki/minh-an.md", []byte("# Minh An\nLIVEWIKI\n"))
	if err != nil {
		t.Fatal(err)
	}
	liveTaken, err := svc.PutTenantAt("minh-an", taken, []byte("LIVEKEEP minh an\n"))
	if err != nil {
		t.Fatal(err)
	}
	liveSess, err := st.CreateSession(store.Session{TenantID: "minh-an", AgentID: minh.ID, Label: "Minh An web"})
	if err != nil {
		t.Fatal(err)
	}
	liveMem, err := st.PutMemory(store.Memory{TenantID: "minh-an", SessionID: liveSess.ID, Kind: store.KindEpisodic, Body: "live episodic minh an"})
	if err != nil {
		t.Fatal(err)
	}

	mapping := Mapping{
		Tenants: []TenantMap{
			{SourceID: minhSrc, Slug: "minh-an", Dest: "minh-an"},
			{SourceID: phapSrc, Slug: "phap-che", Dest: "phap-che"},
		},
		Agents: []AgentMap{
			{SourceID: "agent-minh", SourceTenant: minhSrc, DestID: minh.ID},
			{SourceID: "agent-phap", SourceTenant: phapSrc, DestID: phap.ID},
		},
	}
	customers := mustCustomers(t)
	export := minhExport(minhSrc, phapSrc)
	minhOpt := Options{Mapping: mapping, SourceTenant: minhSrc, Deny: allowDeny(t), Customers: customers, DryRun: true}

	dry, err := Import(st, svc, strings.NewReader(export), minhOpt)
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.Inserted != 6 || dry.Sessions != 3 || dry.Denied != 0 {
		t.Fatalf("dry inserted=%d sessions=%d denied=%d", dry.Inserted, dry.Sessions, dry.Denied)
	}
	minhRep := tenantBy(t, dry, minhSrc)
	if minhRep.Tables[tableDocs].Read != 17 || minhRep.Tables[tableDocs].WillWrite != 6 || minhRep.Tables[tableDocs].Vault != 2 || minhRep.Tables[tableDocs].Durable != 4 {
		t.Fatalf("minh docs %+v", minhRep.Tables[tableDocs])
	}
	if minhRep.Tables[tableDocs].Skipped.TeamScope != 2 || minhRep.Tables[tableDocs].Skipped.System != 3 || minhRep.Tables[tableDocs].Skipped.Path != 2 || minhRep.Tables[tableDocs].Skipped.NativeExists != 1 || minhRep.Tables[tableDocs].Skipped.Binary != 1 || minhRep.Tables[tableDocs].Skipped.TooLarge != 1 || minhRep.Tables[tableDocs].Skipped.Unmapped != 1 {
		t.Fatalf("minh skips %+v", minhRep.Tables[tableDocs].Skipped)
	}
	if minhRep.Tables[tableDocs].Skipped.CustomerPrivate != 0 || minhRep.Tables[tableDocs].Skipped.Expired != 0 || minhRep.Tables[tableDocs].Skipped.MissingEndpoint != 0 {
		t.Fatalf("unused counters changed: %+v", minhRep.Tables[tableDocs].Skipped)
	}
	if minhRep.Tables[tableChunks].Read != 1 || minhRep.Tables[tableChunks].Skipped.Chunks != 1 || minhRep.Tables[tableChunks].WillWrite != 0 {
		t.Fatalf("chunks %+v", minhRep.Tables[tableChunks])
	}
	if strings.Join(minhRep.SystemNames, ",") != "SOUL.md,TOOLS.md" {
		t.Fatalf("system names %q", minhRep.SystemNames)
	}
	phapDry := tenantBy(t, dry, phapSrc)
	if phapDry.Tables[tableDocs].Read != 2 || phapDry.Tables[tableDocs].Skipped.OtherTenant != 2 || phapDry.Tables[tableDocs].WillWrite != 0 {
		t.Fatalf("other tenant %+v", phapDry.Tables[tableDocs])
	}
	raw, _ := json.Marshal(dry)
	for _, secret := range []string{"SOUL.md", "TOOLS.md", "SYSBODY", "USERONLY", "CHUNKSECRET", "TEAMSKIP", "SCOPESKIP", "TRAVERSAL", "hopdongx", "TAKENOVER", "UNMAPPED", "khach-x", "khach-y", "khach-z"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("dry-run report contains %s", secret)
		}
	}
	if files := readVault(t, root); strings.Contains(files, "VAULTMEM") || strings.Contains(files, "SYSBODY") {
		t.Fatal("dry-run wrote a file")
	}

	minhOpt.DryRun = false
	rep, err := Import(st, svc, strings.NewReader(export), minhOpt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.DryRun || rep.Inserted != 6 || rep.Sessions != 3 || rep.Denied != 0 {
		t.Fatalf("import inserted=%d sessions=%d dry=%v", rep.Inserted, rep.Sessions, rep.DryRun)
	}
	notesPath := vp(t, minh.ID, "memory/notes.md")
	policyPath := vp(t, minh.ID, "POLICY.md")
	notes, err := st.FindVaultDocByPath("minh-an", notesPath)
	if err != nil || notes.ID != VaultID("minh-an", minh.ID, notesPath) || notes.Body != "VAULTMEM minh an notes" || !strings.HasPrefix(notes.ID, "gc-") {
		t.Fatalf("notes %+v %v", notes, err)
	}
	policy, err := st.FindVaultDocByPath("minh-an", policyPath)
	if err != nil || policy.Body != "VAULTROOT minh an policy" || strings.Contains(policy.Path, "memory/memory") {
		t.Fatalf("policy %+v %v", policy, err)
	}
	if _, err := st.FindVaultDocByPath("minh-an", vp(t, minh.ID, "_system/SOUL.md")); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("system file was imported")
	}
	kept, err := st.FindVaultDocByPath("minh-an", taken)
	if err != nil || kept.ID != liveTaken.ID || kept.Body != "LIVEKEEP minh an\n" {
		t.Fatalf("taken path changed %+v", kept)
	}
	phapOpt := minhOpt
	phapOpt.SourceTenant = phapSrc
	if _, err := Import(st, svc, strings.NewReader(export), phapOpt); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindVaultDocByPath("phap-che", notesPath); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("tenant phap-che saw minh-an's vault path")
	}
	phapDoc, err := st.FindVaultDocByPath("phap-che", vp(t, phap.ID, "README.md"))
	if err != nil || phapDoc.Body != "VAULTPHAP minh an" {
		t.Fatalf("phap doc %+v %v", phapDoc, err)
	}
	if _, err := st.FindVaultDocByPath("minh-an", vp(t, phap.ID, "README.md")); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("minh-an saw phap-che's file")
	}

	xID := MemoryID(minh.ID, "khach-x", "memory/hop-x.md")
	x, err := st.GetMemory(xID)
	if err != nil || x.Kind != store.KindDurable || x.CustomerKey != "goclaw:khach-x" || x.Body != "memory/hop-x.md\nMinh An chietkhau hopdongx" {
		t.Fatalf("durable x %+v %v", x, err)
	}
	y, err := st.GetMemory(MemoryID(minh.ID, "khach-y", "memory/hop-y.md"))
	if err != nil || y.CustomerKey != "goclaw:khach-y" || !strings.Contains(y.Body, "hopdongy") {
		t.Fatalf("durable y %+v %v", y, err)
	}
	z, err := st.GetMemory(MemoryID(minh.ID, "khach-z", "NOTES.md"))
	if err != nil || z.CustomerKey != "zalo-personal:minh" || !strings.HasPrefix(z.Body, "NOTES.md\n") {
		t.Fatalf("durable z %+v %v", z, err)
	}
	sess, err := st.GetSession(SessionID(minh.ID, "khach-x"))
	if err != nil || !sess.Archived || sess.AgentID != minh.ID || sess.TenantID != "minh-an" {
		t.Fatalf("session %+v %v", sess, err)
	}
	xs, err := st.ListMemories(sess.ID)
	if err != nil || len(xs) != 2 {
		t.Fatalf("session notes %d %v", len(xs), err)
	}
	for _, m := range xs {
		if m.CustomerKey == "" || !strings.HasPrefix(m.ID, "gc-doc:") {
			t.Fatalf("bad note %+v", m)
		}
	}

	assertRecall(t, st, minh.ID, "goclaw:khach-x", "hopdongx", "hopdongy", "hopdongz", "PHAPONLY")
	assertRecall(t, st, minh.ID, "goclaw:khach-y", "hopdongy", "hopdongx", "hopdongz", "PHAPONLY")
	assertRecall(t, st, minh.ID, "zalo-personal:minh", "hopdongz", "hopdongx", "hopdongy", "PHAPONLY")
	assertRecall(t, st, minh.ID, "zalo-personal:other", "", "hopdongx", "hopdongy", "hopdongz")
	hits, err := st.RecallMemories(store.RecallQuery{TenantID: "phap-che", AgentID: phap.ID, ExcludeSession: "ask", Query: "chietkhau", Limit: 20, CustomerScoped: true, CustomerKey: "goclaw:khach-x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if strings.Contains(h.Memory.Body, "hopdongx") || h.Memory.TenantID == "minh-an" {
			t.Fatalf("phap-che recall saw minh-an %+v", h.Memory)
		}
	}

	files := readVault(t, root)
	for _, secret := range []string{"SYSBODY", "USERONLY", "CHUNKSECRET", "MQSECRET", "TEAMSKIP", "SCOPESKIP", "TRAVERSAL", "TAKENOVER", "UNMAPPED", "BIN"} {
		if strings.Contains(files, secret) {
			t.Fatalf("vault file contains %s", secret)
		}
	}
	if !strings.Contains(files, "VAULTMEM minh an notes") || !strings.Contains(files, "LIVEKEEP minh an") || !strings.Contains(files, "LIVEWIKI") {
		t.Fatal("expected vault text missing")
	}

	again, err := Import(st, svc, strings.NewReader(export), minhOpt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Inserted != 0 || again.Sessions != 0 || tenantBy(t, again, minhSrc).Tables[tableDocs].WillWrite != 0 || tenantBy(t, again, minhSrc).Tables[tableDocs].Skipped.Hash != 6 {
		t.Fatalf("second run inserted=%d sessions=%d hash=%d", again.Inserted, again.Sessions, tenantBy(t, again, minhSrc).Tables[tableDocs].Skipped.Hash)
	}
	if got, _ := st.GetMemory(xID); got == nil || got.Body != x.Body || got.CustomerKey != x.CustomerKey {
		t.Fatal("second run changed a durable note")
	}
	if sum, err := st.SaveSummary(liveSess.ID, "live summary minh an"); err != nil || sum.SessionID != liveSess.ID {
		t.Fatal(err)
	}
	if _, err := svc.PutTenantAt("minh-an", "wiki/khac.md", []byte("# Minh An\nLIVEOTHER\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetMemory(xID); got == nil || got.Body != x.Body {
		t.Fatal("live write changed an imported note")
	}
	if got, _ := st.LatestSummary(liveSess.ID); got == nil || got.Body != "live summary minh an" {
		t.Fatal("live summary was lost")
	}
	if got, _ := st.GetMemory(liveMem.ID); got == nil || got.Body != "live summary minh an" {
		t.Fatalf("live episodic note %+v", got)
	}
	onImport, err := st.PutMemory(store.Memory{TenantID: "minh-an", SessionID: sess.ID, Kind: store.KindDurable, Body: "live note on imported session"})
	if err != nil || strings.HasPrefix(onImport.ID, "gc-") {
		t.Fatalf("live note on imported session %+v %v", onImport, err)
	}

	removed, err := RemoveTenant(st, root, "minh-an")
	if err != nil {
		t.Fatal(err)
	}
	if removed.Memories < 4 || removed.VaultDocs < 2 {
		t.Fatalf("removal %+v", removed)
	}
	if _, err := st.GetMemory(xID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("imported note survived removal")
	}
	if _, err := st.FindVaultDocByPath("minh-an", notesPath); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("imported vault doc survived removal")
	}
	if got, _ := st.GetMemory(onImport.ID); got == nil || got.Body != "live note on imported session" {
		t.Fatal("live note on the imported session was removed")
	}
	if _, err := st.GetSession(sess.ID); err != nil {
		t.Fatal("session that still holds a live note was removed")
	}
	if got, _ := st.GetSession(liveSess.ID); got == nil || got.ID != liveSess.ID {
		t.Fatal("live session removed")
	}
	if got, _ := st.LatestSummary(liveSess.ID); got == nil || got.Body != "live summary minh an" {
		t.Fatal("live summary removed")
	}
	if got, _ := st.GetVaultDoc(liveVault.ID); got == nil || !strings.Contains(got.Body, "LIVEWIKI") {
		t.Fatal("live wiki removed")
	}
	if got, _ := st.GetVaultDoc(liveTaken.ID); got == nil || got.Body != "LIVEKEEP minh an\n" {
		t.Fatal("live taken doc removed")
	}
	if got, _ := st.GetVaultDoc(phapDoc.ID); got == nil || got.Body != "VAULTPHAP minh an" {
		t.Fatal("other tenant vault doc removed")
	}
	if files := readVault(t, root); strings.Contains(files, "VAULTMEM") || !strings.Contains(files, "LIVEKEEP minh an") || !strings.Contains(files, "VAULTPHAP") {
		t.Fatal("vault files after removal are wrong")
	}
	if _, err := RemoveTenant(st, root, "phap-che"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetVaultDoc(phapDoc.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("phap doc survived its own removal")
	}
	if got, _ := st.GetVaultDoc(liveVault.ID); got == nil || !strings.Contains(got.Body, "LIVEWIKI") {
		t.Fatal("minh live wiki removed with phap-che")
	}
}

func assertRecall(t *testing.T, st Store, agent, key, want string, forbidden ...string) {
	t.Helper()
	hits, err := st.RecallMemories(store.RecallQuery{
		TenantID: "minh-an", AgentID: agent, ExcludeSession: "ask",
		Query: "chietkhau", Limit: 20, CustomerScoped: true, CustomerKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, h := range hits {
		if h.Memory.CustomerKey != key {
			t.Fatalf("recall %s returned key %s", key, h.Memory.CustomerKey)
		}
		b.WriteString(h.Memory.Body)
	}
	got := b.String()
	if want != "" && !strings.Contains(got, want) {
		t.Fatalf("recall %s missing %s in %q", key, want, got)
	}
	if want == "" && got != "" {
		t.Fatalf("recall %s returned %q", key, got)
	}
	for _, bad := range forbidden {
		if strings.Contains(got, bad) {
			t.Fatalf("recall %s saw %s", key, bad)
		}
	}
}

func tenantBy(t *testing.T, rep *Report, source string) TenantReport {
	t.Helper()
	for _, sec := range rep.Tenants {
		if sec.Source == source {
			return sec
		}
	}
	t.Fatalf("missing tenant %s", source)
	return TenantReport{}
}

func readVault(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || !info.Mode().IsRegular() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(body)
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}

const (
	minhSrc = "11111111-1111-4111-8111-111111111111"
	phapSrc = "22222222-2222-4222-8222-222222222222"
)

func minhExport(minh, phap string) string {
	lines := []any{
		doc(minh, "agent-minh", nil, "memory/notes.md", "VAULTMEM minh an notes", nil, nil),
		doc(minh, "agent-minh", nil, "POLICY.md", "VAULTROOT minh an policy", nil, nil),
		doc(minh, "agent-minh", nil, "_system/SOUL.md", "SYSBODY minh an", nil, nil),
		doc(minh, "agent-minh", nil, "_system/TOOLS.md", "SYSBODY tools", nil, nil),
		doc(minh, "agent-minh", "khach-x", "_system/user-only.md", "USERONLY", nil, nil),
		doc(minh, "agent-minh", "khach-x", "memory/hop-x.md", "Minh An chietkhau hopdongx", nil, nil),
		doc(minh, "agent-minh", "khach-x", "memory/hop-x-2.md", "Minh An chietkhau hopdongx extra", nil, nil),
		doc(minh, "agent-minh", "khach-y", "memory/hop-y.md", "Minh An chietkhau hopdongy", nil, nil),
		doc(minh, "agent-minh", "khach-z", "NOTES.md", "Minh An chietkhau hopdongz", nil, nil),
		doc(minh, "agent-minh", nil, "memory/team.md", "TEAMSKIP", "44444444-4444-4444-8444-444444444444", nil),
		doc(minh, "agent-minh", nil, "memory/scope.md", "SCOPESKIP", nil, "rieng"),
		doc(minh, "agent-minh", nil, "memory/../../etc/passwd", "TRAVERSAL", nil, nil),
		doc(minh, "agent-minh", nil, "other/nope.md", "OTHERPATH", nil, nil),
		chunk(minh, "agent-minh", "CHUNKSECRET"),
		doc(minh, "agent-minh", nil, "memory/taken.md", "TAKENOVER", nil, nil),
		doc(minh, "agent-minh", nil, "memory/bin.md", "BIN\x00ARY", nil, nil),
		doc(minh, "agent-minh", nil, "memory/big.md", strings.Repeat("x", security.MaxAPIBody+1), nil, nil),
		doc(minh, "missing-agent", nil, "memory/no-agent.md", "UNMAPPEDAGENT", nil, nil),
		doc(phap, "agent-phap", nil, "README.md", "VAULTPHAP minh an", nil, nil),
		doc(phap, "agent-phap", "khach-x", "memory/hop-x.md", "PHAPONLY chietkhau", nil, nil),
		doc("99999999-9999-4999-8999-999999999999", "agent-minh", nil, "memory/nope.md", "UNMAPPEDTENANT", nil, nil),
	}
	var b strings.Builder
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			panic(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func vp(t *testing.T, agent, src string) string {
	t.Helper()
	p, err := VaultPath(agent, src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func allowDeny(t *testing.T) *DenyList {
	t.Helper()
	d, err := ReadDenyList(strings.NewReader("held-out\n"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mustCustomers(t *testing.T) *CustomerIndex {
	t.Helper()
	idx, err := ReadCustomerMap(strings.NewReader("zalo_personal\tminh\tkhach-z\tzalo_personal:minh\n"))
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func doc(tenant, agent string, user any, path, content string, team, scope any) map[string]any {
	return map[string]any{
		"id": "doc-" + path, "agent_id": agent, "user_id": user, "path": path,
		"content": content, "hash": "abc123", "created_at": "2026-03-01T04:05:06Z",
		"updated_at": "2026-03-02T04:05:06Z", "team_id": team, "tenant_id": tenant,
		"custom_scope": scope,
	}
}

func chunk(tenant, agent, text string) map[string]any {
	return map[string]any{
		"id": "chunk-1", "agent_id": agent, "document_id": "doc-memory/notes.md",
		"user_id": nil, "path": "memory/notes.md", "start_line": 1, "end_line": 2,
		"hash": "abc123", "text": text, "tenant_id": tenant,
	}
}

func backends(t *testing.T) map[string]func(t *testing.T) (Store, string) {
	t.Helper()
	out := map[string]func(t *testing.T) (Store, string){
		"memory": func(t *testing.T) (Store, string) {
			return store.New(), t.TempDir()
		},
		"sqlite": func(t *testing.T) (Store, string) {
			s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "memdoc.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s, t.TempDir()
		},
	}
	if pg := openPostgres(t); pg != nil {
		out["postgres"] = func(t *testing.T) (Store, string) {
			return pg, t.TempDir()
		}
	}
	return out
}

func openPostgres(t *testing.T) Store {
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
	schema := "memdoc180"
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
	t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`) })
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

func TestDeniedTenantWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		token  string
		source string
		slug   string
		line   string
	}{
		{name: "slug", token: "held-out", source: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", slug: "held-out", line: "held-out"},
		{name: "uuid", token: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", source: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", slug: "held-uuid", line: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := store.New()
			ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "co-van-" + tc.name, DisplayName: "Co van Minh An", Model: "echo"})
			if err != nil {
				t.Fatal(err)
			}
			svc := vault.New(st, t.TempDir())
			live, err := svc.PutTenantAt("minh-an", "wiki/minh-an.md", []byte("# Minh An\nLIVEWIKI\n"))
			if err != nil {
				t.Fatal(err)
			}
			deny, err := ReadDenyList(strings.NewReader(tc.line + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			mapping := Mapping{
				Tenants: []TenantMap{{SourceID: tc.source, Slug: tc.slug, Dest: "minh-an"}},
				Agents:  []AgentMap{{SourceID: "agent-1", SourceTenant: tc.source, DestID: ag.ID}},
			}
			export := docLine(tc.source, "agent-1", "memory/notes.md", "SHOULDNOTWRITE")
			_, err = Import(st, svc, strings.NewReader(export), Options{Mapping: mapping, SourceTenant: tc.token, Deny: deny})
			var denied *DeniedError
			if !errors.As(err, &denied) || denied.Count < 1 {
				t.Fatalf("err=%v", err)
			}
			if got, _ := st.GetVaultDoc(live.ID); got == nil || !strings.Contains(got.Body, "LIVEWIKI") {
				t.Fatal("live row changed")
			}
			docs, err := st.ListImportedMemdocVault("minh-an")
			if err != nil || len(docs) != 0 {
				t.Fatalf("imported vault docs %d %v", len(docs), err)
			}
			if files := readVault(t, svc.Root); strings.Contains(files, "SHOULDNOTWRITE") {
				t.Fatal("denied tenant wrote a file")
			}
		})
	}
}

func TestCustomerMapNormalizesChannel(t *testing.T) {
	idx, err := ReadCustomerMap(strings.NewReader("zalo_personal\tminh\tkhach-z\tzalo_personal:minh\nlark\to1\tkhach-l\tlark:o1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.Key("khach-z"); got != "zalo-personal:minh" {
		t.Fatalf("zalo key %q", got)
	}
	if got := idx.Key("khach-l"); got != "feishu:o1" {
		t.Fatalf("lark key %q", got)
	}
	if got := idx.Key("khach-x"); got != "goclaw:khach-x" {
		t.Fatalf("fallback %q", got)
	}
}

func TestRollbackLeavesOtherKind(t *testing.T) {
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st, root := open(t)
			ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "co-van-roll", DisplayName: "Co van Minh An", Model: "echo"})
			if err != nil {
				t.Fatal(err)
			}
			svc := vault.New(st, root)
			mapping := Mapping{
				Tenants: []TenantMap{{SourceID: minhSrc, Slug: "minh-an", Dest: "minh-an"}},
				Agents:  []AgentMap{{SourceID: "agent-minh", SourceTenant: minhSrc, DestID: ag.ID}},
			}
			export := docLine(minhSrc, "agent-minh", "memory/notes.md", "VAULTMEM minh an") +
				docLineUser(minhSrc, "agent-minh", "khach-x", "memory/hop.md", "Minh An hopdongx")
			opt := Options{Mapping: mapping, SourceTenant: minhSrc, Deny: allowDeny(t)}
			if _, err := Import(st, svc, strings.NewReader(export), opt); err != nil {
				t.Fatal(err)
			}
			ok, err := st.InsertSessionIfNew(store.Session{
				ID: "gc-sess:sample", TenantID: "minh-an", AgentID: ag.ID, Archived: true,
			})
			if err != nil || !ok {
				t.Fatalf("seed gc-sess %v %v", ok, err)
			}
			ok, err = st.InsertMemoryIfNew(store.Memory{
				ID: "gc-ep:sample", TenantID: "minh-an", SessionID: "gc-sess:sample",
				Kind: store.KindEpisodic, Body: "episodic kind stays", CustomerKey: "goclaw:other",
			})
			if err != nil || !ok {
				t.Fatalf("seed gc-ep %v %v", ok, err)
			}
			epBefore, err := st.GetMemory("gc-ep:sample")
			if err != nil {
				t.Fatal(err)
			}
			sessBefore, err := st.GetSession("gc-sess:sample")
			if err != nil || sessBefore.TenantID != "minh-an" {
				t.Fatal(err)
			}
			if _, err := RemoveTenant(st, root, "minh-an"); err != nil {
				t.Fatal(err)
			}
			epAfter, err := st.GetMemory("gc-ep:sample")
			if err != nil || epAfter.Body != epBefore.Body {
				t.Fatalf("gc-ep changed %+v %v", epAfter, err)
			}
			sessAfter, err := st.GetSession("gc-sess:sample")
			if err != nil || sessAfter.ID != sessBefore.ID || sessAfter.AgentID != ag.ID {
				t.Fatalf("gc-sess changed %+v %v", sessAfter, err)
			}
			docs, err := st.ListImportedMemdocVault("minh-an")
			if err != nil || len(docs) != 0 {
				t.Fatalf("vault rows left %d", len(docs))
			}
			if _, err := st.GetMemory(MemoryID(ag.ID, "khach-x", "memory/hop.md")); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("durable note survived rollback")
			}
			if _, err := st.GetSession(SessionID(ag.ID, "khach-x")); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("imported session survived rollback")
			}
		})
	}
}

func docLine(tenant, agent, path, content string) string {
	raw, _ := json.Marshal(doc(tenant, agent, nil, path, content, nil, nil))
	return string(raw) + "\n"
}

func docLineUser(tenant, agent, user, path, content string) string {
	raw, _ := json.Marshal(doc(tenant, agent, user, path, content, nil, nil))
	return string(raw) + "\n"
}
