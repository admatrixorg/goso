// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const canary = "EMBEDMARKERvector OLD-VERSION-BODY token-should-not-leak OWNERMARKERlan AGENTMARKERno USERMARKERno MOTA-KHONG-GHI SYSTEM-SHOULD-NOT-IMPORT DISABLED-SHOULD-NOT-IMPORT ARCHIVED-SHOULD-NOT-IMPORT CFG-SHOULD-NOT-IMPORT"

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Bao_Gia_Minh_An", "bao-gia-minh-an", true},
		{"Tư vấn + Hóa đơn", "tu-van-hoa-don", true},
		{"don-hang", "don-hang", true},
		{"web_search-patterns", "web-search-patterns", true},
		{"Self-Improving + Proactive Agent", "self-improving-proactive-agent", true},
		{"Đơn", "don", true},
		{"", "", false},
		{"---", "", false},
	}
	for _, tc := range cases {
		got, ok := slugify(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("slugify %q = %q %v, want %q %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	long := strings.Repeat("a", 70)
	got, ok := slugify(long)
	if !ok || len(got) != 64 || got != strings.Repeat("a", 64) {
		t.Fatalf("long slug %q %v", got, ok)
	}
	if _, ok := withSuffix(strings.Repeat("b", 64), 2); !ok {
		t.Fatal("suffix of a 64-char name should still fit")
	}
}

func TestMinhAnFixture(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.Mkdir(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	before := dirSnapshot(t, skillsDir)
	mapPath := filepath.Join(dir, "rename-map.json")
	var logBuf bytes.Buffer
	rep, err := Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath, Apply: false, Log: bufLog{&logBuf},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Fatal("dry-run flag")
	}
	if snap := dirSnapshot(t, skillsDir); snap != before {
		t.Fatalf("dry-run wrote skills dir: %s", snap)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, sourceMapFile)); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote the source map")
	}
	minh := tenantCount(t, rep, "minh-an")
	khac := tenantCount(t, rep, "khac")
	if minh.SkillsInput != 12 || minh.Inserted != 6 || minh.Skipped != 0 || minh.Rejected != 6 {
		t.Fatalf("minh-an counts %+v", minh)
	}
	if khac.SkillsInput != 2 || khac.Inserted != 0 || khac.Rejected != 2 || khac.Reasons["foreign-tenant"] != 2 {
		t.Fatalf("khac counts %+v", khac)
	}
	if minh.VersionsInput != 3 || minh.VersionsNotImported != 3 || khac.VersionsNotImported != 1 {
		t.Fatalf("versions minh=%+v khac=%+v", minh, khac)
	}
	if minh.GrantsInput != 4 || minh.GrantsNotImported != 4 || minh.GrantsRewritten != 2 {
		t.Fatalf("minh grants %+v", minh)
	}
	if minh.ConfigsInput != 2 || minh.ConfigsNotImported != 2 {
		t.Fatalf("configs %+v", minh)
	}
	assertRename(t, rep, "minh-an-skill-bao-gia", "Bao_Gia_Minh_An", "bao-gia-minh-an", true)
	assertRename(t, rep, "minh-an-skill-tu-van", "Tư vấn + Hóa đơn", "tu-van-hoa-don", true)
	assertRename(t, rep, "minh-an-skill-don-a", "Don Hang", "don-hang", true)
	assertRename(t, rep, "minh-an-skill-don-b", "don-hang", "don-hang-2", true)
	assertRename(t, rep, "minh-an-skill-system", "minh-an-he-thong", "minh-an-he-thong", false)
	assertGrant(t, rep, "g-agent-bao", "agent", "Bao_Gia_Minh_An", "bao-gia-minh-an")
	assertGrant(t, rep, "g-user-bao", "user", "Bao_Gia_Minh_An", "bao-gia-minh-an")
	g := findGrant(rep, "g-agent-system")
	if g.New != "" || g.Reason != "system-skill" {
		t.Fatalf("system grant %+v", g)
	}
	g = findGrant(rep, "g-user-cfg")
	if g.New != "" || g.Reason != "disabled" {
		t.Fatalf("cfg grant %+v", g)
	}
	g = findGrant(rep, "g-agent-khac")
	if g.New != "khac-don" || g.Reason != "foreign-tenant" {
		t.Fatalf("khac grant %+v", g)
	}
	mapRaw, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(mapRaw, []byte(`"new": "bao-gia-minh-an"`)) {
		t.Fatalf("rename map missing new name: %s", mapRaw)
	}
	assertNoCanary(t, string(mapRaw)+"\n"+logBuf.String()+mustReport(t, rep))

	logBuf.Reset()
	rep, err = Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath, Apply: true, Log: bufLog{&logBuf},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DryRun || tenantCount(t, rep, "minh-an").Inserted != 6 {
		t.Fatalf("apply %+v", tenantCount(t, rep, "minh-an"))
	}
	body := readSkill(t, skillsDir, "bao-gia-minh-an")
	if !strings.Contains(body, "LATEST-BAO-GIA") || strings.Contains(body, "OLD-VERSION-BODY") || strings.Contains(body, "MOTA-KHONG-GHI") {
		t.Fatalf("body %q", body)
	}
	for _, name := range []string{"tu-van-hoa-don", "don-hang", "don-hang-2", "lich-goi", "san-co"} {
		if readSkill(t, skillsDir, name) == "" {
			t.Fatal("missing", name)
		}
	}
	for _, banned := range []string{"minh-an-he-thong", "tam-tat", "luu-tru", "cfg-tat", "thoat", "rong", "khac-don", "bao-gia-minh-an-2"} {
		if _, err := os.Stat(filepath.Join(skillsDir, banned)); !os.IsNotExist(err) {
			t.Fatal("wrote rejected skill", banned)
		}
	}
	if _, err := os.Stat(filepath.Join(fixtureSrc(t), "skills", "bao-gia", "EXECUTED-SKILL")); !os.IsNotExist(err) {
		t.Fatal("skill script executed")
	}
	if _, err := os.Stat("EXECUTED-SKILL"); !os.IsNotExist(err) {
		t.Fatal("skill script executed in cwd")
	}
	t.Setenv("GOSO_SKILLS_DIR", skillsDir)
	doc, err := skill.Load("bao-gia-minh-an")
	if err != nil || !strings.Contains(doc.Body, "LATEST-BAO-GIA") {
		t.Fatalf("load %+v %v", doc, err)
	}
	listed, err := skill.List()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, info := range listed {
		names[info.Name] = true
	}
	if !names["bao-gia-minh-an"] || names["Bao_Gia_Minh_An"] || names[sourceMapFile] {
		t.Fatalf("list %#v", listed)
	}
	sm, err := loadSourceMap(skillsDir)
	if err != nil || len(sm.Entries) != 6 {
		t.Fatalf("source map %+v %v", sm, err)
	}

	beforeFiles := dirSnapshot(t, skillsDir)
	rep, err = Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath, Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	again := tenantCount(t, rep, "minh-an")
	if again.Inserted != 0 || again.Skipped != 6 || again.Rejected != 6 || again.Reasons["unchanged"] != 6 {
		t.Fatalf("rerun %+v", again)
	}
	if dirSnapshot(t, skillsDir) != beforeFiles {
		t.Fatal("rerun changed skill files")
	}
	sm2, err := loadSourceMap(skillsDir)
	if err != nil || len(sm2.Entries) != 6 {
		t.Fatal("source map grew", len(sm2.Entries))
	}

	srcCopy := filepath.Join(dir, "src")
	if err := copyDir(fixtureSrc(t), srcCopy); err != nil {
		t.Fatal(err)
	}
	bao := filepath.Join(srcCopy, "skills", "bao-gia", "SKILL.md")
	if err := os.WriteFile(bao, []byte("CHANGED-BODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kept := readSkill(t, skillsDir, "bao-gia-minh-an")
	rep, err = Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: srcCopy,
		RenameMapPath: mapPath, Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	held := tenantCount(t, rep, "minh-an")
	if held.Reasons["add-only"] != 1 || held.Reasons["unchanged"] != 5 || held.Inserted != 0 {
		t.Fatalf("add-only %+v", held)
	}
	if readSkill(t, skillsDir, "bao-gia-minh-an") != kept {
		t.Fatal("add-only overwrote a skill")
	}
	assertNoCanary(t, logBuf.String()+mustReport(t, rep)+readTree(t, skillsDir))
}

func TestTenantIsolation(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "san-co"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "san-co", "SKILL.md"), []byte("KEEP\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "notes.txt"), []byte("keep-note"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "rename.json")
	if _, err := Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath, Apply: true,
	}); err != nil {
		t.Fatal(err)
	}
	if readSkill(t, skillsDir, "san-co") != "KEEP\n" {
		t.Fatal("pre-existing skill was overwritten")
	}
	if strings.TrimSpace(readSkill(t, skillsDir, "san-co-2")) != "SAN-CO-IMPORT" {
		t.Fatal("colliding skill was not added beside the existing folder")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "khac-don")); !os.IsNotExist(err) {
		t.Fatal("minh-an import wrote the other tenant")
	}
	bao := readSkill(t, skillsDir, "bao-gia-minh-an")
	if _, err := Import(ex, Options{
		Tenant: "khac", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath, Apply: true,
	}); err == nil {
		t.Fatal("khac apply reused the minh-an rename map")
	}
	if readSkill(t, skillsDir, "bao-gia-minh-an") != bao {
		t.Fatal("rejected khac apply changed minh-an")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "khac-don")); !os.IsNotExist(err) {
		t.Fatal("rejected khac apply wrote a skill")
	}
	if _, err := Import(ex, Options{
		Tenant: "khac", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "rename-khac.json"), Apply: true,
	}); err != nil {
		t.Fatal(err)
	}
	if readSkill(t, skillsDir, "bao-gia-minh-an") != bao {
		t.Fatal("khac import changed minh-an bytes")
	}
	if readSkill(t, skillsDir, "san-co") != "KEEP\n" {
		t.Fatal("khac import changed a pre-existing skill")
	}
	if strings.TrimSpace(readSkill(t, skillsDir, "khac-don")) != "KHAC-ONLY" {
		t.Fatal("khac skill missing")
	}
	if strings.TrimSpace(readSkill(t, skillsDir, "bao-gia-minh-an-2")) != "KHAC-COLLIDE" {
		t.Fatal("colliding khac skill overwrote minh-an or was dropped")
	}
	note, err := os.ReadFile(filepath.Join(skillsDir, "notes.txt"))
	if err != nil || string(note) != "keep-note" {
		t.Fatal("notes.txt changed")
	}
	sm, err := loadSourceMap(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	var minhN, khacN int
	for _, e := range sm.Entries {
		switch e.TenantID {
		case "minh-an":
			minhN++
		case "khac":
			khacN++
		default:
			t.Fatal("unexpected tenant", e.TenantID)
		}
	}
	if minhN != 6 || khacN != 2 {
		t.Fatalf("source map tenants minh=%d khac=%d", minhN, khacN)
	}
}

func TestRenameMapMismatchWritesNothing(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mapPath := filepath.Join(dir, "rename.json")
	if err := os.WriteFile(mapPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath, Apply: true,
	})
	if err == nil || !strings.Contains(err.Error(), "rename map does not match") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(skillsDir); !os.IsNotExist(statErr) {
		t.Fatal("mismatch created the skills dir")
	}
}

func TestEdges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(src, "skills", "big"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "skills", "big", "SKILL.md"), bytes.Repeat([]byte("a"), skill.MaxBody+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "skills", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "skills", "bin", "SKILL.md"), []byte("ok\x00no"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "skills", "link"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("SYMLINK-TARGET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(src, "skills", "link", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "skills", "crlf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "skills", "crlf", "SKILL.md"), []byte("dong\r\nmot\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ex := &Export{Skills: []SkillRow{
		{ID: "big", TenantID: "minh-an", Slug: "qua-lon", Status: "active", FilePath: "skills/big/SKILL.md"},
		{ID: "bin", TenantID: "minh-an", Slug: "nhi-phan", Status: "active", FilePath: "skills/bin/SKILL.md"},
		{ID: "link", TenantID: "minh-an", Slug: "lien-ket", Status: "active", FilePath: "skills/link/SKILL.md"},
		{ID: "abs", TenantID: "minh-an", Slug: "tuyet-doi", Status: "active", FilePath: "/tmp/SKILL.md"},
		{ID: "crlf", TenantID: "minh-an", Slug: "xuong-dong", Status: "active", FilePath: "skills/crlf/SKILL.md"},
		{ID: "crlf", TenantID: "minh-an", Slug: "xuong-dong", Status: "active", FilePath: "skills/crlf/SKILL.md"},
	}}
	skillsDir := filepath.Join(dir, "skills")
	rep, err := Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: src,
		RenameMapPath: filepath.Join(dir, "map.json"), Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := tenantCount(t, rep, "minh-an")
	if c.Inserted != 1 || c.Reasons["too-large"] != 1 || c.Reasons["binary-skipped"] != 1 || c.Reasons["symlink"] != 1 || c.Reasons["path-escape"] != 1 || c.Reasons["duplicate"] != 1 {
		t.Fatalf("edges %+v", c)
	}
	if readSkill(t, skillsDir, "xuong-dong") != "dong\nmot\n" {
		t.Fatalf("crlf body %q", readSkill(t, skillsDir, "xuong-dong"))
	}
	tree := readTree(t, skillsDir)
	if strings.Contains(tree, "SYMLINK-TARGET") {
		t.Fatal("followed a symlink")
	}
}

func TestJSONL(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"table":"skills","id":"s1","tenant_id":"minh-an","slug":"mot","status":"active","file_path":"a/SKILL.md","enabled":true}`,
		`{"table":"skill_versions","id":"v1","tenant_id":"minh-an","skill_id":"s1","version":1,"changed_files":["OLD-VERSION-BODY"]}`,
		`{"table":"skill_agent_grants","id":"g1","tenant_id":"minh-an","skill_id":"s1","agent_id":"AGENTMARKERno","granted_by":"token-should-not-leak"}`,
		`{"table":"nope"}`,
	}, "\n"))
	if _, err := ReadExport(bytes.NewReader(raw)); err == nil {
		t.Fatal("unknown table should fail")
	}
	raw = raw[:bytes.LastIndex(raw, []byte("\n"))]
	ex, err := ReadExport(bytes.NewReader(raw))
	if err != nil || len(ex.Skills) != 1 || len(ex.Versions) != 1 || len(ex.AgentGrants) != 1 {
		t.Fatalf("jsonl %+v %v", ex, err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "a")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("JSONL-BODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(dir, "out")
	rep, err := Import(ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: filepath.Join(dir, "src"),
		RenameMapPath: filepath.Join(dir, "map.json"), Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tenantCount(t, rep, "minh-an").Inserted != 1 || readSkill(t, skillsDir, "mot") != "JSONL-BODY\n" {
		t.Fatalf("jsonl import %+v", rep)
	}
	if strings.Contains(mustReport(t, rep), "AGENTMARKERno") || strings.Contains(mustReport(t, rep), "OLD-VERSION-BODY") {
		t.Fatal("jsonl report leaked")
	}
}

func TestImportLeavesMemoryUntouched(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "goso.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "minh-an-sales", DisplayName: "Co van Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{TenantID: "minh-an", AgentID: ag.ID, Label: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := st.PutMemory(store.Memory{TenantID: "minh-an", SessionID: sess.ID, Kind: "note", Body: "Minh An ghi chu mau"})
	if err != nil {
		t.Fatal(err)
	}
	ex := loadFixture(t)
	if _, err := Import(ex, Options{
		Tenant: "minh-an", SkillsDir: filepath.Join(dir, "skills"), SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"), Apply: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetMemory(mem.ID)
	if err != nil || got.Body != "Minh An ghi chu mau" || got.TenantID != "minh-an" {
		t.Fatalf("memory changed %+v %v", got, err)
	}
}

func TestPackageDoesNotExecute(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if strings.Contains(text, "os/exec") || strings.Contains(text, "syscall.Exec") || strings.Contains(text, "LookPath") {
			t.Fatal(e.Name(), "imports an executor")
		}
	}
}

func loadFixture(t *testing.T) *Export {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "minh-an.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ex, err := ReadExport(f)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func fixtureSrc(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "src")
}

func tenantCount(t *testing.T, rep Report, id string) TenantCount {
	t.Helper()
	for _, c := range rep.Tenants {
		if c.TenantID == id {
			return c
		}
	}
	t.Fatalf("missing tenant %s in %+v", id, rep.Tenants)
	return TenantCount{}
}

func assertRename(t *testing.T, rep Report, id, old, newName string, apply bool) {
	t.Helper()
	for _, r := range rep.Renames {
		if r.SourceID == id {
			if r.Old != old || r.New != newName || r.Apply != apply {
				t.Fatalf("rename %+v want old %s new %s apply %v", r, old, newName, apply)
			}
			return
		}
	}
	t.Fatal("missing rename", id)
}

func assertGrant(t *testing.T, rep Report, id, kind, old, newName string) {
	t.Helper()
	g := findGrant(rep, id)
	if g.Kind != kind || g.Old != old || g.New != newName || g.Reason != "" {
		t.Fatalf("grant %+v", g)
	}
}

func findGrant(rep Report, id string) GrantRename {
	for _, g := range rep.Grants {
		if g.GrantID == id {
			return g
		}
	}
	return GrantRename{}
}

func readSkill(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func mustReport(t *testing.T, rep Report) string {
	t.Helper()
	raw, err := FormatReport(rep)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertNoCanary(t *testing.T, text string) {
	t.Helper()
	for _, part := range strings.Fields(canary) {
		if strings.Contains(text, part) {
			t.Fatalf("output contains %s", part)
		}
	}
}

func dirSnapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(rel)
		b.WriteByte('\n')
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.Write(raw)
			b.WriteByte('\n')
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func readTree(t *testing.T, root string) string {
	t.Helper()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return ""
	}
	return dirSnapshot(t, root)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, raw, 0o644)
	})
}

type bufLog struct{ b *bytes.Buffer }

func (l bufLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.b, format+"\n", args...)
}

func TestReportJSON(t *testing.T) {
	var rep Report
	if err := json.Unmarshal([]byte(mustReport(t, rep)), &rep); err != nil {
		t.Fatal(err)
	}
}
