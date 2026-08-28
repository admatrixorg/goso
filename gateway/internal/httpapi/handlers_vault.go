// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
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
		d, err := vaultSvc(st).Get(id)
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
			Title string `json:"title"`
			Body  string `json:"body"`
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
		existed, _ := st.FindVaultDocByTitle(body.Title)
		if existed != nil && !store.SameTenant(existed.TenantID, tid) {
			existed = nil
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
		d, gerr := vaultSvc(st).Get(id)
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
		hits, err := vaultSvc(st).Search(q)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if hits == nil {
			hits = []store.VaultSearchHit{}
		}
		tid := requestTenant(r)
		out := make([]store.VaultSearchHit, 0, len(hits))
		for _, h := range hits {
			d, err := st.GetVaultDoc(h.ID)
			if err == nil && d != nil && store.SameTenant(d.TenantID, tid) {
				out = append(out, h)
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleSyncVault(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := vaultSvc(st).Sync()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}
