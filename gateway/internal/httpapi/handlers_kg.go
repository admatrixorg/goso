// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleCreateKGEntity(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			Body string `json:"body"`
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
