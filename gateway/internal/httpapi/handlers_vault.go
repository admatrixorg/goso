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
		list := vaultDocsInTenant(vaultSvc(st).List(), requestTenant(r))
		if list == nil {
			list = []*store.VaultDoc{}
		}
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
		d, err := vaultSvc(st).Get(requestTenant(r), id)
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
			AgentID     string `json:"agent_id"`
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
		// Target tenant, path, agent, project, and customer fields are not
		// grants. A mismatched tenant is rejected. The others are ignored.
		if strings.TrimSpace(body.TenantID) != "" && !store.SameTenant(body.TenantID, tid) {
			writeErr(w, http.StatusForbidden, "target tenant denied")
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
		d, err := svc.PutTenant(body.Title, body.Body, tid)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		status := http.StatusCreated
		if existed != nil {
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
		d, gerr := vaultSvc(st).Get(requestTenant(r), id)
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
		hits, err := vaultSvc(st).Search(requestTenant(r), q)
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
		if g.Nodes == nil {
			g.Nodes = []vault.GraphNode{}
		}
		if g.Edges == nil {
			g.Edges = []vault.GraphEdge{}
		}
		writeJSON(w, http.StatusOK, g)
	}
}
