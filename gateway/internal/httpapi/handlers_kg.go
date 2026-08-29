// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleCreateKGEntity(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name    string `json:"name"`
			Kind    string `json:"kind"`
			Body    string `json:"body"`
			AgentID string `json:"agent_id"`
			Source  string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		e, err := st.PutKGEntity(store.KGEntity{
			TenantID: requestTenant(r),
			Name:     strings.TrimSpace(body.Name),
			Kind:     strings.TrimSpace(body.Kind),
			Body:     strings.TrimSpace(body.Body),
			AgentID:  strings.TrimSpace(body.AgentID),
			Source:   strings.TrimSpace(body.Source),
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, e)
	}
}

func handleCreateKGRelation(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FromID string `json:"from_id"`
			ToID   string `json:"to_id"`
			Rel    string `json:"rel"`
			Body   string `json:"body"`
			Source string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		rel, err := st.PutKGRelation(store.KGRelation{
			TenantID: requestTenant(r),
			FromID:   strings.TrimSpace(body.FromID),
			ToID:     strings.TrimSpace(body.ToID),
			Rel:      strings.TrimSpace(body.Rel),
			Body:     strings.TrimSpace(body.Body),
			Source:   strings.TrimSpace(body.Source),
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, rel)
	}
}

func handleSearchKG(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeErr(w, http.StatusBadRequest, "q is required")
			return
		}
		hits, err := st.SearchProgressive(q, requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if hits == nil {
			hits = []store.KGSearchHit{}
		}
		writeJSON(w, http.StatusOK, hits)
	}
}

func handleExpandKG(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		exp, err := st.ExpandKG(id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if exp == nil || exp.Entity == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if hideWrongTenant(w, exp.Entity.TenantID, requestTenant(r)) {
			return
		}
		if exp.Relations == nil {
			exp.Relations = []store.KGRelationExpand{}
		}
		writeJSON(w, http.StatusOK, exp)
	}
}

func handleGraphKG(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aid := strings.TrimSpace(r.URL.Query().Get("agent_id"))
		if aid == "" {
			writeErr(w, http.StatusBadRequest, "agent_id is required")
			return
		}
		tid := requestTenant(r)
		if _, err := agentVisible(st, aid, tid); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		limit := store.KGGraphDefaultNodeCap
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				limit = n
			}
		}
		g, err := st.ListKGGraph(store.KGGraphQuery{
			Tenant:  tid,
			AgentID: aid,
			Scope:   strings.TrimSpace(r.URL.Query().Get("scope")),
			Q:       strings.TrimSpace(r.URL.Query().Get("q")),
			Limit:   limit,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if g == nil {
			g = store.BuildKGGraph(nil, nil, store.KGGraphQuery{Limit: limit})
		}
		if g.Nodes == nil {
			g.Nodes = []store.KGGraphNode{}
		}
		if g.Edges == nil {
			g.Edges = []store.KGGraphEdge{}
		}
		fts := st.HasMemoryFTS()
		search := "substring"
		if fts {
			search = "fts5"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"nodes":                  g.Nodes,
			"edges":                  g.Edges,
			"truncated":              g.Truncated,
			"node_cap":               g.NodeCap,
			"edge_cap":               g.EdgeCap,
			"total_nodes":            g.TotalNodes,
			"total_edges":            g.TotalEdges,
			"inferred_are_not_facts": true,
			"search":                 search,
			"fts":                    fts,
			"embedding":              "not_configured",
			"embedding_configured":   false,
		})
	}
}
