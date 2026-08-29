// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

const (
	// KGGraphDefaultNodeCap is the operator default for GET /api/kg/graph.
	KGGraphDefaultNodeCap = 40
	// KGGraphMaxNodeCap hard-stops a large-graph request.
	KGGraphMaxNodeCap = 200
	KGSourcePosted    = "posted"
	KGSourceExtracted = "extracted"
)

// KGGraphQuery filters an agent-scoped node/edge list.
type KGGraphQuery struct {
	Tenant  string
	AgentID string
	Scope   string
	Q       string
	Limit   int
}

// KGGraphNode is one L2 entity in the bounded explorer list. No canvas.
type KGGraphNode struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Snippet    string     `json:"snippet"`
	AgentID    string     `json:"agent_id,omitempty"`
	Source     string     `json:"source"`
	Inferred   bool       `json:"inferred"`
	CreatedAt  time.Time  `json:"created_at"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// KGGraphEdge is one recorded or extracted relation. Inferred is not a fact.
type KGGraphEdge struct {
	ID       string `json:"id"`
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	FromName string `json:"from_name,omitempty"`
	ToName   string `json:"to_name,omitempty"`
	Rel      string `json:"rel"`
	Source   string `json:"source"`
	Inferred bool   `json:"inferred"`
}

// KGGraph is a capped node/edge list for the explorer. The control plane
// renders it as a list; this is not a canvas.
type KGGraph struct {
	Nodes               []KGGraphNode `json:"nodes"`
	Edges               []KGGraphEdge `json:"edges"`
	Truncated           bool          `json:"truncated"`
	NodeCap             int           `json:"node_cap"`
	EdgeCap             int           `json:"edge_cap"`
	TotalNodes          int           `json:"total_nodes"`
	TotalEdges          int           `json:"total_edges"`
	InferredAreNotFacts bool          `json:"inferred_are_not_facts"`
}

func ClampKGGraphLimit(limit int) int {
	if limit <= 0 {
		return KGGraphDefaultNodeCap
	}
	if limit > KGGraphMaxNodeCap {
		return KGGraphMaxNodeCap
	}
	return limit
}

func NormalizeKGSource(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case KGSourceExtracted, "inferred", "heuristic", "extract":
		return KGSourceExtracted
	case KGSourcePosted, "recorded", "explicit":
		return KGSourcePosted
	default:
		return ""
	}
}

func resolveKGSource(source, kind string) string {
	if src := NormalizeKGSource(source); src != "" {
		return src
	}
	if strings.EqualFold(strings.TrimSpace(kind), "extracted") {
		return KGSourceExtracted
	}
	return KGSourcePosted
}

func KGInferred(source string) bool {
	return NormalizeKGSource(source) == KGSourceExtracted || source == KGSourceExtracted
}

func kgLooksSecret(s string) bool {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "private key") || strings.Contains(s, "-----") {
		return true
	}
	if strings.Contains(s, "sk-") || strings.Contains(s, "gsk_") || strings.Contains(s, "xai-") {
		return true
	}
	if strings.Contains(lower, "bearer ") {
		return true
	}
	if strings.Contains(lower, "api_key") || strings.Contains(lower, "bot_token") {
		return true
	}
	return false
}

func kgPublicSnippet(name, body string) string {
	name = strings.TrimSpace(name)
	body = strings.TrimSpace(body)
	blob := strings.TrimSpace(name + " " + body)
	if kgLooksSecret(blob) {
		return SnippetAround(name, name, 80)
	}
	return SnippetAround(blob, name, 80)
}

func matchKGScope(scope, source string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "all":
		return true
	case KGSourcePosted, "recorded":
		return source == KGSourcePosted
	case KGSourceExtracted, "inferred":
		return source == KGSourceExtracted
	default:
		return true
	}
}

// BuildKGGraph returns a bounded, agent-filtered node/edge list. Graph GET
// never includes entity body — only a secret-stripped snippet.
func BuildKGGraph(ents []*KGEntity, rels []*KGRelation, q KGGraphQuery) *KGGraph {
	limit := ClampKGGraphLimit(q.Limit)
	edgeCap := limit * 2
	if edgeCap < 1 {
		edgeCap = 1
	}
	g := &KGGraph{
		Nodes:               []KGGraphNode{},
		Edges:               []KGGraphEdge{},
		NodeCap:             limit,
		EdgeCap:             edgeCap,
		InferredAreNotFacts: true,
	}
	agent := strings.TrimSpace(q.AgentID)
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	tenant := NormalizeTenant(q.Tenant)

	var matched []*KGEntity
	for _, e := range ents {
		if e == nil || !kgCurrent(e.ValidFrom, e.ValidUntil) {
			continue
		}
		if !SameTenant(e.TenantID, tenant) {
			continue
		}
		if agent != "" && strings.TrimSpace(e.AgentID) != agent {
			continue
		}
		src := resolveKGSource(e.Source, e.Kind)
		if !matchKGScope(q.Scope, src) {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(e.Name + " " + e.Kind + " " + e.ID)
			if !kgLooksSecret(e.Body) {
				hay += " " + strings.ToLower(e.Body)
			}
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		cp := *e
		cp.Source = src
		matched = append(matched, &cp)
	}
	g.TotalNodes = len(matched)
	if len(matched) > limit {
		g.Truncated = true
		matched = matched[:limit]
	}
	names := map[string]string{}
	seen := map[string]struct{}{}
	for _, e := range matched {
		src := resolveKGSource(e.Source, e.Kind)
		names[e.ID] = e.Name
		seen[e.ID] = struct{}{}
		g.Nodes = append(g.Nodes, KGGraphNode{
			ID:         e.ID,
			Name:       e.Name,
			Kind:       e.Kind,
			Snippet:    kgPublicSnippet(e.Name, e.Body),
			AgentID:    e.AgentID,
			Source:     src,
			Inferred:   KGInferred(src),
			CreatedAt:  e.CreatedAt,
			ValidFrom:  e.ValidFrom,
			ValidUntil: e.ValidUntil,
		})
	}

	var matchedRels []*KGRelation
	for _, r := range rels {
		if r == nil || !kgCurrent(r.ValidFrom, r.ValidUntil) {
			continue
		}
		if !SameTenant(r.TenantID, tenant) {
			continue
		}
		if _, ok := seen[r.FromID]; !ok {
			continue
		}
		if _, ok := seen[r.ToID]; !ok {
			continue
		}
		src := resolveKGSource(r.Source, "")
		if !matchKGScope(q.Scope, src) {
			continue
		}
		cp := *r
		cp.Source = src
		matchedRels = append(matchedRels, &cp)
	}
	g.TotalEdges = len(matchedRels)
	if len(matchedRels) > edgeCap {
		g.Truncated = true
		matchedRels = matchedRels[:edgeCap]
	}
	for _, r := range matchedRels {
		src := resolveKGSource(r.Source, "")
		g.Edges = append(g.Edges, KGGraphEdge{
			ID:       r.ID,
			FromID:   r.FromID,
			ToID:     r.ToID,
			FromName: names[r.FromID],
			ToName:   names[r.ToID],
			Rel:      r.Rel,
			Source:   src,
			Inferred: KGInferred(src),
		})
	}
	return g
}
