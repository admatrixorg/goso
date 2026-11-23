// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func vaultSvc(st store.StoreIface) *vault.Service {
	return vault.New(st, vault.Dir())
}

func handleListVaultDocs(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, ok := vaultViewerFromRequest(w, r, st)
		if !ok {
			return
		}
		list := vaultDocsVisible(vaultDocsInTenant(vaultSvc(st).List(), requestTenant(r)), v)
		writeJSON(w, http.StatusOK, map[string]any{"docs": list})
	}
}

func handleGetVaultDoc(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		v, ok := vaultViewerFromRequest(w, r, st)
		if !ok {
			return
		}
		d, err := vaultSvc(st).GetAs(requestTenant(r), id, v)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if hideWrongTenant(w, d.TenantID, requestTenant(r)) {
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func handlePutVaultDoc(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title       string `json:"title"`
			Body        string `json:"body"`
			TenantID    string `json:"tenant_id"`
			Path        string `json:"path"`
			Scope       string `json:"scope"`
			AgentID     string `json:"agent_id"`
			TeamID      string `json:"team_id"`
			ProjectID   string `json:"project_id"`
			CustomerKey string `json:"customer_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		if body.Title == "" {
			writeErr(w, http.StatusBadRequest, "title is required")
			return
		}
		svc := vaultSvc(st)
		tid := requestTenant(r)
		// Target tenant and path are not grants: a mismatched tenant is
		// rejected and path is ignored. scope, agent_id, team_id and
		// customer_key narrow who reads the doc inside this tenant; an agent
		// or team of another tenant is a 400. project_id is ignored.
		if strings.TrimSpace(body.TenantID) != "" && !store.SameTenant(body.TenantID, tid) {
			writeErr(w, http.StatusForbidden, "target tenant denied")
			return
		}
		acc, err := vault.ResolveAccess(st, tid, vault.Access{Scope: body.Scope, AgentID: body.AgentID, TeamID: body.TeamID, CustomerKey: body.CustomerKey})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(body.Path) != "" {
			if _, err := vault.LogicalPath(body.Path); err != nil {
				writeErr(w, http.StatusBadRequest, "path escape")
				return
			}
		}
		existed, err := st.FindVaultDocByTitle(tid, body.Title)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if existed != nil && !store.SameTenant(existed.TenantID, tid) {
			writeErr(w, http.StatusForbidden, "target tenant denied")
			return
		}
		d, err := svc.PutTenantAs(body.Title, body.Body, tid, acc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		status := http.StatusCreated
		if existed != nil && existed.ID == d.ID {
			status = http.StatusOK
		}
		writeJSON(w, status, d)
	}
}

func handleVaultDocLinks(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		v, ok := vaultViewerFromRequest(w, r, st)
		if !ok {
			return
		}
		d, gerr := vaultSvc(st).GetAs(requestTenant(r), id, v)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, gerr.Error())
			return
		}
		if hideWrongTenant(w, d.TenantID, requestTenant(r)) {
			return
		}
		outbound, inbound, err := vaultSvc(st).Links(id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if outbound == nil {
			outbound = []store.VaultLink{}
		}
		if inbound == nil {
			inbound = []store.VaultLink{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"outbound": outbound, "inbound": inbound})
	}
}

func handleSearchVault(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeErr(w, http.StatusBadRequest, "q is required")
			return
		}
		v, ok := vaultViewerFromRequest(w, r, st)
		if !ok {
			return
		}
		hits, err := vaultSvc(st).SearchAs(requestTenant(r), q, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if hits == nil {
			hits = []store.VaultSearchHit{}
		}
		writeJSON(w, http.StatusOK, hits)
	}
}

func handleSyncVault(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Query and header scope are not a second tenant. requestTenant is
		// the existing resolver (admin token + multi-tenant, otherwise default).
		res, err := vaultSvc(st).SyncTenant(requestTenant(r))
		if err != nil {
			if errors.Is(err, vault.ErrSyncDisabled) || errors.Is(err, vault.ErrTargetTenant) {
				writeErr(w, http.StatusForbidden, err.Error())
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleVaultInventory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// apply=1 is ignored. This handler never writes.
		inv, err := vaultSvc(st).Inventory(requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, inv)
	}
}

func handleVaultHealth(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h, err := vaultSvc(st).Health(requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, h)
	}
}

// handleVaultGraph answers the compact DTO the control plane graph view reads.
func handleVaultGraph(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := vault.DefaultGraphNodeCap
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err == nil {
				limit = n
			}
		}
		g := vaultSvc(st).Graph(limit, requestTenant(r))
		type gnode struct {
			ID  string `json:"id"`
			T   string `json:"t"`
			P   string `json:"p"`
			DT  string `json:"dt"`
			Deg int    `json:"deg"`
		}
		type gedge struct {
			ID   string `json:"id"`
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		}
		deg := map[string]int{}
		for _, e := range g.Edges {
			deg[e.FromID]++
			if e.ToID != "" {
				deg[e.ToID]++
			}
		}
		nodes := make([]gnode, 0, len(g.Nodes))
		nodeSet := map[string]bool{}
		for _, n := range g.Nodes {
			nodeSet[n.ID] = true
			nodes = append(nodes, gnode{ID: n.ID, T: n.Title, P: n.Path, DT: "document", Deg: deg[n.ID]})
		}
		edges := make([]gedge, 0, len(g.Edges))
		for _, e := range g.Edges {
			if e.FromID == "" || e.ToID == "" || !nodeSet[e.FromID] || !nodeSet[e.ToID] {
				continue
			}
			edges = append(edges, gedge{ID: e.FromID + "->" + e.ToID, From: e.FromID, To: e.ToID, Type: "wikilink"})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"nodes": nodes, "edges": edges,
			"total_nodes": len(nodes), "total_edges": len(edges),
			"truncated": g.Truncated, "node_cap": g.NodeCap,
		})
	}
}
