// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// supersedeBody is optional: {"valid_until": RFC3339}. Empty body = now.
func parseSupersedeBody(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	var body struct {
		ValidUntil string `json:"valid_until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return time.Time{}, false
	}
	raw := strings.TrimSpace(body.ValidUntil)
	if raw == "" {
		return time.Now().UTC(), true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "valid_until must be RFC3339")
		return time.Time{}, false
	}
	return t.UTC(), true
}

func writeSupersedeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeErr(w, http.StatusBadRequest, err.Error())
}

// POST /api/kg/entities/{id}/supersede
func handleSupersedeKGEntity(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		until, ok := parseSupersedeBody(w, r)
		if !ok {
			return
		}
		if err := st.SupersedeKGEntity(requestTenant(r), id, until); err != nil {
			writeSupersedeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "valid_until": until})
	}
}

// POST /api/kg/relations/{id}/supersede
func handleSupersedeKGRelation(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		until, ok := parseSupersedeBody(w, r)
		if !ok {
			return
		}
		if err := st.SupersedeKGRelation(requestTenant(r), id, until); err != nil {
			writeSupersedeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "valid_until": until})
	}
}
