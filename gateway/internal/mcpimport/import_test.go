// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestExportDropsSecrets(t *testing.T) {
	ex := readFixture(t)
	s := fmt.Sprintf("%#v", ex)
	if strings.Contains(s, "placeholder-minh-an") {
		t.Fatal("decoded export retained a placeholder secret")
	}
}

func TestDryRunDoesNotWrite(t *testing.T) {
	ex := readFixture(t)
	base := store.New()
	if _, err := base.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales", DisplayName: "sales"}); err != nil {
		t.Fatal(err)
	}
	cs := &countStore{Store: base}
	rep, err := Import(cs, ex, Options{Tenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || cs.creates != 0 || cs.links != 0 {
		t.Fatalf("dry-run wrote creates=%d links=%d dry=%v", cs.creates, cs.links, rep.DryRun)
	}
	if n := len(base.ListConnectors()); n != 0 {
		t.Fatalf("connectors %d", n)
	}
}

func TestMinhAnMemory(t *testing.T) {
	runMinhAn(t, store.New(), "")
}

func TestMinhAnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goso.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	runMinhAn(t, st, path)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	assertFileHasNoSecret(t, path)
}

func TestNameTakenDoesNotOverwrite(t *testing.T) {
	st := store.New()
	kept := store.ConnectorRecord{
		Name: "goclaw-minh-an-notes", Transport: "http", Endpoint: "http://127.0.0.1:9/keep",
		CredentialRef: "env-keep", SchemaVersion: "1.0", Enabled: true,
	}
	if _, err := st.CreateConnector(kept); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales"}); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, readFixture(t), Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	minh := mustTenant(t, rep, "minh-an")
	if minh.MCPServers.Inserted != 2 || minh.MCPServers.Rejected != 1 || reasonCount(minh.MCPServers, "name-taken") != 1 {
		t.Fatalf("servers %+v", minh.MCPServers)
	}
	if minh.MCPAgentGrants.Inserted != 0 || reasonCount(minh.MCPAgentGrants, "server-not-owned") != 3 {
		t.Fatalf("grants %+v", minh.MCPAgentGrants)
	}
	got, err := st.GetConnector("goclaw-minh-an-notes")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Endpoint != kept.Endpoint || got.CredentialRef != kept.CredentialRef || got.SchemaVersion != kept.SchemaVersion {
		t.Fatalf("pre-existing connector changed: %+v", got)
	}
	agents := st.ListAgents()
	if len(agents) != 1 {
		t.Fatal(len(agents))
	}
	names, err := st.ListAgentConnectors(agents[0].ID)
	if err != nil || len(names) != 0 {
		t.Fatalf("links %v %v", names, err)
	}
	if findConn(st.ListConnectors(), "goclaw-minh-an-files") == nil || findConn(st.ListConnectors(), "goclaw-minh-an-search") == nil {
		t.Fatal("other servers were not inserted")
	}
	if findConn(st.ListConnectors(), "goclaw-other-shop-ledger") != nil {
		t.Fatal("foreign server inserted")
	}
}

func TestSourceIDRenameDoesNotDuplicate(t *testing.T) {
	st := store.New()
	seedAgents(t, st)
	ex := readFixture(t)
	if _, err := Import(st, ex, Options{Tenant: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	before := findConn(st.ListConnectors(), "goclaw-minh-an-notes")
	if before == nil {
		t.Fatal("missing notes")
	}
	for i := range ex.servers {
		if ex.servers[i].ID == "00000000-0000-4000-8000-0000000000a1" {
			ex.servers[i].Name = "notes-renamed"
		}
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	minh := mustTenant(t, rep, "minh-an")
	if minh.MCPServers.Inserted != 0 || reasonCount(minh.MCPServers, "unchanged") != 3 {
		t.Fatalf("servers %+v", minh.MCPServers)
	}
	if findConn(st.ListConnectors(), "goclaw-minh-an-notes-renamed") != nil {
		t.Fatal("rename inserted a second connector")
	}
	after := findConn(st.ListConnectors(), "goclaw-minh-an-notes")
	if after == nil || after.Endpoint != before.Endpoint || after.Enabled != before.Enabled || !after.CreatedAt.Equal(before.CreatedAt) {
		t.Fatalf("stored notes changed")
	}
	if len(rep.NeedsCredential) != 2 || rep.NeedsCredential[0].SourceName != "notes-renamed" {
		t.Fatalf("needs %+v", rep.NeedsCredential)
	}
	assertNoSecret(t, mustJSON(t, rep))
}

func TestDisabledGrantDoesNotDeleteLink(t *testing.T) {
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"mcp_servers":[{"id":"00000000-0000-4000-8000-0000000000a2","name":"files","transport":"stdio","command":"minh-an-files","tenant_id":"minh-an"}],"mcp_agent_grants":[{"server_id":"00000000-0000-4000-8000-0000000000a2","agent_id":"tu-van-sales","enabled":false,"tenant_id":"minh-an"}]}`)
	ex, err := ReadExport(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(st, ex, Options{Tenant: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	if err := st.LinkAgentConnector(ag.ID, "goclaw-minh-an-files"); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	minh := mustTenant(t, rep, "minh-an")
	if reasonCount(minh.MCPAgentGrants, "unchanged") != 1 || minh.MCPAgentGrants.Inserted != 0 {
		t.Fatalf("%+v", minh.MCPAgentGrants)
	}
	names, err := st.ListAgentConnectors(ag.ID)
	if err != nil || len(names) != 1 || names[0] != "goclaw-minh-an-files" {
		t.Fatalf("link removed: %v %v", names, err)
	}
}

func TestUserinfoStrippedAndListed(t *testing.T) {
	st := store.New()
	body := []byte(`{"mcp_servers":[{"id":"00000000-0000-4000-8000-0000000000aa","name":"notes","transport":"sse","url":"http://user:placeholder-minh-an-userinfo@127.0.0.1:9/minh-an","tenant_id":"minh-an"}]}`)
	ex, err := ReadExport(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%#v", ex), "placeholder-minh-an") {
		t.Fatal("userinfo retained on export")
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	got := findConn(st.ListConnectors(), "goclaw-minh-an-notes")
	if got == nil || got.Enabled || got.Endpoint != "http://127.0.0.1:9/minh-an" || got.CredentialRef != "" {
		t.Fatalf("%+v", got)
	}
	if len(rep.NeedsCredential) != 1 || rep.NeedsCredential[0].SourceName != "notes" {
		t.Fatalf("%+v", rep.NeedsCredential)
	}
	assertNoSecret(t, mustJSON(t, rep), got.Endpoint, got.CredentialRef, got.SchemaVersion)
}

func TestInvalidTransportWritesNothing(t *testing.T) {
	st := store.New()
	body := []byte(`{"mcp_servers":[{"id":"00000000-0000-4000-8000-0000000000ab","name":"notes","transport":"carrier-pigeon","url":"http://127.0.0.1:9/minh-an","api_key":"placeholder-minh-an-api-key","tenant_id":"minh-an"}]}`)
	ex, err := ReadExport(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	minh := mustTenant(t, rep, "minh-an")
	if minh.MCPServers.Rejected != 1 || reasonCount(minh.MCPServers, "invalid-transport") != 1 || len(rep.NeedsCredential) != 0 {
		t.Fatalf("%+v needs %d", minh.MCPServers, len(rep.NeedsCredential))
	}
	if n := len(st.ListConnectors()); n != 0 {
		t.Fatalf("wrote %d", n)
	}
	assertNoSecret(t, mustJSON(t, rep))
}

func TestSlugCollision(t *testing.T) {
	st := store.New()
	body := []byte(`{"mcp_servers":[
	  {"id":"00000000-0000-4000-8000-0000000000a1","name":"Notes","transport":"stdio","command":"minh-an-files","tenant_id":"minh-an"},
	  {"id":"00000000-0000-4000-8000-0000000000a2","name":"notes","transport":"stdio","command":"other-cmd","tenant_id":"minh-an"}
	]}`)
	ex, err := ReadExport(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	minh := mustTenant(t, rep, "minh-an")
	if minh.MCPServers.Inserted != 1 || reasonCount(minh.MCPServers, "name-taken") != 1 {
		t.Fatalf("%+v", minh.MCPServers)
	}
	got := findConn(st.ListConnectors(), "goclaw-minh-an-notes")
	if got == nil || got.Endpoint != "minh-an-files" {
		t.Fatalf("%+v", got)
	}
	if len(st.ListConnectors()) != 1 {
		t.Fatalf("count %d", len(st.ListConnectors()))
	}
}

func TestJSONL(t *testing.T) {
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales"}); err != nil {
		t.Fatal(err)
	}
	body := []byte(strings.Join([]string{
		`{"table":"mcp_servers","id":"00000000-0000-4000-8000-0000000000a2","name":"files","transport":"stdio","command":"minh-an-files","api_key":"","tenant_id":"minh-an"}`,
		`{"table":"mcp_agent_grants","server_id":"00000000-0000-4000-8000-0000000000a2","agent_id":"tu-van-sales","enabled":true,"tenant_id":"minh-an"}`,
		`{"table":"mcp_user_credentials","tenant_id":"minh-an","api_key":"placeholder-minh-an-user-cred"}`,
	}, "\n"))
	ex, err := ReadExport(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%#v", ex), "placeholder-minh-an") {
		t.Fatal("jsonl retained a secret")
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	minh := mustTenant(t, rep, "minh-an")
	if minh.MCPServers.Inserted != 1 || minh.MCPAgentGrants.Inserted != 1 || minh.CredentialTables.NotImported != 1 {
		t.Fatalf("servers %+v grants %+v creds %+v", minh.MCPServers, minh.MCPAgentGrants, minh.CredentialTables)
	}
	got := findConn(st.ListConnectors(), "goclaw-minh-an-files")
	if got == nil || got.Enabled || got.CredentialRef != "" {
		t.Fatalf("%+v", got)
	}
	assertNoSecret(t, mustJSON(t, rep))
}

func TestExportTooLarge(t *testing.T) {
	old := maxExportBytes
	maxExportBytes = 32
	t.Cleanup(func() { maxExportBytes = old })
	_, err := ReadExport(bytes.NewReader(bytes.Repeat([]byte("a"), 33)))
	if err != ErrExportTooLarge {
		t.Fatalf("%v", err)
	}
}

func TestSourceTenantRejectsEmpty(t *testing.T) {
	st := store.New()
	body := []byte(`{"mcp_servers":[{"id":"00000000-0000-4000-8000-0000000000a1","name":"notes","transport":"stdio","command":"minh-an-files","tenant_id":"","api_key":"placeholder-minh-an-api-key"}]}`)
	ex, err := ReadExport(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, ex, Options{Tenant: "minh-an", SourceTenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ListConnectors()) != 0 || len(rep.NeedsCredential) != 0 {
		t.Fatal("empty tenant imported")
	}
	var found bool
	for _, row := range rep.Tenants {
		if row.TenantID == "" && reasonCount(row.MCPServers, "foreign-tenant") == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", rep.Tenants)
	}
	assertNoSecret(t, mustJSON(t, rep))
}

type countStore struct {
	Store
	creates int
	links   int
}

func (c *countStore) CreateConnector(rec store.ConnectorRecord) (*store.ConnectorRecord, error) {
	c.creates++
	return nil, os.ErrInvalid
}

func (c *countStore) LinkAgentConnector(agentID, connectorName string) error {
	c.links++
	return os.ErrInvalid
}

func runMinhAn(t *testing.T, st store.StoreIface, dbPath string) {
	t.Helper()
	if err := st.SetToolFlag("web_search", true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnector(store.ConnectorRecord{
		Name: "zalocrm", Transport: "http", Endpoint: "http://127.0.0.1:8089",
		CredentialRef: "secret:keep-zalocrm", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	sales, other := seedAgents(t, st)
	ex := readFixture(t)

	dry, err := Import(st, ex, Options{Tenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun {
		t.Fatal("dry-run flag")
	}
	assertMinhAnCounts(t, dry, true)
	if n := len(st.ListConnectors()); n != 1 {
		t.Fatalf("dry-run connectors %d", n)
	}
	if names, err := st.ListAgentConnectors(sales.ID); err != nil || len(names) != 0 {
		t.Fatalf("dry-run links %v %v", names, err)
	}

	rep, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DryRun {
		t.Fatal("apply was dry")
	}
	assertMinhAnCounts(t, rep, true)
	assertNoSecret(t, mustJSON(t, rep))
	notes := findConn(st.ListConnectors(), "goclaw-minh-an-notes")
	files := findConn(st.ListConnectors(), "goclaw-minh-an-files")
	search := findConn(st.ListConnectors(), "goclaw-minh-an-search")
	if notes == nil || files == nil || search == nil {
		t.Fatal("missing connector")
	}
	if notes.Enabled || notes.Transport != "mcp-http" || notes.Endpoint != "http://127.0.0.1:9/minh-an-notes" || notes.CredentialRef != "" || notes.TimeoutMS != 60000 || notes.SchemaVersion != sourceMarker("minh-an", notesID) || len(notes.ManifestJSON) != 0 {
		t.Fatalf("notes %+v", notes)
	}
	if files.Enabled || files.Transport != "mcp-stdio" || files.Endpoint != "minh-an-files" || strings.Contains(files.Endpoint, "--root") || files.CredentialRef != "" || files.TimeoutMS != 30000 {
		t.Fatalf("files %+v", files)
	}
	if search.Enabled || search.Transport != "mcp-http" || search.Endpoint != "http://127.0.0.1:9/minh-an-search" || search.CredentialRef != "" || search.TimeoutMS != 15000 {
		t.Fatalf("search %+v", search)
	}
	if findConn(st.ListConnectors(), "goclaw-other-shop-ledger") != nil {
		t.Fatal("foreign connector")
	}
	crm, err := st.GetConnector("zalocrm")
	if err != nil || !crm.Enabled || crm.Endpoint != "http://127.0.0.1:8089" || crm.CredentialRef != "secret:keep-zalocrm" {
		t.Fatalf("zalocrm %+v %v", crm, err)
	}
	names, err := st.ListAgentConnectors(sales.ID)
	if err != nil || len(names) != 1 || names[0] != "goclaw-minh-an-notes" {
		t.Fatalf("sales links %v %v", names, err)
	}
	onames, err := st.ListAgentConnectors(other.ID)
	if err != nil || len(onames) != 0 {
		t.Fatalf("other links %v %v", onames, err)
	}
	if !st.GetToolFlag("web_search") || st.GetToolFlag("read_file") {
		t.Fatal("tool flags changed")
	}
	if _, ok := st.GetAgentToolFlag(sales.ID, "web_search"); ok {
		t.Fatal("agent tool flag written")
	}
	if _, err := st.GetSecret("connector/goclaw-minh-an-notes/token"); err == nil {
		t.Fatal("token secret stored")
	}
	for _, c := range st.ListConnectors() {
		assertNoSecret(t, c.Name, c.Transport, c.Endpoint, c.CredentialRef, c.SchemaVersion, c.ManifestURL, string(c.ManifestJSON))
	}
	created := notes.CreatedAt

	again, err := Import(st, ex, Options{Tenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	assertMinhAnCounts(t, again, false)
	notes2 := findConn(st.ListConnectors(), "goclaw-minh-an-notes")
	if notes2 == nil || !notes2.CreatedAt.Equal(created) || notes2.Endpoint != notes.Endpoint || notes2.Enabled {
		t.Fatal("rerun changed notes")
	}
	if len(st.ListConnectors()) != 4 {
		t.Fatalf("rerun count %d", len(st.ListConnectors()))
	}
	names, err = st.ListAgentConnectors(sales.ID)
	if err != nil || len(names) != 1 {
		t.Fatalf("rerun links %v %v", names, err)
	}
	assertNoSecret(t, mustJSON(t, again))
	if dbPath != "" {
		_ = dbPath
	}
}

func assertMinhAnCounts(t *testing.T, rep Report, first bool) {
	t.Helper()
	minh := mustTenant(t, rep, "minh-an")
	other := mustTenant(t, rep, "other-shop")
	if first {
		if minh.MCPServers.Input != 3 || minh.MCPServers.Inserted != 3 || minh.MCPServers.Skipped != 0 || minh.MCPServers.Rejected != 0 {
			t.Fatalf("servers %+v", minh.MCPServers)
		}
		if minh.MCPAgentGrants.Input != 5 || minh.MCPAgentGrants.Inserted != 1 || minh.MCPAgentGrants.Skipped != 1 || minh.MCPAgentGrants.Rejected != 2 || minh.MCPAgentGrants.NotImported != 1 {
			t.Fatalf("grants %+v", minh.MCPAgentGrants)
		}
		if reasonCount(minh.MCPAgentGrants, "grant-disabled") != 1 || reasonCount(minh.MCPAgentGrants, "agent-missing") != 1 || reasonCount(minh.MCPAgentGrants, "agent-other-tenant") != 1 || reasonCount(minh.MCPAgentGrants, "needs-decision") != 1 {
			t.Fatalf("grant reasons %+v", minh.MCPAgentGrants.Reasons)
		}
	} else {
		if minh.MCPServers.Inserted != 0 || minh.MCPServers.Skipped != 3 || reasonCount(minh.MCPServers, "unchanged") != 3 {
			t.Fatalf("rerun servers %+v", minh.MCPServers)
		}
		if minh.MCPAgentGrants.Inserted != 0 || reasonCount(minh.MCPAgentGrants, "unchanged") != 1 || reasonCount(minh.MCPAgentGrants, "grant-disabled") != 1 {
			t.Fatalf("rerun grants %+v", minh.MCPAgentGrants)
		}
	}
	if other.MCPServers.Input != 1 || other.MCPServers.Rejected != 1 || reasonCount(other.MCPServers, "foreign-tenant") != 1 || other.MCPServers.Inserted != 0 {
		t.Fatalf("other servers %+v", other.MCPServers)
	}
	if other.MCPAgentGrants.Rejected != 1 || other.MCPUserGrants.NotImported != 1 || other.BuiltinToolTenantConfigs.NotImported != 1 {
		t.Fatalf("other %+v", other)
	}
	if minh.MCPUserGrants.NotImported != 1 || minh.BuiltinToolTenantConfigs.NotImported != 1 || minh.CredentialTables.NotImported != 2 {
		t.Fatalf("held %+v %+v %+v", minh.MCPUserGrants, minh.BuiltinToolTenantConfigs, minh.CredentialTables)
	}
	if rep.BuiltinTools.Input != 1 || rep.BuiltinTools.NotImported != 1 {
		t.Fatalf("builtin %+v", rep.BuiltinTools)
	}
	if len(rep.NeedsCredential) != 2 || rep.NeedsCredential[0].SourceName != "notes" || rep.NeedsCredential[1].SourceName != "search" {
		t.Fatalf("needs %+v", rep.NeedsCredential)
	}
	for _, need := range rep.NeedsCredential {
		if need.Reason != "needs-credential" || need.TenantID != "minh-an" || need.GosoName == "" {
			t.Fatalf("need %+v", need)
		}
	}
	if decisionCount(rep, "mcp_user_credentials", "") != 1 || decisionCount(rep, "mcp_context_credentials", "") != 1 {
		t.Fatalf("cred decisions %+v", rep.NotImported)
	}
	if decisionCount(rep, "mcp_user_grants", "") != 2 || decisionCount(rep, "builtin_tools", "") != 1 || decisionCount(rep, "builtin_tool_tenant_configs", "") != 2 {
		t.Fatalf("decisions %+v", rep.NotImported)
	}
	if decisionCount(rep, "mcp_servers", "api_key") != 2 || decisionCount(rep, "mcp_servers", "headers") != 1 || decisionCount(rep, "mcp_servers", "env") != 1 {
		t.Fatalf("columns %+v", rep.NotImported)
	}
	api := decision(rep, "mcp_servers", "api_key")
	if api.Reason != "dropped-secret" {
		t.Fatalf("api_key decision %+v", api)
	}
	headers := decision(rep, "mcp_servers", "headers")
	if headers.Reason != "needs-decision" {
		t.Fatalf("headers decision %+v", headers)
	}
}

func seedAgents(t *testing.T, st store.StoreIface) (*store.Agent, *store.Agent) {
	t.Helper()
	sales, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales", DisplayName: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAgent(store.Agent{TenantID: "other-shop", AgentKey: "other-shop-ke-toan", DisplayName: "ke toan"})
	if err != nil {
		t.Fatal(err)
	}
	return sales, other
}

const notesID = "00000000-0000-4000-8000-0000000000a1"

func readFixture(t *testing.T) *Export {
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

func mustTenant(t *testing.T, rep Report, id string) TenantReport {
	t.Helper()
	for _, row := range rep.Tenants {
		if row.TenantID == id {
			return row
		}
	}
	t.Fatalf("missing tenant %s in %+v", id, rep.Tenants)
	return TenantReport{}
}

func reasonCount(c Counts, code string) int {
	for _, r := range c.Reasons {
		if r.Code == code {
			return r.Count
		}
	}
	return 0
}

func decisionCount(rep Report, table, column string) int {
	return decision(rep, table, column).Count
}

func decision(rep Report, table, column string) Decision {
	for _, d := range rep.NotImported {
		if d.Table == table && d.Column == column {
			return d
		}
	}
	return Decision{}
}

func findConn(list []*store.ConnectorRecord, name string) *store.ConnectorRecord {
	for _, c := range list {
		if c != nil && c.Name == name {
			return c
		}
	}
	return nil
}

func mustJSON(t *testing.T, rep Report) string {
	t.Helper()
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNoSecret(t *testing.T, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if strings.Contains(part, "placeholder-minh-an") {
			t.Fatal("output contains a placeholder secret")
		}
	}
}

func assertFileHasNoSecret(t *testing.T, path string) {
	t.Helper()
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("placeholder-minh-an")) {
			t.Fatalf("database file %s contains a placeholder secret", filepath.Base(p))
		}
	}
}
