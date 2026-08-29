// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const memorySnippetWindow = 80

type memoryListRow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	SessionID string    `json:"session_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	Kind      string    `json:"kind"`
	Snippet   string    `json:"snippet"`
	CreatedAt time.Time `json:"created_at"`
}

type memoryDetail struct {
	memoryListRow
	Body string `json:"body"`
}

func handleMemoryIndex(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fts := st.HasMemoryFTS()
		search := "substring"
		if fts {
			search = "fts5"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"search":               search,
			"fts":                  fts,
			"embedding":            "not_configured",
			"embedding_configured": false,
		})
	}
}

func handleListMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := strings.TrimSpace(r.URL.Query().Get("session_id"))
		aid := strings.TrimSpace(r.URL.Query().Get("agent_id"))
		kind := strings.TrimSpace(r.URL.Query().Get("kind"))
		tid := requestTenant(r)
		if sid != "" {
			if _, err := sessionVisible(st, sid, tid); err != nil {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
		}
		if aid != "" {
			if _, err := agentVisible(st, aid, tid); err != nil {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
		}
		if kind != "" {
			kind = store.NormalizeMemoryKind(kind)
			if kind == store.KindMessage {
				writeErr(w, http.StatusBadRequest, "kind is reserved")
				return
			}
		}
		list, err := st.QueryMemories(store.MemoryQuery{SessionID: sid, AgentID: aid, Kind: kind})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		list = memoriesInTenant(list, tid)
		writeJSON(w, http.StatusOK, map[string]any{"memories": publicMemoryList(st, list)})
	}
}

func handleGetMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		m, err := st.GetMemory(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if !store.SameTenant(m.TenantID, requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if _, err := sessionVisible(st, m.SessionID, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, publicMemoryDetail(st, m))
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
		kind, ok := allowMemoryKind(body.Kind)
		if !ok {
			writeErr(w, http.StatusBadRequest, "kind is reserved")
			return
		}
		tid := requestTenant(r)
		if _, err := sessionVisible(st, body.SessionID, tid); err != nil {
			writeErr(w, http.StatusBadRequest, "session not found")
			return
		}
		m, err := st.PutMemory(store.Memory{
			TenantID:  tid,
			SessionID: body.SessionID,
			Body:      body.Body,
			Kind:      kind,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, publicMemoryDetail(st, m))
	}
}

func handlePatchMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		cur, err := st.GetMemory(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		tid := requestTenant(r)
		if !store.SameTenant(cur.TenantID, tid) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if _, err := sessionVisible(st, cur.SessionID, tid); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		var body struct {
			Body *string `json:"body"`
			Kind *string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		next := *cur
		if body.Body != nil {
			next.Body = strings.TrimSpace(*body.Body)
		}
		if body.Kind != nil {
			kind, ok := allowMemoryKind(*body.Kind)
			if !ok {
				writeErr(w, http.StatusBadRequest, "kind is reserved")
				return
			}
			next.Kind = kind
		}
		if strings.TrimSpace(next.Body) == "" {
			writeErr(w, http.StatusBadRequest, "body is required")
			return
		}
		m, err := st.UpdateMemory(next)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, publicMemoryDetail(st, m))
	}
}

func handleDeleteMemory(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		cur, err := st.GetMemory(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		tid := requestTenant(r)
		if !store.SameTenant(cur.TenantID, tid) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if _, err := sessionVisible(st, cur.SessionID, tid); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if err := st.DeleteMemory(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		tid := requestTenant(r)
		out := make([]store.SearchHit, 0, len(hits))
		for _, h := range hits {
			if _, err := sessionVisible(st, h.SessionID, tid); err == nil {
				out = append(out, h)
			}
		}
		hits = out
		if hits == nil {
			hits = []store.SearchHit{}
		}
		writeJSON(w, http.StatusOK, hits)
	}
}

func allowMemoryKind(raw string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(raw), store.KindMessage) {
		return "", false
	}
	kind := store.NormalizeMemoryKind(raw)
	if kind == store.KindMessage {
		return "", false
	}
	return kind, true
}

func publicMemoryList(st store.StoreIface, list []*store.Memory) []memoryListRow {
	out := make([]memoryListRow, 0, len(list))
	for _, m := range list {
		if m == nil {
			continue
		}
		out = append(out, memoryListRowOf(st, m))
	}
	return out
}

func publicMemoryDetail(st store.StoreIface, m *store.Memory) memoryDetail {
	return memoryDetail{memoryListRow: memoryListRowOf(st, m), Body: m.Body}
}

func memoryListRowOf(st store.StoreIface, m *store.Memory) memoryListRow {
	row := memoryListRow{
		ID:        m.ID,
		TenantID:  m.TenantID,
		SessionID: m.SessionID,
		Kind:      m.Kind,
		Snippet:   store.SnippetAround(m.Body, "", memorySnippetWindow),
		CreatedAt: m.CreatedAt,
	}
	if sess, err := st.GetSession(m.SessionID); err == nil && sess != nil {
		row.AgentID = sess.AgentID
	}
	return row
}
