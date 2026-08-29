// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import "github.com/mqglobal/goso/gateway/internal/store"

const (
	// DefaultGraphNodeCap is the operator default for GET /api/vault/graph.
	DefaultGraphNodeCap = 40
	// MaxGraphNodeCap hard-stops a large-vault graph request.
	MaxGraphNodeCap = 200
)

// GraphNode is one vault document in a bounded relationship list.
type GraphNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

// GraphEdge is one [[wikilink]] in that list. This is not a canvas.
type GraphEdge struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id,omitempty"`
	Raw    string `json:"raw"`
}

// Graph is a capped node/edge list. The control plane renders it as a list.
type Graph struct {
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Truncated bool        `json:"truncated"`
	NodeCap   int         `json:"node_cap"`
}

func clampGraphLimit(limit int) int {
	if limit <= 0 {
		return DefaultGraphNodeCap
	}
	if limit > MaxGraphNodeCap {
		return MaxGraphNodeCap
	}
	return limit
}

// Graph returns a bounded relationship list. allow filters tenant rows.
func (s *Service) Graph(limit int, allow func(*store.VaultDoc) bool) *Graph {
	limit = clampGraphLimit(limit)
	g := &Graph{
		Nodes:   []GraphNode{},
		Edges:   []GraphEdge{},
		NodeCap: limit,
	}
	byID := map[string]*store.VaultDoc{}
	var allowed []*store.VaultDoc
	for _, d := range s.List() {
		if d == nil {
			continue
		}
		if allow != nil && !allow(d) {
			continue
		}
		byID[d.ID] = d
		allowed = append(allowed, d)
	}

	type packed struct {
		doc *store.VaultDoc
		ob  []store.VaultLink
		ib  []store.VaultLink
	}
	var linked []packed
	var isolated []*store.VaultDoc
	maxScan := MaxGraphNodeCap * 2
	for i, d := range allowed {
		if i >= maxScan {
			g.Truncated = true
			break
		}
		ob, ib, err := s.Links(d.ID)
		if err != nil {
			isolated = append(isolated, d)
			continue
		}
		if len(ob)+len(ib) == 0 {
			isolated = append(isolated, d)
			continue
		}
		linked = append(linked, packed{doc: d, ob: ob, ib: ib})
	}

	seen := map[string]struct{}{}
	add := func(d *store.VaultDoc) bool {
		if d == nil {
			return true
		}
		if _, ok := seen[d.ID]; ok {
			return true
		}
		if len(g.Nodes) >= limit {
			g.Truncated = true
			return false
		}
		seen[d.ID] = struct{}{}
		g.Nodes = append(g.Nodes, GraphNode{ID: d.ID, Title: d.Title, Path: d.Path})
		return true
	}

	for _, p := range linked {
		if !add(p.doc) {
			break
		}
		for _, l := range p.ob {
			if l.ToID == "" {
				continue
			}
			if t := byID[l.ToID]; t != nil && !add(t) {
				break
			}
		}
		for _, l := range p.ib {
			if t := byID[l.FromID]; t != nil && !add(t) {
				break
			}
		}
	}
	if len(g.Nodes) < limit {
		for _, d := range isolated {
			if !add(d) {
				break
			}
		}
	}

	edgeCap := limit * 2
	if edgeCap < 1 {
		edgeCap = 1
	}
	for _, p := range linked {
		if _, ok := seen[p.doc.ID]; !ok {
			continue
		}
		for _, l := range p.ob {
			if len(g.Edges) >= edgeCap {
				g.Truncated = true
				break
			}
			g.Edges = append(g.Edges, GraphEdge{FromID: l.FromID, ToID: l.ToID, Raw: l.Raw})
		}
	}
	return g
}
