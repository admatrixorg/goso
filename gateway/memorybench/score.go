// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memorybench

import (
	"fmt"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// CaseResult is one scored question.
type CaseResult struct {
	ID         string
	Group      string
	Title      string
	Pass       bool
	Injected   int
	Note       string
	Reason     string
	ExpectZero bool
}

// GroupScore is a pass count for one group.
type GroupScore struct {
	ID     string
	Name   string
	Passed int
	Total  int
	Cases  map[string]bool
}

// EngineScore is one store's run of the whole suite.
type EngineScore struct {
	Engine  string
	Passed  int
	Total   int
	Groups  map[string]GroupScore
	Results []CaseResult
}

func (g GroupScore) Score() string { return fmt.Sprintf("%d/%d", g.Passed, g.Total) }

func (e EngineScore) Score() string { return fmt.Sprintf("%d/%d", e.Passed, e.Total) }

func (e EngineScore) ByID() map[string]CaseResult {
	out := make(map[string]CaseResult, len(e.Results))
	for _, r := range e.Results {
		out[r.ID] = r
	}
	return out
}

// Run writes each case into a clean store and scores pipeline.RecallFor.
// The store is wiped between cases so bm25 document frequency stays local
// to that case.
func Run(st store.StoreIface, cases []Case) (EngineScore, error) {
	if err := Validate(cases); err != nil {
		return EngineScore{}, err
	}
	results := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		got, err := runCase(st, c)
		if err != nil {
			return EngineScore{}, err
		}
		results = append(results, got)
	}
	return summarize(results), nil
}

func summarize(results []CaseResult) EngineScore {
	out := EngineScore{
		Total:   len(results),
		Groups:  map[string]GroupScore{},
		Results: results,
	}
	for _, g := range Groups {
		out.Groups[g.ID] = GroupScore{ID: g.ID, Name: g.Name, Cases: map[string]bool{}}
	}
	for _, r := range results {
		g := out.Groups[r.Group]
		g.Total++
		g.Cases[r.ID] = r.Pass
		if r.Pass {
			g.Passed++
			out.Passed++
		}
		out.Groups[r.Group] = g
	}
	return out
}

type madeSession struct {
	id      string
	agentID string
	tenant  string
}

func runCase(st store.StoreIface, c Case) (CaseResult, error) {
	res := CaseResult{
		ID: c.ID, Group: c.Group, Title: c.Title, ExpectZero: c.ExpectZero,
	}
	if err := resetStore(st); err != nil {
		return res, fmt.Errorf("%s: reset: %w", c.ID, err)
	}
	sessions, err := writeCase(st, c)
	if err != nil {
		return res, err
	}
	ask, ok := sessions[c.AskSession]
	if !ok {
		return res, fmt.Errorf("%s: ask session was not created", c.ID)
	}
	q := store.RecallQuery{
		TenantID:       ask.tenant,
		AgentID:        ask.agentID,
		ExcludeSession: ask.id,
		Query:          c.Query,
		CustomerScoped: c.CustomerScoped,
	}
	if c.CustomerScoped {
		q.CustomerKey = store.CustomerKeyForLabel(c.AskLabel)
		if q.CustomerKey == "" {
			return res, fmt.Errorf("%s: empty customer key", c.ID)
		}
	}
	if _, err := st.RecallMemories(q); err != nil {
		return res, fmt.Errorf("%s: recall: %w", c.ID, err)
	}
	note, n := pipeline.RecallFor(st, q)
	res.Note = note
	res.Injected = n
	res.Pass, res.Reason = judge(c, note, n)
	return res, nil
}

func judge(c Case, note string, n int) (bool, string) {
	if c.ExpectZero {
		if n == 0 && strings.TrimSpace(note) == "" {
			return true, ""
		}
		return false, fmt.Sprintf("want 0 items, injected %d", n)
	}
	lower := strings.ToLower(note)
	var missing, forbidden []string
	for _, s := range c.MustContain {
		if !strings.Contains(lower, strings.ToLower(s)) {
			missing = append(missing, s)
		}
	}
	for _, s := range c.MustNotContain {
		if strings.Contains(lower, strings.ToLower(s)) {
			forbidden = append(forbidden, s)
		}
	}
	if n > 0 && len(missing) == 0 && len(forbidden) == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("injected=%d missing=%v forbidden=%v", n, missing, forbidden)
}

type sessSpec struct {
	name   string
	agent  string
	tenant string
	label  string
}

func writeCase(st store.StoreIface, c Case) (map[string]madeSession, error) {
	specs := map[string]sessSpec{}
	add := func(name, agent, tenant, label string) error {
		name = strings.TrimSpace(name)
		agent = logicalAgent(agent)
		tenant = c.tenantOf(tenant)
		prev, ok := specs[name]
		if !ok {
			specs[name] = sessSpec{name: name, agent: agent, tenant: tenant, label: label}
			return nil
		}
		if prev.agent != agent || prev.tenant != tenant {
			return fmt.Errorf("%s: session %s is used by two agents", c.ID, name)
		}
		if label != "" && prev.label != "" && label != prev.label {
			return fmt.Errorf("%s: session %s has two labels", c.ID, name)
		}
		if prev.label == "" && label != "" {
			prev.label = label
			specs[name] = prev
		}
		return nil
	}
	for _, n := range c.Notes {
		if err := add(n.Session, n.Agent, n.Tenant, n.Label); err != nil {
			return nil, err
		}
	}
	if err := add(c.AskSession, c.AskAgent, c.AskTenant, c.AskLabel); err != nil {
		return nil, err
	}

	type ak struct{ tenant, agent string }
	agentIDs := map[ak]string{}
	for _, spec := range specs {
		key := ak{spec.tenant, spec.agent}
		if _, ok := agentIDs[key]; ok {
			continue
		}
		created, err := st.CreateAgent(store.Agent{
			AgentKey:    spec.tenant + "-" + spec.agent + "-" + c.ID,
			DisplayName: agentLabel(spec.agent),
			TenantID:    spec.tenant,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: create agent: %w", c.ID, err)
		}
		agentIDs[key] = created.ID
	}

	out := map[string]madeSession{}
	for name, spec := range specs {
		created, err := st.CreateSession(store.Session{
			AgentID:  agentIDs[ak{spec.tenant, spec.agent}],
			TenantID: spec.tenant,
			Label:    spec.label,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: create session %s: %w", c.ID, name, err)
		}
		out[name] = madeSession{id: created.ID, agentID: created.AgentID, tenant: created.TenantID}
	}

	for _, n := range c.Notes {
		sess := out[n.Session]
		kind := n.Kind
		if kind == "" {
			kind = store.KindEpisodic
		}
		if _, err := st.PutMemory(store.Memory{
			SessionID: sess.id,
			TenantID:  sess.tenant,
			Kind:      kind,
			Body:      n.Body,
		}); err != nil {
			return nil, fmt.Errorf("%s: put memory: %w", c.ID, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	return out, nil
}

func resetStore(st store.StoreIface) error {
	for _, a := range st.ListAgents() {
		if a == nil {
			continue
		}
		if err := st.DeleteAgent(a.ID); err != nil {
			return err
		}
	}
	return nil
}

// FormatReport prints group scores and the cases that missed, for the CI log.
func FormatReport(engine string, score EngineScore) string {
	var b strings.Builder
	fmt.Fprintf(&b, "memory-vi %s %s\n", engine, score.Score())
	for _, g := range Groups {
		gs := score.Groups[g.ID]
		fmt.Fprintf(&b, "  %s %s\n", gs.Name, gs.Score())
	}
	for _, r := range score.Results {
		if r.Pass {
			continue
		}
		fmt.Fprintf(&b, "  miss %s %s (%s)\n", r.ID, r.Title, r.Reason)
	}
	return b.String()
}
