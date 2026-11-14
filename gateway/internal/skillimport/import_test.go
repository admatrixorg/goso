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
	rep, err := callImport(t, ex, Options{
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

	if _, err := os.Stat(filepath.Join(fixtureSrc(t), "skills", "bao-gia", "EXECUTED-SKILL")); !os.IsNotExist(err) {
		t.Fatal("skill script executed")
	}
	ents, err := os.ReadDir(skillsDir)
	if err != nil || len(ents) != 0 {
		t.Fatalf("dry-run wrote %#v", ents)
	}
	assertNoCanary(t, logBuf.String()+mustReport(t, rep))
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
	rep, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: mapPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if readSkill(t, skillsDir, "san-co") != "KEEP\n" {
		t.Fatal("pre-existing skill was overwritten")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "san-co-2")); !os.IsNotExist(err) {
		t.Fatal("existing folder was given a second skill")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "khac-don")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote the other tenant")
	}
	minh := tenantCount(t, rep, "minh-an")
	if minh.Reasons["native-exists"] != 1 || minh.Inserted != 5 {
		t.Fatalf("native-exists %+v", minh)
	}
	note, err := os.ReadFile(filepath.Join(skillsDir, "notes.txt"))
	if err != nil || string(note) != "keep-note" {
		t.Fatal("notes.txt changed")
	}
}

func TestNativeExistsDoesNotUpdate(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "bao-gia-minh-an"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "bao-gia-minh-an", "SKILL.md"), []byte("KEEP-NATIVE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sumBytes([]byte("KEEP-NATIVE\n"))
	if err := saveSourceMap(skillsDir, sourceMap{Version: mapVersion, Entries: []sourceEntry{{
		TenantID: "minh-an", SourceID: "minh-an-skill-bao-gia", Old: "Bao_Gia_Minh_An", Name: "bao-gia-minh-an", Sum: sum,
	}}}); err != nil {
		t.Fatal(err)
	}
	rep, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tenantCount(t, rep, "minh-an").Reasons["native-exists"] != 1 {
		t.Fatalf("reasons %+v", tenantCount(t, rep, "minh-an"))
	}
	if readSkill(t, skillsDir, "bao-gia-minh-an") != "KEEP-NATIVE\n" {
		t.Fatal("differing skill was updated")
	}
}

func TestRollbackKeepsNativeAndOtherTenant(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	for _, name := range []string{"keep-native", "minh-skill", "khac-skill"} {
		if err := os.MkdirAll(filepath.Join(skillsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillsDir, name, "SKILL.md"), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := saveSourceMap(skillsDir, sourceMap{Version: mapVersion, Entries: []sourceEntry{
		{TenantID: "minh-an", SourceID: "s-minh", Old: "minh-skill", Name: "minh-skill", Sum: "a"},
		{TenantID: "khac", SourceID: "s-khac", Old: "khac-skill", Name: "khac-skill", Sum: "b"},
	}}); err != nil {
		t.Fatal(err)
	}
	deny := writeDeny(t, "not-this-tenant")
	dry, err := Rollback(Options{Tenant: "minh-an", SkillsDir: skillsDir, DenyFile: deny})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.Matched != 1 || dry.Removed != 0 {
		t.Fatalf("dry %+v", dry)
	}
	if readSkill(t, skillsDir, "minh-skill") != "minh-skill\n" {
		t.Fatal("dry-run rollback removed a file")
	}
	got, err := Rollback(Options{Tenant: "minh-an", SkillsDir: skillsDir, DenyFile: deny, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Removed != 1 {
		t.Fatalf("removed %+v", got)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "minh-skill")); !os.IsNotExist(err) {
		t.Fatal("mapped folder still present")
	}
	if readSkill(t, skillsDir, "keep-native") != "keep-native\n" || readSkill(t, skillsDir, "khac-skill") != "khac-skill\n" {
		t.Fatal("rollback removed native or other-tenant data")
	}
	sm, err := loadSourceMap(skillsDir)
	if err != nil || len(sm.Entries) != 1 || sm.Entries[0].TenantID != "khac" {
		t.Fatalf("map %+v %v", sm, err)
	}
}

func TestApplyWritesNothing(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	_, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"), Apply: true,
	})
	if err == nil || !strings.Contains(err.Error(), "apply refused") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(skillsDir); !os.IsNotExist(statErr) {
		t.Fatal("apply created the skills dir")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "map.json")); !os.IsNotExist(statErr) {
		t.Fatal("apply wrote the rename map")
	}
}

func TestDenyFile(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	opt := func(deny string) Options {
		return Options{
			Tenant: "minh-an", SourceTenant: "minh-an", DenyFile: deny,
			SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
			RenameMapPath: filepath.Join(dir, "map.json"),
		}
	}
	assertZero := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected an error")
		}
		if _, statErr := os.Stat(skillsDir); !os.IsNotExist(statErr) {
			t.Fatal("wrote the skills dir")
		}
		if _, statErr := os.Stat(filepath.Join(dir, "map.json")); !os.IsNotExist(statErr) {
			t.Fatal("wrote the rename map")
		}
	}
	_, err := Import(ex, opt(filepath.Join(dir, "missing.txt")))
	assertZero(err)
	if !strings.Contains(err.Error(), "missing") {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("\n# comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Import(ex, opt(empty))
	assertZero(err)
	if !strings.Contains(err.Error(), "empty") {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("not a tenant!!!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Import(ex, opt(bad))
	assertZero(err)
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatal(err)
	}
	listed := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(listed, []byte("fake-co\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(ex, Options{
		Tenant: "fake-co", SourceTenant: "minh-an", DenyFile: listed,
		SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"),
	})
	assertZero(err)
	if rep.DeniedTenants != 1 || strings.Contains(mustReport(t, rep), "fake-co") || strings.Contains(err.Error(), "fake-co") {
		t.Fatalf("denied report leaked a name: %s %v", mustReport(t, rep), err)
	}
	rep, err = Import(ex, Options{
		Tenant: "minh-an", SourceTenant: "fake-co", DenyFile: listed,
		SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"),
	})
	assertZero(err)
	if rep.DeniedTenants != 1 || strings.Contains(mustReport(t, rep), "fake-co") {
		t.Fatalf("source deny %+v %s", rep, mustReport(t, rep))
	}
}

func TestDeniedExportTenantIsCountedNotNamed(t *testing.T) {
	ex := loadFixture(t)
	ex.Skills = append(ex.Skills, SkillRow{
		ID: "blocked-skill", TenantID: "blocked-tenant", Slug: "blocked-skill",
		Status: "active", FilePath: "skills/bao-gia/SKILL.md",
	})
	ex.Versions = append(ex.Versions, VersionRow{
		ID: "blocked-ver", TenantID: "blocked-tenant", SkillID: "blocked-skill", Version: 1,
	})
	ex.AgentGrants = append(ex.AgentGrants, AgentGrantRow{
		ID: "blocked-grant", TenantID: "blocked-tenant", SkillID: "blocked-skill",
	})
	ex.UserGrants = append(ex.UserGrants, UserGrantRow{
		ID: "blocked-user", TenantID: "blocked-tenant", SkillID: "blocked-skill", UserID: "blocked-user",
	})
	ex.TenantConfigs = append(ex.TenantConfigs, TenantConfigRow{
		SkillID: "blocked-skill", TenantID: "blocked-tenant",
	})
	dir := t.TempDir()
	var logBuf bytes.Buffer
	rep, err := Import(ex, Options{
		Tenant: "minh-an", SourceTenant: "minh-an", DenyFile: writeDeny(t, "blocked-tenant"),
		SkillsDir: filepath.Join(dir, "skills"), SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"), Log: bufLog{&logBuf},
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatal(err)
	}
	if rep.DeniedTenants < 1 {
		t.Fatalf("denied count %d", rep.DeniedTenants)
	}
	blob := mustReport(t, rep) + "\n" + logBuf.String() + "\n" + err.Error()
	if strings.Contains(blob, "blocked-tenant") {
		t.Fatalf("denied tenant id leaked: %s", blob)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "map.json")); !os.IsNotExist(statErr) {
		t.Fatal("denied export wrote the rename map")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "skills")); !os.IsNotExist(statErr) {
		t.Fatal("denied export wrote skills")
	}
}

func TestMaskPhone(t *testing.T) {
	if strings.Contains(MaskIdentifiers("call 0901234567"), "0901234567") {
		t.Fatal("phone leaked")
	}
	ex := &Export{Skills: []SkillRow{{
		ID: "s", TenantID: "minh-an", Slug: "0901234567", Status: "active",
		FilePath: "skills/bao-gia/SKILL.md",
	}}, UserGrants: []UserGrantRow{{
		ID: "g", TenantID: "minh-an", SkillID: "s", UserID: "0901234567", GrantedBy: "0901234567",
	}}}
	dir := t.TempDir()
	rep, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: filepath.Join(dir, "skills"), SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	out := mustReport(t, rep)
	raw, err := os.ReadFile(filepath.Join(dir, "map.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out+string(raw), "0901234567") {
		t.Fatal("report printed a phone")
	}
}

func TestRenameMapMismatchWritesNothing(t *testing.T) {
	ex := loadFixture(t)
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	_, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "rename.json"), Apply: true,
	})
	if err == nil || !strings.Contains(err.Error(), "apply refused") {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(skillsDir); !os.IsNotExist(statErr) {
		t.Fatal("apply created the skills dir")
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
	body, reason, err := readSkillBytes(src, "skills/crlf/SKILL.md")
	if err != nil || reason != "" || string(body) != "dong\nmot\n" {
		t.Fatalf("crlf %q %s %v", body, reason, err)
	}
	rep, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: src,
		RenameMapPath: filepath.Join(dir, "map.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	c := tenantCount(t, rep, "minh-an")
	if c.Inserted != 1 || c.Reasons["too-large"] != 1 || c.Reasons["binary-skipped"] != 1 || c.Reasons["symlink"] != 1 || c.Reasons["path-escape"] != 1 || c.Reasons["duplicate"] != 1 {
		t.Fatalf("edges %+v", c)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "xuong-dong")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote a skill")
	}
	if strings.Contains(readTree(t, skillsDir), "SYMLINK-TARGET") {
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
	rep, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: skillsDir, SrcDir: filepath.Join(dir, "src"),
		RenameMapPath: filepath.Join(dir, "map.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tenantCount(t, rep, "minh-an").Inserted != 1 {
		t.Fatalf("jsonl import %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "mot")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote a skill")
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
	if _, err := callImport(t, ex, Options{
		Tenant: "minh-an", SkillsDir: filepath.Join(dir, "skills"), SrcDir: fixtureSrc(t),
		RenameMapPath: filepath.Join(dir, "map.json"),
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

func callImport(t *testing.T, ex *Export, opt Options) (Report, error) {
	t.Helper()
	if opt.SourceTenant == "" {
		opt.SourceTenant = opt.Tenant
	}
	if opt.DenyFile == "" {
		opt.DenyFile = writeDeny(t, "not-this-tenant")
	}
	return Import(ex, opt)
}

func writeDeny(t *testing.T, id string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deny.txt")
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
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
