// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

type knowledgeFixture struct {
	st                    *store.Store
	web, chMine, chOther  *store.Session
	foreign               *store.Session
	shared, mineEnt, xEnt *store.KGEntity
	foreignEnt            *store.KGEntity
	memMine, memOther     *store.Memory
	memForeign, memWeb    *store.Memory
}

func newKnowledgeFixture(t *testing.T) *knowledgeFixture {
	t.Helper()
	st := store.New()
	f := &knowledgeFixture{st: st}
	ag, err := st.CreateAgent(store.Agent{AgentKey: "tro-ly-minh-an", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	fa, err := st.CreateAgent(store.Agent{AgentKey: "tro-ly-khac", TenantID: "tenant-khac"})
	if err != nil {
		t.Fatal(err)
	}
	sess := func(agent, tenant, label string) *store.Session {
		s, err := st.CreateSession(store.Session{AgentID: agent, TenantID: tenant, Label: label})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	f.web = sess(ag.ID, "minh-an", "")
	f.chMine = sess(ag.ID, "minh-an", "zalo-oa:1001")
	f.chOther = sess(ag.ID, "minh-an", "zalo-oa:2002")
	f.foreign = sess(fa.ID, "tenant-khac", "")
	ent := func(tenant, name, source, sessionID string) *store.KGEntity {
		e, err := st.PutKGEntity(store.KGEntity{TenantID: tenant, Name: name, Kind: "product", Source: source, SessionID: sessionID, ValidFrom: time.Now().Add(-time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	f.shared = ent("minh-an", "Gói Minh An Pro", store.KGSourcePosted, "")
	f.mineEnt = ent("minh-an", "Gói Minh An khách 1001", store.KGSourcePosted, f.chMine.ID)
	f.xEnt = ent("minh-an", "Gói Minh An suy luận", store.KGSourceExtracted, "")
	f.foreignEnt = ent("tenant-khac", "Gói Minh An tenant khác", store.KGSourcePosted, "")
	if _, err := st.PutKGRelation(store.KGRelation{TenantID: "minh-an", FromID: f.shared.ID, ToID: f.mineEnt.ID, Rel: "includes", Source: store.KGSourcePosted, ValidFrom: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutKGRelation(store.KGRelation{TenantID: "minh-an", FromID: f.shared.ID, ToID: f.xEnt.ID, Rel: "related", Source: store.KGSourceExtracted, ValidFrom: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	mem := func(s *store.Session, body string) *store.Memory {
		m, err := st.PutMemory(store.Memory{TenantID: s.TenantID, SessionID: s.ID, Kind: "summary", Body: body})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	f.memMine = mem(f.chMine, "Khách 1001 hỏi bảng giá")
	f.memOther = mem(f.chOther, "Khách 2002 hỏi bảo hành")
	f.memForeign = mem(f.foreign, "Ghi chú tenant khác")
	f.memWeb = mem(f.web, "Ghi chú vận hành")
	return f
}

func callKnowledge(t *testing.T, st store.StoreIface, tenant, sessionID, name string, args map[string]any) (string, error) {
	t.Helper()
	return DispatchMemoryTool(st, tenant, sessionID, llm.ToolCall{Name: name, Arguments: args})
}

func TestKnowledgeGraphSearch_TenantAndChannel(t *testing.T) {
	f := newKnowledgeFixture(t)
	body, err := callKnowledge(t, f.st, "minh-an", f.web.ID, ToolKnowledgeGraphSearch, map[string]any{"query": "Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{f.shared.ID, f.mineEnt.ID, f.xEnt.ID} {
		if !strings.Contains(body, id) {
			t.Fatalf("web caller missing %s: %s", id, body)
		}
	}
	if strings.Contains(body, f.foreignEnt.ID) || !strings.Contains(body, `"rel":"includes"`) {
		t.Fatalf("web caller: %s", body)
	}
	// Channel: recorded facts shared or about this customer only.
	body, err = callKnowledge(t, f.st, "minh-an", f.chOther.ID, ToolKnowledgeGraphSearch, map[string]any{"query": "Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, f.shared.ID) || strings.Contains(body, f.mineEnt.ID) || strings.Contains(body, f.xEnt.ID) ||
		strings.Contains(body, `"rel":"includes"`) || strings.Contains(body, `"rel":"related"`) {
		t.Fatalf("other customer's channel: %s", body)
	}
	body, _ = callKnowledge(t, f.st, "minh-an", f.chMine.ID, ToolKnowledgeGraphSearch, map[string]any{"query": "Minh An"})
	if !strings.Contains(body, f.mineEnt.ID) || !strings.Contains(body, `"rel":"includes"`) || strings.Contains(body, f.xEnt.ID) {
		t.Fatalf("own customer's channel: %s", body)
	}
	if _, err := callKnowledge(t, f.st, "minh-an", f.web.ID, ToolKnowledgeGraphSearch, map[string]any{}); err == nil {
		t.Fatal("empty query accepted")
	}
}

func TestMemoryGet_TenantAndChannel(t *testing.T) {
	f := newKnowledgeFixture(t)
	ok := func(sessionID, id string) bool {
		body, err := callKnowledge(t, f.st, "minh-an", sessionID, ToolMemoryGet, map[string]any{"id": id})
		return err == nil && strings.Contains(body, id)
	}
	if !ok(f.web.ID, f.memMine.ID) || !ok(f.web.ID, f.memWeb.ID) {
		t.Fatal("web caller cannot read tenant notes")
	}
	if ok(f.web.ID, f.memForeign.ID) {
		t.Fatal("read another tenant's note")
	}
	if !ok(f.chMine.ID, f.memMine.ID) {
		t.Fatal("channel lost its own customer's note")
	}
	if ok(f.chMine.ID, f.memOther.ID) || ok(f.chMine.ID, f.memWeb.ID) {
		t.Fatal("channel read a note of another customer or an operator note")
	}
}

func TestVaultTool_AgentScopeAndChannelCustomer(t *testing.T) {
	st := store.New()
	root := vaultRoot(t)
	x, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an-x", TenantID: "acme"})
	y, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an-y", TenantID: "acme"})
	svc := vault.New(st, root)
	own, err := svc.PutTenantAs("Kịch bản riêng X", "kịch bản chốt đơn", "acme", vault.Access{Scope: "agent", AgentID: x.ID})
	if err != nil {
		t.Fatal(err)
	}
	cust, err := svc.PutTenantAs("Ghi chú khách 42", "kịch bản cho khách 42", "acme", vault.Access{CustomerKey: store.CustomerKeyForLabel("telegram:42")})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.PutTenantAs("Ghi chú khách 99", "kịch bản cho khách 99", "acme", vault.Access{CustomerKey: store.CustomerKeyForLabel("telegram:99")})
	if err != nil {
		t.Fatal(err)
	}
	search := func(agentID, sessionID string) string {
		body, err := DispatchVaultToolAs(st, root, "acme", agentID, sessionID, llm.ToolCall{Name: ToolVaultSearch, Arguments: map[string]any{"query": "kịch bản"}})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	if b := search(x.ID, ""); !strings.Contains(b, own.ID) {
		t.Fatalf("agent X lost its doc: %s", b)
	}
	if b := search(y.ID, ""); strings.Contains(b, own.ID) {
		t.Fatalf("agent Y saw agent X's doc: %s", b)
	}
	if _, err := DispatchVaultToolAs(st, root, "acme", y.ID, "", llm.ToolCall{Name: ToolVaultRead, Arguments: map[string]any{"id": own.ID}}); err == nil {
		t.Fatal("agent Y read agent X's doc")
	}
	sess, err := st.CreateSession(store.Session{AgentID: y.ID, TenantID: "acme", Label: "telegram:42"})
	if err != nil {
		t.Fatal(err)
	}
	b := search("", sess.ID)
	if !strings.Contains(b, cust.ID) || strings.Contains(b, other.ID) || strings.Contains(b, own.ID) {
		t.Fatalf("channel thread of customer 42: %s", b)
	}
	// A tenant channel instance label is a channel thread too.
	inst, err := st.CreateSession(store.Session{AgentID: y.ID, TenantID: "acme", Label: "acme/zalo-oa/minh-an-bot:7"})
	if err != nil {
		t.Fatal(err)
	}
	if b := search("", inst.ID); strings.Contains(b, cust.ID) || strings.Contains(b, other.ID) || strings.Contains(b, own.ID) {
		t.Fatalf("tenant instance thread saw another customer's doc: %s", b)
	}
}
