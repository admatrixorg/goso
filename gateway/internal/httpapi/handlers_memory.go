// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleListMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sid == "" {
			writeErr(w, http.StatusBadRequest, "session_id is required")
			return
		}
		list, err := st.ListMemories(sid)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if list == nil {
			list = []*store.Memory{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"memories": list})
	}
}

func handleCreateMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
			Body      string `json:"body"`
			Kind      string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.SessionID = strings.TrimSpace(body.SessionID)
		body.Body = strings.TrimSpace(body.Body)
		if body.SessionID == "" {
			writeErr(w, http.StatusBadRequest, "session_id is required")
			return
		}
		if body.Body == "" {
			writeErr(w, http.StatusBadRequest, "body is required")
			return
		}
		m, err := st.PutMemory(store.Memory{
			SessionID: body.SessionID,
			Body:      body.Body,
			Kind:      strings.TrimSpace(body.Kind),
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, m)
	}
}

func handleSearchMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeErr(w, http.StatusBadRequest, "q is required")
			return
		}
		hits, err := st.SearchMemory(q)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if hits == nil {
			hits = []store.SearchHit{}
		}
		writeJSON(w, http.StatusOK, hits)
	}
}
