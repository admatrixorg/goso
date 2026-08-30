// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package impexp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCatalogExportImportRoundTrip(t *testing.T) {
	st := store.New()
	svc := New(st)
	ag, err := st.CreateAgent(store.Agent{AgentKey: "bot", DisplayName: "Bot", Instructions: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	tm, err := st.CreateTeam(store.Team{Name: "Ops", LeadAgentID: ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnector(store.ConnectorRecord{Name: "crm", Transport: "http", Endpoint: "http://127.0.0.1:9", CredentialRef: "secret:connector/crm/token"}); err != nil {
		t.Fatal(err)
	}

	cat := svc.Catalog("")
	if len(cat.Agents) != 1 || len(cat.Teams) != 1 || len(cat.MCP) != 1 {
		t.Fatalf("catalog %+v", cat)
	}
	if cat.MCP[0].TokenSet != true {
		t.Fatal("token_set expected from credential_ref")
	}
	b, _ := json.Marshal(cat)
	if strings.Contains(string(b), "sk-") || strings.Contains(strings.ToLower(string(b)), `"token":`) {
		t.Fatalf("catalog leaked: %s", b)
	}

	job, err := svc.Export("", Selection{TeamIDs: []string{tm.ID}, AgentIDs: []string{ag.ID}, MCPNames: []string{"crm"}})
	if err != nil {
		t.Fatal(err)
	}
	if job.Archive == nil || job.Archive.IncludeSecrets {
		t.Fatal("export archive")
	}
	ab, _ := json.Marshal(job.Archive)
	if strings.Contains(string(ab), "secret:connector") || strings.Contains(strings.ToLower(string(ab)), `"token":`) {
		t.Fatalf("export leaked: %s", ab)
	}
	if ContainsSecrets(job.Archive) {
		t.Fatal("export secrets")
	}

	dst := store.New()
	other := New(dst)
	raw, _ := json.Marshal(job.Archive)
	prev, err := other.Preview("", raw)
	if err != nil || !prev.Valid {
		t.Fatalf("preview %v %+v", err, prev)
	}

	imp, err := other.Import("", raw, ConflictSkip, true)
	if err != nil || !imp.DryRun {
		t.Fatalf("dry %v %+v", err, imp)
	}
	if len(dst.ListAgents()) != 0 {
		t.Fatal("dry run wrote agents")
	}

	imp, err = other.Import("", raw, ConflictSkip, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(dst.ListAgents()) != 1 || len(dst.ListTeams()) != 1 || len(dst.ListConnectors()) != 1 {
		t.Fatalf("import counts a=%d t=%d m=%d", len(dst.ListAgents()), len(dst.ListTeams()), len(dst.ListConnectors()))
	}
	if len(imp.Report.CredentialsNeeded) == 0 {
		t.Fatal("mcp credentials_needed")
	}
	con := dst.ListConnectors()[0]
	if con.CredentialRef != "" && strings.HasPrefix(con.CredentialRef, "secret:") {
		t.Fatalf("imported credential_ref %q", con.CredentialRef)
	}

	rb, err := other.Rollback(imp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Status != StatusDone {
		t.Fatalf("rollback %s", rb.Status)
	}
	if len(dst.ListAgents()) != 0 || len(dst.ListTeams()) != 0 {
		t.Fatalf("rollback leftover a=%d t=%d", len(dst.ListAgents()), len(dst.ListTeams()))
	}
}

func TestConflictSkipOverwriteRename(t *testing.T) {
	st := store.New()
	svc := New(st)
	if _, err := st.CreateAgent(store.Agent{AgentKey: "bot", DisplayName: "Old", Instructions: "keep"}); err != nil {
		t.Fatal(err)
	}
	arch := mustJSON(t, Archive{
		Schema: Schema, SchemaVersion: SchemaVersion,
		Agents: []AgentItem{{AgentKey: "bot", DisplayName: "New", Instructions: "next"}},
	})

	skip, err := svc.Import("", arch, ConflictSkip, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(skip.Report.Skipped) != 1 {
		t.Fatalf("skip %+v", skip.Report)
	}
	got, _ := st.ListAgents()[0], 0
	if got.Instructions != "keep" {
		t.Fatalf("skip mutated %q", got.Instructions)
	}

	over, err := svc.Import("", arch, ConflictOverwrite, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(over.Report.Overwritten) != 1 {
		t.Fatalf("over %+v", over.Report)
	}
	got = st.ListAgents()[0]
	if got.Instructions != "next" {
		t.Fatalf("overwrite %q", got.Instructions)
	}

	ren, err := svc.Import("", arch, ConflictRename, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ren.Report.Renamed) != 1 {
		t.Fatalf("rename %+v", ren.Report)
	}
	if len(st.ListAgents()) != 2 {
		t.Fatalf("rename count %d", len(st.ListAgents()))
	}
}

func TestExportRejectsEmptyAndSmuggledToken(t *testing.T) {
	st := store.New()
	svc := New(st)
	if _, err := svc.Export("", Selection{}); err != ErrEmptySelection {
		t.Fatalf("empty %v", err)
	}
	dir := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", dir)
	if err := os.MkdirAll(filepath.Join(dir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo", "SKILL.md"), []byte("use sk-live-fixture-not-vendor-zzzz carefully"), 0o644); err != nil {
		t.Fatal(err)
	}
	job, err := svc.Export("", Selection{SkillNames: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	body := job.Archive.Skills[0].Body
	if strings.Contains(body, "sk-live") {
		t.Fatalf("skill body leaked %q", body)
	}
}

func TestSecretsNeverLeaveBox(t *testing.T) {
	st := store.New()
	t.Setenv("GOSO_MASTER_KEY", strings.Repeat("ab", 32)[:64])
	if err := secrets.Put(st, "connector/crm/token", []byte("sk-live-fixture-not-vendor-zzzz")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnector(store.ConnectorRecord{Name: "crm", Transport: "http", CredentialRef: "secret:connector/crm/token"}); err != nil {
		t.Fatal(err)
	}
	svc := New(st)
	job, err := svc.Export("", Selection{MCPNames: []string{"crm"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(job)
	if strings.Contains(string(b), "sk-live-fixture-not-vendor-zzzz") {
		t.Fatalf("plaintext token in job: %s", b)
	}
	if !job.Archive.MCP[0].TokenSet {
		t.Fatal("token_set")
	}
}

func TestCatalogTenantIsolation(t *testing.T) {
	st := store.New()
	svc := New(st)
	if _, err := st.CreateAgent(store.Agent{TenantID: "alpha", AgentKey: "a1", DisplayName: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{TenantID: "beta", AgentKey: "b1", DisplayName: "B"}); err != nil {
		t.Fatal(err)
	}
	cat := svc.Catalog("alpha")
	if len(cat.Agents) != 1 || cat.Agents[0].AgentKey != "a1" {
		t.Fatalf("alpha catalog %+v", cat.Agents)
	}
	job, err := svc.Export("beta", Selection{AgentIDs: []string{"a1", "b1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Archive.Agents) != 1 || job.Archive.Agents[0].AgentKey != "b1" {
		t.Fatalf("beta export %+v", job.Archive.Agents)
	}
}

func TestImportForwardAgentLinks(t *testing.T) {
	st := store.New()
	svc := New(st)
	arch := mustJSON(t, Archive{
		Schema: Schema, SchemaVersion: SchemaVersion,
		Agents: []AgentItem{
			{AgentKey: "lead", DisplayName: "Lead", Links: []string{"help"}},
			{AgentKey: "help", DisplayName: "Help"},
		},
	})
	imp, err := svc.Import("", arch, ConflictSkip, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Report.Created) != 2 {
		t.Fatalf("created %+v", imp.Report)
	}
	var lead *store.Agent
	for _, a := range st.ListAgents() {
		if a.AgentKey == "lead" {
			lead = a
		}
	}
	if lead == nil {
		t.Fatal("lead")
	}
	links, err := st.ListAgentLinks(lead.ID)
	if err != nil || len(links) != 1 {
		t.Fatalf("links %v %+v", err, links)
	}
}

func TestImportRejectsMissingSchema(t *testing.T) {
	st := store.New()
	svc := New(st)
	_, err := svc.Import("", []byte(`{"agents":[{"agent_key":"bot","display_name":"Bot"}]}`), ConflictSkip, true)
	if err != ErrSchema {
		t.Fatalf("want schema err got %v", err)
	}
}

func mustJSON(t *testing.T, a Archive) []byte {
	t.Helper()
	StripArchive(&a)
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
