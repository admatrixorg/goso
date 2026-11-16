// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agentimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const goclawSalesID = "019d568d-0000-7000-8000-000000000001"

// importRenamedAgent imports a GoClaw agent whose key clashes with another
// tenant's agent, so the agent importer renames it and records the mapping.
func importRenamedAgent(t *testing.T, st store.StoreIface) *store.Agent {
	t.Helper()
	if _, err := st.CreateAgent(store.Agent{AgentKey: "tu-van", DisplayName: "Other", TenantID: "other-co"}); err != nil {
		t.Fatal(err)
	}
	ex, err := agentimport.ReadExport(strings.NewReader(`{"agents":[{"id":"` + goclawSalesID +
		`","tenant_id":"minh-an","agent_key":"tu-van","display_name":"Tu van","status":"active"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := agentimport.Import(st, ex, agentimport.Options{SourceTenant: "minh-an", DestTenant: "minh-an", Apply: true, Deny: testDeny()})
	if err != nil || len(rep.RenamedKeys) != 1 {
		t.Fatalf("agent import %+v %v", rep, err)
	}
	for _, a := range st.ListAgents() {
		if a.AgentKey == rep.RenamedKeys[0].DestKey {
			return a
		}
	}
	t.Fatal("renamed agent missing")
	return nil
}

func tgRow() ChannelRow {
	off := false
	return ChannelRow{ID: "ch-tg-1", TenantID: "minh-an", Name: "shop-bot", ChannelType: "telegram", AgentID: goclawSalesID, Enabled: &off}
}

func TestChannelBindsRenamedAgentThroughImportMap(t *testing.T) {
	st := store.New()
	agent := importRenamedAgent(t, st)
	rep, err := Import(st, &Export{ChannelInstances: []ChannelRow{tgRow()}}, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/shop-bot")
	if err != nil || cfg.AgentID != agent.ID || cfg.Enabled {
		t.Fatalf("channel %+v %v report %+v", cfg, err, rep)
	}
}

// Rows imported before the fix have no agent. A rerun fills it in, dry-run
// first, and leaves the channel off.
func TestRerunRebindsChannelImportedWithoutAgent(t *testing.T) {
	st := store.New()
	// First run before the agent exists: the channel lands with no agent.
	if _, err := Import(st, &Export{ChannelInstances: []ChannelRow{tgRow()}}, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := st.GetChannelConfig("minh-an/telegram/shop-bot"); cfg == nil || cfg.AgentID != "" {
		t.Fatalf("setup %+v", cfg)
	}
	agent := importRenamedAgent(t, st)
	ex := &Export{ChannelInstances: []ChannelRow{tgRow()}}

	dry, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny(), DryRun: true})
	if err != nil || dry.Tenants[0].ChannelInstances.Rebound != 1 {
		t.Fatalf("dry %+v %v", dry, err)
	}
	if cfg, _ := st.GetChannelConfig("minh-an/telegram/shop-bot"); cfg.AgentID != "" {
		t.Fatal("dry-run wrote")
	}
	got, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil || got.Tenants[0].ChannelInstances.Rebound != 1 {
		t.Fatalf("apply %+v %v", got, err)
	}
	cfg, _ := st.GetChannelConfig("minh-an/telegram/shop-bot")
	if cfg.AgentID != agent.ID || cfg.Enabled {
		t.Fatalf("after rebind %+v", cfg)
	}
	again, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil || again.Tenants[0].ChannelInstances.Rebound != 0 || again.Tenants[0].ChannelInstances.Skipped != 1 {
		t.Fatalf("third run %+v %v", again, err)
	}
}

// A channel a person already changed is not touched by a rerun.
func TestRerunKeepsEditedChannel(t *testing.T) {
	st := store.New()
	if _, err := Import(st, &Export{ChannelInstances: []ChannelRow{tgRow()}}, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := st.GetChannelConfig("minh-an/telegram/shop-bot")
	cfg.DMPolicy = "allowlist"
	if err := st.PutChannelConfig(*cfg); err != nil {
		t.Fatal(err)
	}
	importRenamedAgent(t, st)
	rep, err := Import(st, &Export{ChannelInstances: []ChannelRow{tgRow()}}, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil || rep.Tenants[0].ChannelInstances.Rebound != 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	if got, _ := st.GetChannelConfig("minh-an/telegram/shop-bot"); got.AgentID != "" || got.DMPolicy != "allowlist" {
		t.Fatalf("edited row changed %+v", got)
	}
}

func zcrmRow(config string) ChannelRow {
	off := false
	return ChannelRow{ID: "ch-zp-1", TenantID: "minh-an", Name: "zcrm-shop", ChannelType: "zalo_personal", Config: []byte(config), Enabled: &off}
}

// A ZaloCRM bridge channel keeps the account it answers for and where to
// reply. Rows imported before settings were kept get them on a rerun.
func TestZCRMChannelKeepsBridgeSettings(t *testing.T) {
	st := store.New()
	opts := Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()}
	if _, err := Import(st, &Export{ChannelInstances: []ChannelRow{zcrmRow(`{}`)}}, opts); err != nil {
		t.Fatal(err)
	}
	const name = "minh-an/zalo-personal/zcrm-shop"
	if cfg, err := st.GetChannelConfig(name); err != nil || len(cfg.Settings) != 0 {
		t.Fatalf("setup %+v %v", cfg, err)
	}
	ex := &Export{ChannelInstances: []ChannelRow{zcrmRow(`{"account_id":"acc-1","crm_base_url":"https://crm.example.test","group_enabled":true}`)}}
	got, err := Import(st, ex, opts)
	if err != nil || got.Tenants[0].ChannelInstances.Rebound != 1 {
		t.Fatalf("rerun %+v %v", got, err)
	}
	cfg, _ := st.GetChannelConfig(name)
	want := map[string]string{"account_id": "acc-1", "crm_base_url": "https://crm.example.test", "group_enabled": "true"}
	for k, v := range want {
		if cfg.Settings[k] != v {
			t.Fatalf("settings %v want %v", cfg.Settings, want)
		}
	}
	if cfg.Enabled {
		t.Fatal("imported channel turned on")
	}
	again, err := Import(st, ex, opts)
	if err != nil || again.Tenants[0].ChannelInstances.Rebound != 0 || again.Tenants[0].ChannelInstances.Skipped != 1 {
		t.Fatalf("third run %+v %v", again, err)
	}
}
