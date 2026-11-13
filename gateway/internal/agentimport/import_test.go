// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	srcTenant  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	otherSrc   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"
	destTenant = "minh-an"
	otherDest  = "minh-an-b"

	secretMarker = "SECRETMARKERdonotimport"
	embedMarker  = "EMBEDMARKERvector"
	modelMarker  = "SHOULD-NOT-BECOME-MODEL"
	descMarker   = "MO-TA-KHONG-LUU"
	emojiMarker  = "EMOJI-KHONG-LUU"
	memoryMarker = "MEMORYMARKERuntouched"
	keepText     = "KEEPINSTRUCTIONS"
)

func testDeny() map[string]struct{} {
	return map[string]struct{}{"blocked-sample": {}}
}

func rbOpt(apply bool) RollbackOptions {
	return RollbackOptions{Apply: apply, Deny: testDeny()}
}

func TestMain(m *testing.M) {
	_ = os.Setenv("GOSO_LITE", "")
	_ = os.Setenv("GOSO_DATABASE_URL", "")
	os.Exit(m.Run())
}

type backend struct {
	name string
	st   store.StoreIface
}

func openBackends(t *testing.T) []backend {
	t.Helper()
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	out := []backend{{name: "memory", st: store.New()}, {name: "sqlite", st: sq}}
	if pg := openPostgres(t); pg != nil {
		out = append(out, backend{name: "postgres", st: pg})
	}
	if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		seen := false
		for _, b := range out {
			if b.name == "postgres" {
				seen = true
			}
		}
		if !seen {
			t.Fatal("CI requires GOSO_TEST_POSTGRES_DSN")
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
		return nil
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	schema := "goso_ai_" + hex.EncodeToString(raw[:])
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()
	t.Cleanup(func() {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return
		}
		defer db.Close()
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

func loadFixture(t *testing.T, name string) *Export {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
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

func TestMinhAnFixture(t *testing.T) {
	ex := loadFixture(t, "minh-an.json")
	for _, b := range openBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			runMinhAn(t, b.st, ex)
		})
	}
}

func runMinhAn(t *testing.T, st store.StoreIface, ex *Export) {
	t.Helper()
	kept, err := st.CreateAgent(store.Agent{
		TenantID: destTenant, AgentKey: "minh-an-kept", DisplayName: "Giu Minh An", Instructions: keepText,
	})
	if err != nil {
		t.Fatal(err)
	}
	kept2, err := st.CreateAgent(store.Agent{
		TenantID: destTenant, AgentKey: "minh-an-kept-2", DisplayName: "Giu Minh An 2", Instructions: keepText,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddAgentLink(kept.ID, kept2.ID); err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: kept.ID, TenantID: destTenant, Label: "giu"})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := st.PutMemory(store.Memory{SessionID: sess.ID, TenantID: destTenant, Kind: store.KindEpisodic, Body: memoryMarker})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAgent(store.Agent{
		TenantID: otherDest, AgentKey: "minh-an-b-seed", DisplayName: "Seed Minh An B", Instructions: keepText,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTeam, err := st.CreateTeam(store.Team{TenantID: otherDest, Name: "Nhom seed Minh An B", LeadAgentID: other.ID})
	if err != nil {
		t.Fatal(err)
	}

	opt := Options{SourceTenant: srcTenant, DestTenant: destTenant, Deny: testDeny()}
	dry, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun {
		t.Fatal("dry-run flag")
	}
	expectFirst(t, dry)
	if dry.DeniedTenants != 0 {
		t.Fatalf("denied count %d", dry.DeniedTenants)
	}
	if mapRowCount(t, st, destTenant) != 0 {
		t.Fatal("dry-run wrote map rows")
	}
	if got := agentByKey(t, st, "minh-an-tu-van"); got != nil {
		t.Fatal("dry-run wrote an agent")
	}
	if len(agentsIn(st, destTenant)) != 2 || len(agentsIn(st, otherDest)) != 1 {
		t.Fatal("dry-run changed tenant counts")
	}

	opt.Apply = true
	first, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if first.DryRun {
		t.Fatal("apply was a dry-run")
	}
	expectFirst(t, first)
	assertNoLeak(t, st, first)
	tu := agentByKey(t, st, "minh-an-tu-van")
	phap := agentByKey(t, st, "minh-an-phap-che")
	ke := agentByKey(t, st, "minh-an-ke-toan")
	nghi := agentByKey(t, st, "minh-an-nghi")
	if tu == nil || phap == nil || ke == nil || nghi == nil {
		t.Fatal("missing imported agent")
	}
	if tu.TenantID != destTenant || tu.DisplayName != "Tu van Minh An" || tu.Instructions != "Mau tu van Minh An." || tu.Model != "sample-model" || tu.LLMProvider != "openrouter" || tu.Enabled {
		t.Fatalf("tu van %+v", tu)
	}
	if strings.Contains(tu.Instructions, "200000") || strings.Contains(tu.Instructions, descMarker) {
		t.Fatalf("instructions absorbed a dropped column: %q", tu.Instructions)
	}
	if phap.Enabled || phap.Instructions != "Mau phap che Minh An." || ke.Enabled || ke.Instructions != "Mau ke toan Minh An." {
		t.Fatal("prompt text")
	}
	if nghi.Enabled || nghi.Instructions != "Mau agent tam dung." {
		t.Fatalf("disabled %+v", nghi)
	}
	if agentByKey(t, st, "minh-an-xoa") != nil || agentByKey(t, st, "minh-an-la") != nil || agentByKey(t, st, "minh-an-thieu-id") != nil || agentByKey(t, st, "minh-an-b-tu-van") != nil {
		t.Fatal("skipped agent was written")
	}
	if len(agentsIn(st, destTenant)) != 6 || len(agentsIn(st, otherDest)) != 1 {
		t.Fatalf("tenant counts dest=%d other=%d", len(agentsIn(st, destTenant)), len(agentsIn(st, otherDest)))
	}
	teams := teamsIn(st, destTenant)
	if len(teams) != 1 || teams[0].Name != "Nhom tu van Minh An" || teams[0].LeadAgentID != tu.ID {
		t.Fatalf("teams %+v", teams)
	}
	members, err := st.ListTeamMembers(teams[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Fatalf("members %+v", members)
	}
	roleOf := map[string]string{}
	for _, m := range members {
		roleOf[m.AgentID] = m.Role
	}
	if roleOf[tu.ID] != "lead" || roleOf[phap.ID] != "member" || roleOf[ke.ID] != "member" {
		t.Fatalf("roles %+v", roleOf)
	}
	if _, ok := roleOf[nghi.ID]; ok {
		t.Fatal("disabled agent was added as lead")
	}
	if !st.HasAgentLink(tu.ID, phap.ID) || !st.HasAgentLink(phap.ID, tu.ID) || !st.HasAgentLink(tu.ID, ke.ID) || !st.HasAgentLink(ke.ID, tu.ID) {
		t.Fatal("missing link")
	}
	if st.HasAgentLink(phap.ID, ke.ID) || st.HasAgentLink(tu.ID, tu.ID) {
		t.Fatal("extra link")
	}
	otherGot, err := st.GetAgent(other.ID)
	if err != nil || otherGot.DisplayName != "Seed Minh An B" || otherGot.Instructions != keepText {
		t.Fatal("tenant B agent changed")
	}
	otherTeamGot, err := st.GetTeam(otherTeam.ID)
	if err != nil || otherTeamGot.Name != "Nhom seed Minh An B" || otherTeamGot.LeadAgentID != other.ID {
		t.Fatal("tenant B team changed")
	}
	if len(teamsIn(st, otherDest)) != 1 {
		t.Fatal("tenant B gained a team")
	}
	keptGot, err := st.GetAgent(kept.ID)
	if err != nil || keptGot.Instructions != keepText || keptGot.DisplayName != "Giu Minh An" {
		t.Fatal("pre-existing agent changed")
	}
	if !st.HasAgentLink(kept.ID, kept2.ID) {
		t.Fatal("pre-existing link changed")
	}
	memGot, err := st.GetMemory(mem.ID)
	if err != nil || memGot.Body != memoryMarker {
		t.Fatal("memory changed")
	}
	if len(st.ListVaultDocs()) != 0 {
		t.Fatal("vault changed")
	}

	ids := map[string]string{tu.AgentKey: tu.ID, phap.AgentKey: phap.ID, ke.AgentKey: ke.ID, nghi.AgentKey: nghi.ID}
	stamp := tu.UpdatedAt
	second, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	expectRerun(t, second)
	tu2 := agentByKey(t, st, "minh-an-tu-van")
	if tu2.ID != ids[tu.AgentKey] || tu2.Instructions != tu.Instructions || tu2.DisplayName != tu.DisplayName || !tu2.UpdatedAt.Equal(stamp) || tu2.Enabled {
		t.Fatalf("rerun rewrote tu van %+v", tu2)
	}
	if agentByKey(t, st, "minh-an-phap-che").ID != ids[phap.AgentKey] || agentByKey(t, st, "minh-an-nghi").ID != ids[nghi.AgentKey] || agentByKey(t, st, "minh-an-nghi").Enabled {
		t.Fatal("rerun rewrote an agent")
	}
	if len(agentsIn(st, destTenant)) != 6 || len(teamsIn(st, destTenant)) != 1 {
		t.Fatal("rerun duplicated rows")
	}
	again, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	expectRerun(t, again)
	if !agentByKey(t, st, "minh-an-tu-van").UpdatedAt.Equal(stamp) {
		t.Fatal("dry-run after apply wrote")
	}

	rbDry, err := Rollback(st, destTenant, rbOpt(false))
	if err != nil {
		t.Fatal(err)
	}
	if !rbDry.DryRun || rbDry.Removed == nil || rbDry.Removed.Agents != 4 || rbDry.Removed.Teams != 1 || rbDry.Removed.TeamMembers != 3 || rbDry.Removed.AgentLinks != 4 {
		t.Fatalf("rollback dry-run %+v", rbDry.Removed)
	}
	if agentByKey(t, st, "minh-an-tu-van") == nil || !st.HasAgentLink(kept.ID, kept2.ID) {
		t.Fatal("rollback dry-run deleted rows")
	}
	rb, err := Rollback(st, destTenant, rbOpt(true))
	if err != nil {
		t.Fatal(err)
	}
	if rb.DryRun || rb.Removed.Agents != 4 || rb.Removed.Teams != 1 || rb.Removed.TeamMembers != 3 || rb.Removed.AgentLinks != 4 {
		t.Fatalf("rollback %+v", rb.Removed)
	}
	if agentByKey(t, st, "minh-an-tu-van") != nil || agentByKey(t, st, "minh-an-nghi") != nil || len(teamsIn(st, destTenant)) != 0 {
		t.Fatal("rollback left imported rows")
	}
	if len(agentsIn(st, destTenant)) != 2 || len(agentsIn(st, otherDest)) != 1 {
		t.Fatal("rollback touched tenants it does not own")
	}
	keptGot, err = st.GetAgent(kept.ID)
	if err != nil || keptGot.Instructions != keepText || !st.HasAgentLink(kept.ID, kept2.ID) {
		t.Fatal("rollback removed pre-existing rows")
	}
	if otherGot, err = st.GetAgent(other.ID); err != nil || otherGot.DisplayName != "Seed Minh An B" {
		t.Fatal("rollback touched tenant B")
	}
	if memGot, err = st.GetMemory(mem.ID); err != nil || memGot.Body != memoryMarker {
		t.Fatal("rollback touched memory")
	}
	rb2, err := Rollback(st, destTenant, rbOpt(true))
	if err != nil {
		t.Fatal(err)
	}
	if rb2.Removed.Agents != 0 || rb2.Removed.Teams != 0 || rb2.Removed.TeamMembers != 0 || rb2.Removed.AgentLinks != 0 {
		t.Fatalf("second rollback %+v", rb2.Removed)
	}
}

func expectFirst(t *testing.T, rep *Report) {
	t.Helper()
	agents := findSection(t, rep.Agents, srcTenant)
	requireSection(t, agents, 10, 4, 3, 3, map[string]int{
		reasonDeleted: 1, reasonNativeExists: 1, reasonDuplicate: 1,
		reasonAgentKeyInvalid: 1, reasonIDMissing: 1, reasonStatusUnknown: 1,
	})
	if agents.Statuses["active"] != 3 || agents.Statuses["inactive"] != 1 || len(agents.Statuses) != 2 {
		t.Fatalf("statuses %+v", agents.Statuses)
	}
	requireSection(t, findSection(t, rep.Agents, otherSrc), 1, 0, 1, 0, map[string]int{reasonTenantMismatch: 1})
	requireSection(t, findSection(t, rep.Teams, srcTenant), 5, 1, 3, 1, map[string]int{
		reasonInactive: 1, reasonLeadMissing: 1, reasonDuplicate: 1, reasonNameMissing: 1,
	})
	requireSection(t, findSection(t, rep.Teams, otherSrc), 1, 0, 1, 0, map[string]int{reasonTenantMismatch: 1})
	requireSection(t, findSection(t, rep.TeamMembers, srcTenant), 9, 3, 5, 1, map[string]int{
		reasonTeamMissing: 1, reasonAgentNotImported: 1, reasonNativeExists: 2, reasonDuplicate: 1, reasonRoleUnsupported: 1,
	})
	requireSection(t, findSection(t, rep.TeamMembers, otherSrc), 1, 0, 1, 0, map[string]int{reasonTenantMismatch: 1})
	requireSection(t, findSection(t, rep.AgentLinks, srcTenant), 8, 3, 3, 2, map[string]int{
		reasonEndpointMissing: 1, reasonInactive: 1, reasonDuplicate: 1, reasonSelfLink: 1, reasonDirectionUnknown: 1,
	})
	requireSection(t, findSection(t, rep.AgentLinks, otherSrc), 1, 0, 1, 0, map[string]int{reasonTenantMismatch: 1})
	requireHeld(t, rep, tablePermissions, srcTenant, 2, reasonNeedsDecision)
	requireHeld(t, rep, tablePermissions, otherSrc, 1, reasonNeedsDecision)
	requireHeld(t, rep, tableOverrides, srcTenant, 1, reasonNeedsDecision)
	requireHeld(t, rep, tableProfiles, srcTenant, 1, reasonNeedsDecision)
	requireHeld(t, rep, tableShares, srcTenant, 2, reasonNeedsDecision)
	requireHeld(t, rep, "skills", srcTenant, 1, reasonNotThisSpec)
}

func expectRerun(t *testing.T, rep *Report) {
	t.Helper()
	agents := findSection(t, rep.Agents, srcTenant)
	if len(agents.Statuses) != 0 {
		t.Fatalf("rerun statuses %+v", agents.Statuses)
	}
	requireSection(t, agents, 10, 0, 7, 3, map[string]int{
		reasonUnchanged: 4, reasonDeleted: 1, reasonNativeExists: 1, reasonDuplicate: 1,
		reasonAgentKeyInvalid: 1, reasonIDMissing: 1, reasonStatusUnknown: 1,
	})
	requireSection(t, findSection(t, rep.Teams, srcTenant), 5, 0, 4, 1, map[string]int{
		reasonUnchanged: 1, reasonInactive: 1, reasonLeadMissing: 1, reasonDuplicate: 1, reasonNameMissing: 1,
	})
	requireSection(t, findSection(t, rep.TeamMembers, srcTenant), 9, 0, 8, 1, map[string]int{
		reasonUnchanged: 3, reasonTeamMissing: 1, reasonAgentNotImported: 1, reasonNativeExists: 2, reasonDuplicate: 1, reasonRoleUnsupported: 1,
	})
	requireSection(t, findSection(t, rep.AgentLinks, srcTenant), 8, 0, 6, 2, map[string]int{
		reasonUnchanged: 3, reasonEndpointMissing: 1, reasonInactive: 1, reasonDuplicate: 1, reasonSelfLink: 1, reasonDirectionUnknown: 1,
	})
}

func TestAddOnlyDoesNotOverwrite(t *testing.T) {
	body := `{
	  "agents": [{
	    "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00aa",
	    "tenant_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
	    "agent_key": "minh-an-addonly",
	    "display_name": "Ban dau Minh An",
	    "provider": "openrouter",
	    "model": "sample-model",
	    "status": "active",
	    "frontmatter": "Mau ban dau."
	  }]
	}`
	changed := strings.ReplaceAll(body, "Mau ban dau.", "Mau da sua.")
	changed = strings.ReplaceAll(changed, "Ban dau Minh An", "Ten moi Minh An")
	changed = strings.ReplaceAll(changed, "sample-model", "model-moi")
	for _, b := range openBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			ex, err := ReadExport(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Import(b.st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}); err != nil {
				t.Fatal(err)
			}
			next, err := ReadExport(strings.NewReader(changed))
			if err != nil {
				t.Fatal(err)
			}
			rep, err := Import(b.st, next, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()})
			if err != nil {
				t.Fatal(err)
			}
			sec := findSection(t, rep.Agents, srcTenant)
			if sec.Inserted != 0 || sec.Updated != 0 || sec.Reasons[reasonUnchanged] != 1 {
				t.Fatalf("report %+v", sec)
			}
			got := agentByKey(t, b.st, "minh-an-addonly")
			if got == nil || got.Instructions != "Mau ban dau." || got.DisplayName != "Ban dau Minh An" || got.Model != "sample-model" {
				t.Fatalf("overwrote %+v", got)
			}
		})
	}
}

func TestAgentKeyTakenDoesNotTouchOtherTenant(t *testing.T) {
	body := `{
	  "agents": [{
	    "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00bb",
	    "tenant_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
	    "agent_key": "minh-an-gap-key",
	    "display_name": "Muon ghi de",
	    "status": "active",
	    "frontmatter": "Khong duoc ghi."
	  }]
	}`
	for _, b := range openBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			seed, err := b.st.CreateAgent(store.Agent{
				TenantID: otherDest, AgentKey: "minh-an-gap-key", DisplayName: "Giu tenant B", Instructions: keepText,
			})
			if err != nil {
				t.Fatal(err)
			}
			ex, err := ReadExport(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			rep, err := Import(b.st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()})
			if err != nil {
				t.Fatal(err)
			}
			sec := findSection(t, rep.Agents, srcTenant)
			if sec.Inserted != 0 || sec.Reasons[reasonNativeExists] != 1 {
				t.Fatalf("report %+v", sec)
			}
			if agentByKey(t, b.st, "minh-an-gap-key").ID != seed.ID {
				t.Fatal("key collision created a second agent")
			}
			got, err := b.st.GetAgent(seed.ID)
			if err != nil || got.TenantID != otherDest || got.DisplayName != "Giu tenant B" || got.Instructions != keepText {
				t.Fatalf("other tenant changed %+v", got)
			}
			if len(agentsIn(b.st, destTenant)) != 0 {
				t.Fatal("dest gained an agent")
			}
			rb, err := Rollback(b.st, destTenant, rbOpt(true))
			if err != nil {
				t.Fatal(err)
			}
			if rb.Removed.Agents != 0 {
				t.Fatalf("rollback removed %d", rb.Removed.Agents)
			}
			if _, err := b.st.GetAgent(seed.ID); err != nil {
				t.Fatal("rollback removed the other tenant")
			}
		})
	}
}

func TestRollbackStopsWhenAgentIsStillLead(t *testing.T) {
	body := `{
	  "agents": [{
	    "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00cc",
	    "tenant_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
	    "agent_key": "minh-an-lead-block",
	    "display_name": "Lead Minh An",
	    "status": "active",
	    "frontmatter": "Mau lead."
	  }]
	}`
	st := store.New()
	ex, err := ReadExport(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	ag := agentByKey(t, st, "minh-an-lead-block")
	if ag == nil {
		t.Fatal("missing agent")
	}
	if _, err := st.CreateTeam(store.Team{TenantID: destTenant, Name: "Nhom ngoai", LeadAgentID: ag.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := Rollback(st, destTenant, rbOpt(true)); err == nil || !strings.Contains(err.Error(), "team lead") {
		t.Fatalf("err %v", err)
	}
	if agentByKey(t, st, "minh-an-lead-block") == nil {
		t.Fatal("stopped rollback deleted the agent")
	}
	for _, tm := range st.ListTeams() {
		if err := st.DeleteTeam(tm.ID); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Rollback(st, destTenant, rbOpt(true))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Removed.Agents != 1 || agentByKey(t, st, "minh-an-lead-block") != nil {
		t.Fatalf("removed %+v", rep.Removed)
	}
}

func TestEmptyDirectionIsOutbound(t *testing.T) {
	body := `{
	  "agents": [
	    {"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00d1","tenant_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","agent_key":"minh-an-dir-a","display_name":"A","status":"active"},
	    {"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00d2","tenant_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","agent_key":"minh-an-dir-b","display_name":"B","status":"active"}
	  ],
	  "agent_links": [
	    {"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00d3","tenant_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","source_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00d1","target_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00d2","status":"active","created_by":"user-lan"}
	  ]
	}`
	st := store.New()
	ex, err := ReadExport(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	a := agentByKey(t, st, "minh-an-dir-a")
	b := agentByKey(t, st, "minh-an-dir-b")
	if a == nil || b == nil || !st.HasAgentLink(a.ID, b.ID) || st.HasAgentLink(b.ID, a.ID) {
		t.Fatal("empty direction was not outbound")
	}
}

func TestJSONL(t *testing.T) {
	ex := loadFixture(t, "minh-an.jsonl")
	st := store.New()
	rep, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	requireSection(t, findSection(t, rep.Agents, srcTenant), 1, 1, 0, 0, nil)
	requireHeld(t, rep, tableShares, srcTenant, 1, reasonNeedsDecision)
	got := agentByKey(t, st, "minh-an-jsonl")
	if got == nil || got.Instructions != "Mau JSONL Minh An." || strings.Contains(got.Instructions, secretMarker) {
		t.Fatalf("jsonl agent %+v", got)
	}
	assertNoLeak(t, st, rep)
}

func TestRejectsBadExportAndBadTenant(t *testing.T) {
	if _, err := ReadExport(strings.NewReader("")); err == nil {
		t.Fatal("empty")
	}
	if _, err := ReadExport(strings.NewReader("{")); err == nil {
		t.Fatal("broken")
	}
	if _, err := ReadExport(bytes.NewReader(bytes.Repeat([]byte("x"), MaxExportBytes+1))); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("size %v", err)
	}
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{TenantID: "default", AgentKey: "minh-an-default-seed", DisplayName: "Seed"}); err != nil {
		t.Fatal(err)
	}
	ex, err := ReadExport(strings.NewReader(`{"agents":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: "not a tenant"}); err == nil {
		t.Fatal("bad dest")
	}
	if _, err := Import(st, ex, Options{DestTenant: destTenant, Deny: testDeny()}); err == nil {
		t.Fatal("missing source")
	}
	if len(agentsIn(st, "default")) != 1 || len(st.ListAgents()) != 1 {
		t.Fatal("rejected import wrote")
	}
	if _, err := Rollback(st, "not a tenant", rbOpt(true)); err == nil {
		t.Fatal("bad rollback dest")
	}
}

func TestConcurrentApplyNoDuplicates(t *testing.T) {
	ex := loadFixture(t, "minh-an.json")
	st := store.New()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var inserted int
	var firstErr error
	opt := Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rep, err := Import(st, ex, opt)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
				return
			}
			if err != nil {
				return
			}
			sec, ok := sectionByTenant(rep.Agents, srcTenant)
			if !ok {
				firstErr = errors.New("missing agent section")
				return
			}
			inserted += sec.Inserted
		}()
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	if inserted != 4 {
		t.Fatalf("inserted %d", inserted)
	}
	if len(agentsIn(st, destTenant)) != 4 {
		t.Fatalf("agents %d", len(agentsIn(st, destTenant)))
	}
}

func TestLinkAlreadyThereIsNotClaimed(t *testing.T) {
	body := `{
	  "agents": [
	    {"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e1","tenant_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","agent_key":"minh-an-link-a","display_name":"A","status":"active"},
	    {"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e2","tenant_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","agent_key":"minh-an-link-b","display_name":"B","status":"active"}
	  ],
	  "agent_links": [
	    {"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e3","tenant_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","source_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e1","target_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e2","direction":"outbound","status":"active","created_by":"user-lan"}
	  ]
	}`
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "links.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ex, err := ReadExport(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	opt := Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}
	if _, err := Import(st, ex, opt); err != nil {
		t.Fatal(err)
	}
	db, pg := st.SQL()
	if pg {
		t.Fatal("sqlite")
	}
	if _, err := db.Exec(`DELETE FROM goclaw_agent_import_map WHERE source_key=?`, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e3"); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	sec := findSection(t, rep.AgentLinks, srcTenant)
	if sec.Inserted != 0 || sec.Reasons[reasonNativeExists] != 1 {
		t.Fatalf("report %+v", sec)
	}
	var mapped int
	if err := db.QueryRow(`SELECT COUNT(*) FROM goclaw_agent_import_map WHERE source_key=?`, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00e3").Scan(&mapped); err != nil {
		t.Fatal(err)
	}
	if mapped != 0 {
		t.Fatal("native-exists adopted the edge into the map")
	}
	a := agentByKey(t, st, "minh-an-link-a")
	b := agentByKey(t, st, "minh-an-link-b")
	if a == nil || b == nil || !st.HasAgentLink(a.ID, b.ID) {
		t.Fatal("existing edge was removed")
	}
	rb, err := Rollback(st, destTenant, rbOpt(true))
	if err != nil {
		t.Fatal(err)
	}
	if rb.Removed.AgentLinks != 0 {
		t.Fatalf("claimed a pre-existing edge: %+v", rb.Removed)
	}
}

func requireSection(t *testing.T, s Section, in, ins, skip, rej int, reasons map[string]int) {
	t.Helper()
	if s.Input != in || s.Inserted != ins || s.Updated != 0 || s.Skipped != skip || s.Rejected != rej {
		t.Fatalf("counts got input=%d inserted=%d updated=%d skipped=%d rejected=%d want %d %d 0 %d %d (%+v)", s.Input, s.Inserted, s.Updated, s.Skipped, s.Rejected, in, ins, skip, rej, s.Reasons)
	}
	if len(reasons) == 0 {
		if len(s.Reasons) != 0 {
			t.Fatalf("reasons %+v", s.Reasons)
		}
		return
	}
	if len(s.Reasons) != len(reasons) {
		t.Fatalf("reasons %+v want %+v", s.Reasons, reasons)
	}
	for k, v := range reasons {
		if s.Reasons[k] != v {
			t.Fatalf("reason %s=%d want %d in %+v", k, s.Reasons[k], v, s.Reasons)
		}
	}
}

func findSection(t *testing.T, sections []Section, tenant string) Section {
	t.Helper()
	s, ok := sectionByTenant(sections, tenant)
	if !ok {
		t.Fatalf("missing section %s in %+v", tenant, sections)
	}
	return s
}

func sectionByTenant(sections []Section, tenant string) (Section, bool) {
	for _, s := range sections {
		if s.SourceTenant == tenant {
			return s, true
		}
	}
	return Section{}, false
}

func requireHeld(t *testing.T, rep *Report, table, tenant string, n int, reason string) {
	t.Helper()
	for _, h := range rep.NotImported {
		if h.Table == table && h.SourceTenant == tenant {
			if h.Input != n || h.Reason != reason {
				t.Fatalf("held %+v", h)
			}
			return
		}
	}
	t.Fatalf("missing held %s %s", table, tenant)
}

func agentByKey(t *testing.T, st store.StoreIface, key string) *store.Agent {
	t.Helper()
	var found *store.Agent
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == key {
			if found != nil {
				t.Fatalf("duplicate key %s", key)
			}
			cp := *a
			found = &cp
		}
	}
	return found
}

func agentsIn(st store.StoreIface, tenant string) []*store.Agent {
	var out []*store.Agent
	for _, a := range st.ListAgents() {
		if a != nil && store.SameTenant(a.TenantID, tenant) {
			out = append(out, a)
		}
	}
	return out
}

func teamsIn(st store.StoreIface, tenant string) []*store.Team {
	var out []*store.Team
	for _, tm := range st.ListTeams() {
		if tm != nil && store.SameTenant(tm.TenantID, tenant) {
			out = append(out, tm)
		}
	}
	return out
}

func assertNoLeak(t *testing.T, st store.StoreIface, rep *Report) {
	t.Helper()
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	blobs := []string{string(raw)}
	for _, a := range st.ListAgents() {
		if a == nil {
			continue
		}
		blobs = append(blobs, a.DisplayName, a.Instructions, a.Model, a.LLMProvider, a.AgentKey)
	}
	for _, tm := range st.ListTeams() {
		if tm != nil {
			blobs = append(blobs, tm.Name)
		}
	}
	for _, b := range blobs {
		for _, marker := range []string{secretMarker, embedMarker, modelMarker, descMarker, emojiMarker, "0.01", "blocked-sample"} {
			if strings.Contains(b, marker) {
				t.Fatalf("dropped column leaked (%s) in %q", marker, b)
			}
		}
	}
}

func mapRowCount(t *testing.T, st store.StoreIface, dest string) int {
	t.Helper()
	pool, ok := st.(sqlPool)
	if !ok {
		return 0
	}
	db, pg := pool.SQL()
	if db == nil {
		return 0
	}
	q := `SELECT COUNT(*) FROM goclaw_agent_import_map WHERE dest_tenant=?`
	if pg {
		q = `SELECT COUNT(*) FROM goclaw_agent_import_map WHERE dest_tenant=$1`
	}
	var n int
	if err := db.QueryRow(q, dest).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeniedExportTenantIsCountedNotNamed(t *testing.T) {
	const blocked = "blocked-tenant"
	body := `{
	  "agents": [
	    {
	      "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00a1",
	      "tenant_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
	      "agent_key": "minh-an-allowed",
	      "display_name": "Duoc Minh An",
	      "status": "active",
	      "frontmatter": "Mau."
	    },
	    {
	      "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00a2",
	      "tenant_id": "blocked-tenant",
	      "agent_key": "minh-an-bi-chan",
	      "display_name": "Khong ghi",
	      "status": "active"
	    }
	  ],
	  "agent_shares": [
	    {
	      "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00a3",
	      "tenant_id": "blocked-tenant",
	      "agent_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00a2",
	      "user_id": "user-lan",
	      "role": "user",
	      "granted_by": "user-hoa"
	    }
	  ]
	}`
	ex, err := ReadExport(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	rep, err := Import(st, ex, Options{
		SourceTenant: srcTenant,
		DestTenant:   destTenant,
		Apply:        true,
		Deny:         map[string]struct{}{blocked: {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DeniedTenants != 1 {
		t.Fatalf("denied count %d", rep.DeniedTenants)
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), blocked) {
		t.Fatalf("report named a denied tenant: %s", raw)
	}
	if agentByKey(t, st, "minh-an-bi-chan") != nil {
		t.Fatal("denied row was written")
	}
	got := agentByKey(t, st, "minh-an-allowed")
	if got == nil || got.Enabled || got.TenantID != destTenant {
		t.Fatalf("allowed %+v", got)
	}
	for _, sec := range append(append(append([]Section{}, rep.Agents...), rep.Teams...), append(rep.TeamMembers, rep.AgentLinks...)...) {
		if strings.EqualFold(sec.SourceTenant, blocked) {
			t.Fatalf("section %+v", sec)
		}
	}
	for _, h := range rep.NotImported {
		if strings.EqualFold(h.SourceTenant, blocked) {
			t.Fatalf("held %+v", h)
		}
	}
}

func TestConfigErrorIsError(t *testing.T) {
	var err error = &ConfigError{Msg: "dest tenant is invalid"}
	var cfg *ConfigError
	if !errors.As(err, &cfg) {
		t.Fatal("as")
	}
}

func oneAgentExport(key, status string) string {
	return `{
	  "agents": [{
	    "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaa00f1",
	    "tenant_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
	    "agent_key": "` + key + `",
	    "display_name": "Mau Minh An",
	    "status": "` + status + `",
	    "frontmatter": "Mau."
	  }]
	}`
}

func TestDenylistBlocksTenantAndBadFiles(t *testing.T) {
	body := oneAgentExport("minh-an-deny", "active")
	ex, err := ReadExport(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{TenantID: destTenant, AgentKey: "minh-an-kept", DisplayName: "Giu"}); err != nil {
		t.Fatal(err)
	}
	before := len(st.ListAgents())
	_, err = Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: map[string]struct{}{strings.ToLower(srcTenant): {}}})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("source %v", err)
	}
	_, err = Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: "blocked-sample", Apply: true, Deny: testDeny()})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("dest %v", err)
	}
	if _, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true}); err == nil {
		t.Fatal("empty denylist ran")
	}
	if len(st.ListAgents()) != before || agentByKey(t, st, "minh-an-deny") != nil {
		t.Fatal("denied import wrote a row")
	}
	dir := t.TempDir()
	if _, err := LoadDenylist(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("missing file")
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("# only a comment\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDenylist(empty); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty %v", err)
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("not valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDenylist(bad); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed %v", err)
	}
	raw, _ := json.Marshal(Report{DeniedTenants: 2})
	if strings.Contains(string(raw), "blocked-sample") || !strings.Contains(string(raw), `"denied_tenants":2`) && !strings.Contains(string(raw), `"denied_tenants": 2`) {
		t.Fatalf("report %s", raw)
	}
}

func TestReservedChannelKeysWriteNothing(t *testing.T) {
	if len(channel.Names) != 7 {
		t.Fatalf("channel catalog %v", channel.Names)
	}
	for _, key := range channel.Names {
		t.Run(key, func(t *testing.T) {
			ex, err := ReadExport(strings.NewReader(oneAgentExport(key, "active")))
			if err != nil {
				t.Fatal(err)
			}
			st := store.New()
			rep, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()})
			if err != nil {
				t.Fatal(err)
			}
			sec := findSection(t, rep.Agents, srcTenant)
			if sec.Inserted != 0 || sec.Reasons[reasonReservedKey] != 1 || agentByKey(t, st, key) != nil {
				t.Fatalf("key %s report %+v", key, sec)
			}
		})
	}
}

func TestImportedAgentMentionIsInactive(t *testing.T) {
	ex, err := ReadExport(strings.NewReader(oneAgentExport("minh-an-mention", "active")))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	if _, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	imported := agentByKey(t, st, "minh-an-mention")
	if imported == nil || imported.Enabled {
		t.Fatalf("imported %+v", imported)
	}
	host, err := st.CreateAgent(store.Agent{TenantID: destTenant, AgentKey: "minh-an-host", DisplayName: "Host"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: host.ID, TenantID: destTenant, Label: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	rt := &agent.Runtime{Store: st}
	_, err = rt.ChatWithTurn(context.Background(), sess.ID, "chao", "", false, agent.ChatTurn{MentionAgentID: imported.ID}, nil)
	if !errors.Is(err, agent.ErrAgentInactive) {
		t.Fatalf("mention %v", err)
	}
}

func TestRollbackStopsForLaterSession(t *testing.T) {
	ex, err := ReadExport(strings.NewReader(oneAgentExport("minh-an-later", "active")))
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	if _, err := Import(st, ex, Options{SourceTenant: srcTenant, DestTenant: destTenant, Apply: true, Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	ag := agentByKey(t, st, "minh-an-later")
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: destTenant, Label: "sau khi nhap"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: "Mau Minh An"}); err != nil {
		t.Fatal(err)
	}
	dry, err := Rollback(st, destTenant, rbOpt(false))
	if err != nil {
		t.Fatal(err)
	}
	if dry.PostImport == nil || dry.PostImport.Sessions != 1 || dry.PostImport.Messages != 1 {
		t.Fatalf("count %+v", dry.PostImport)
	}
	if _, err := Rollback(st, destTenant, rbOpt(true)); err == nil || !strings.Contains(err.Error(), "post-import") {
		t.Fatalf("rollback %v", err)
	}
	if _, err := st.GetSession(sess.ID); err != nil {
		t.Fatal("session was deleted")
	}
	if agentByKey(t, st, "minh-an-later") == nil {
		t.Fatal("agent was deleted")
	}
	gone, err := Rollback(st, destTenant, RollbackOptions{Apply: true, Confirm: true, Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	if gone.Removed.Agents != 1 {
		t.Fatalf("confirm %+v", gone.Removed)
	}
	if _, err := st.GetSession(sess.ID); err == nil {
		t.Fatal("confirmed rollback left the session")
	}
}
