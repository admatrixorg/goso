// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	ToolKnowledgeGraphSearch = "knowledge_graph_search"
	ToolMemoryGet            = "memory_get"

	// kgSearchDefault and kgSearchMax bound knowledge_graph_search.
	kgSearchDefault = 10
	kgSearchMax     = 40
	// kgNeighbourNodes is how many top entities get their relations listed.
	kgNeighbourNodes = 10
	kgRelationCap    = 80
)

func knowledgeToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name:        ToolKnowledgeGraphSearch,
			Description: "Search knowledge-graph entities of the calling agent's tenant by name, kind or text. Returns current entities (id, name, kind, snippet) and their current relations with the names at both ends. In a customer conversation only recorded facts shared or about this customer. Read-only. Args: query, limit (default 10, max 40).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"q":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`),
		},
		{
			Name:        ToolMemoryGet,
			Description: "Read one L1 memory note by id (from memory_search). Same tenant only; in a customer conversation only this customer's notes. Read-only. Args: id.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		},
	}
}

type kgSearchRelation struct {
	ID       string `json:"id"`
	FromID   string `json:"from_id"`
	FromName string `json:"from_name"`
	ToID     string `json:"to_id"`
	ToName   string `json:"to_name"`
	Rel      string `json:"rel"`
	Source   string `json:"source,omitempty"`
}

type kgSearchResult struct {
	Entities  []store.KGGraphNode `json:"entities"`
	Relations []kgSearchRelation  `json:"relations"`
	Total     int                 `json:"total"`
	Truncated bool                `json:"truncated"`
}

func argLimit(args map[string]any, def, max int) int {
	n := def
	switch v := args["limit"].(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case string:
		if x, err := strconv.Atoi(v); err == nil {
			n = x
		}
	}
	if n <= 0 {
		n = def
	}
	if n > max {
		n = max
	}
	return n
}

// dispatchKGSearch is tenant-confined. Channel callers get recorded (posted)
// entities shared or stamped with their customer, never extracted ones.
func dispatchKGSearch(st store.StoreIface, tenant, callerSessionID string, args map[string]any) (string, error) {
	q := argString(args, "query", "q")
	if q == "" {
		return `{"error":"query is required"}`, errors.New("query is required")
	}
	sc := scopeForCaller(st, callerSessionID)
	gq := store.KGGraphQuery{Tenant: tenant, Q: q, Limit: argLimit(args, kgSearchDefault, kgSearchMax)}
	if sc.channel {
		gq.Scope = store.KGSourcePosted
		gq.CustomerScoped = true
		gq.CustomerKey = sc.customer
	}
	g, err := st.ListKGGraph(gq)
	if err != nil {
		return `{"error":` + jsonQuote(err.Error()) + `}`, err
	}
	res := kgSearchResult{Entities: []store.KGGraphNode{}, Relations: []kgSearchRelation{}}
	if g != nil {
		res.Entities = append(res.Entities, g.Nodes...)
		res.Total = g.TotalNodes
		res.Truncated = g.Truncated
	}
	names := map[string]string{}
	for _, n := range res.Entities {
		names[n.ID] = n.Name
	}
	// visibleName resolves the other end of a relation under the same rules.
	visibleName := func(id string) (string, bool) {
		if n, ok := names[id]; ok {
			return n, true
		}
		e, err := st.GetKGEntity(id, sc.kgRead())
		if err != nil || e == nil || !store.SameTenant(e.TenantID, tenant) ||
			!store.KGIsCurrent(e.ValidFrom, e.ValidUntil) ||
			(sc.channel && e.Source == store.KGSourceExtracted) {
			return "", false
		}
		names[id] = e.Name
		return e.Name, true
	}
	seen := map[string]struct{}{}
	for i, n := range res.Entities {
		if i >= kgNeighbourNodes || len(res.Relations) >= kgRelationCap {
			break
		}
		rels, err := st.ListKGRelations(n.ID, sc.kgRead())
		if err != nil {
			continue
		}
		for _, r := range rels {
			if r == nil || len(res.Relations) >= kgRelationCap {
				continue
			}
			if _, ok := seen[r.ID]; ok {
				continue
			}
			if !store.SameTenant(r.TenantID, tenant) || !store.KGIsCurrent(r.ValidFrom, r.ValidUntil) {
				continue
			}
			if sc.channel && r.Source == store.KGSourceExtracted {
				continue
			}
			from, ok1 := visibleName(r.FromID)
			to, ok2 := visibleName(r.ToID)
			if !ok1 || !ok2 {
				continue
			}
			seen[r.ID] = struct{}{}
			res.Relations = append(res.Relations, kgSearchRelation{
				ID: r.ID, FromID: r.FromID, FromName: from, ToID: r.ToID, ToName: to, Rel: r.Rel, Source: r.Source,
			})
		}
	}
	return encodeVault(res)
}

// dispatchMemoryGet returns one L1 note of the tenant. A channel caller only
// reads notes stamped with its own customer, from a session it may see.
func dispatchMemoryGet(st store.StoreIface, tenant, callerSessionID string, args map[string]any) (string, error) {
	id := argString(args, "id")
	if id == "" {
		return `{"error":"id is required"}`, errors.New("id is required")
	}
	m, err := st.GetMemory(id)
	if err != nil || m == nil || !store.SameTenant(m.TenantID, tenant) {
		return `{"error":"not found"}`, store.ErrNotFound
	}
	sc := scopeForCaller(st, callerSessionID)
	if sc.channel {
		if sc.customer == "" || m.CustomerKey != sc.customer {
			return `{"error":"not found"}`, store.ErrNotFound
		}
		sess, err := st.GetSession(m.SessionID)
		if err != nil || !sc.sessionVisible(sess) {
			return `{"error":"not found"}`, store.ErrNotFound
		}
	}
	return encodeVault(m)
}
