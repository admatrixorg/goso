// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	ToolSessionsList    = "sessions_list"
	ToolSessionsHistory = "sessions_history"
	// SessionToolCap is the max sessions or messages returned by the tools.
	SessionToolCap = 50
)

type sessionListItem struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Label     string    `json:"label,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type sessionHistoryItem struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionToolSpecs are always advertised. LLM ToolCalls only (no keyword match).
func SessionToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name:        ToolSessionsList,
			Description: "List sessions in the calling agent's tenant. Read-only. Cap 50. No message bodies. No args.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		{
			Name:        ToolSessionsHistory,
			Description: "List messages for a session in the calling agent's tenant. Fail-closed if missing or other tenant. Cap 50. Args: session_id.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"}},"required":["session_id"]}`),
		},
	}
}

// DispatchSessionTool runs sessions_list / sessions_history jailed to tenant.
func DispatchSessionTool(st store.StoreIface, tenant string, call llm.ToolCall) (string, error) {
	if st == nil {
		return `{"error":"store required"}`, errors.New("store required")
	}
	name := strings.TrimSpace(call.Name)
	if c, t, ok := SplitAdvertised(name); ok && c != "" {
		name = t
	}
	tenant = store.NormalizeTenant(tenant)
	switch name {
	case ToolSessionsList:
		return dispatchSessionsList(st, tenant)
	case ToolSessionsHistory:
		sid := argString(call.Arguments, "session_id")
		if sid == "" {
			return `{"error":"session_id is required"}`, errors.New("session_id is required")
		}
		return dispatchSessionsHistory(st, tenant, sid)
	default:
		return `{"error":"unknown tool"}`, errors.New("unknown tool")
	}
}

func dispatchSessionsList(st store.StoreIface, tenant string) (string, error) {
	all := st.ListSessions()
	items := make([]sessionListItem, 0, len(all))
	for _, s := range all {
		if s == nil || !store.SameTenant(s.TenantID, tenant) {
			continue
		}
		items = append(items, sessionListItem{
			ID:        s.ID,
			AgentID:   s.AgentID,
			Label:     s.Label,
			CreatedAt: s.CreatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID > items[j].ID
	})
	if len(items) > SessionToolCap {
		items = items[:SessionToolCap]
	}
	b, err := json.Marshal(map[string]any{"sessions": items})
	if err != nil {
		return `{"error":"encode"}`, err
	}
	return string(b), nil
}

func dispatchSessionsHistory(st store.StoreIface, tenant, sessionID string) (string, error) {
	sess, err := st.GetSession(sessionID)
	if err != nil || sess == nil || !store.SameTenant(sess.TenantID, tenant) {
		return `{"error":"not found"}`, store.ErrNotFound
	}
	raw, err := st.ListMessages(sessionID)
	if err != nil {
		return `{"error":"not found"}`, store.ErrNotFound
	}
	if len(raw) > SessionToolCap {
		raw = raw[len(raw)-SessionToolCap:]
	}
	items := make([]sessionHistoryItem, 0, len(raw))
	for _, m := range raw {
		if m == nil {
			continue
		}
		items = append(items, sessionHistoryItem{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}
	b, err := json.Marshal(map[string]any{"session_id": sess.ID, "messages": items})
	if err != nil {
		return `{"error":"encode"}`, err
	}
	return string(b), nil
}
