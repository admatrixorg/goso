// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleGoClawChat(ctx context.Context, conn *websocket.Conn, st store.StoreIface, provider llm.Provider, req goclawReq) error {
	switch req.Method {
	case "chat.send":
		return goclawChatSend(ctx, conn, st, provider, req)
	case "chat.history":
		return goclawChatHistory(conn, st, req)
	case "chat.abort":
		return writeGoClaw(conn, req.ID, map[string]any{"cancelled": true, "aborted": false})
	case "chat.session.status":
		return writeGoClaw(conn, req.ID, map[string]any{"isRunning": false})
	case "sessions.list":
		return goclawSessionsList(conn, st, req, wsScopeTenant(wsStateFromCtx(ctx)))
	case "sessions.delete":
		return goclawSessionsDelete(conn, st, req)
	case "teams.tasks.active-by-session":
		return writeGoClaw(conn, req.ID, map[string]any{"tasks": []any{}})
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}

func writeGoClawEvent(conn *websocket.Conn, event string, payload any) error {
	return conn.WriteJSON(map[string]any{"type": "event", "event": event, "payload": payload})
}

func emitAgentEvent(conn *websocket.Conn, typ, agentKey, runID, sessionKey string, payload any) error {
	body := map[string]any{
		"type":       typ,
		"agentId":    agentKey,
		"runId":      runID,
		"sessionKey": sessionKey,
		"channel":    "ws",
	}
	if payload != nil {
		body["payload"] = payload
	}
	return writeGoClawEvent(conn, "agent", body)
}

func findAgentByIDOrKey(st store.StoreIface, idOrKey, tid string) *store.Agent {
	idOrKey = strings.TrimSpace(idOrKey)
	if idOrKey == "" || st == nil {
		return nil
	}
	if a, err := agentVisible(st, idOrKey, tid); err == nil && a != nil {
		return a
	}
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == idOrKey && store.SameTenant(a.TenantID, tid) {
			return a
		}
	}
	return nil
}

func sessionByLabel(st store.StoreIface, label string) *store.Session {
	label = strings.TrimSpace(label)
	if label == "" || st == nil {
		return nil
	}
	for _, s := range st.ListSessions() {
		if s != nil && s.Label == label {
			return s
		}
	}
	return nil
}

func ensureWSSession(st store.StoreIface, agent *store.Agent, sessionKey string) (*store.Session, error) {
	if sess := sessionByLabel(st, sessionKey); sess != nil {
		return sess, nil
	}
	return st.CreateSession(store.Session{
		TenantID: agent.TenantID,
		AgentID:  agent.ID,
		Label:    sessionKey,
	})
}

func goclawChatSend(ctx context.Context, conn *websocket.Conn, st store.StoreIface, provider llm.Provider, req goclawReq) error {
	var p struct {
		Message    string `json:"message"`
		AgentID    string `json:"agentId"`
		SessionKey string `json:"sessionKey"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &p)
	}
	p.Message = strings.TrimSpace(p.Message)
	p.AgentID = strings.TrimSpace(p.AgentID)
	p.SessionKey = strings.TrimSpace(p.SessionKey)
	if p.Message == "" {
		return writeGoClawErr(conn, req.ID, "INVALID_REQUEST", "message is required")
	}
	if p.SessionKey == "" {
		return writeGoClawErr(conn, req.ID, "INVALID_REQUEST", "sessionKey is required")
	}
	if p.AgentID == "" {
		if parts := strings.Split(p.SessionKey, ":"); len(parts) >= 2 && parts[0] == "agent" {
			p.AgentID = parts[1]
		}
	}
	agent := findAgentByIDOrKey(st, p.AgentID, wsIdentityTenant(wsStateFromCtx(ctx)))
	if agent == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "agent not found")
	}
	if !agent.Enabled {
		return writeGoClawErr(conn, req.ID, "INVALID_REQUEST", "agent is disabled")
	}
	sess, err := ensureWSSession(st, agent, p.SessionKey)
	if err != nil || sess == nil {
		msg := "session create failed"
		if err != nil {
			msg = err.Error()
		}
		return writeGoClawErr(conn, req.ID, "INTERNAL", msg)
	}
	if _, err := st.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: p.Message, AgentID: agent.ID}); err != nil {
		return writeGoClawErr(conn, req.ID, "INTERNAL", err.Error())
	}
	resolved, rerr := llm.Resolve(st, agent.LLMProvider, agent.Model, provider)
	if rerr != nil {
		return writeGoClawErr(conn, req.ID, "INVALID_REQUEST", rerr.Error())
	}
	history, _ := st.ListMessages(sess.ID)
	msgs := make([]llm.Message, 0, len(history)+1)
	if instr := strings.TrimSpace(agent.Instructions); instr != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: instr})
	}
	for _, m := range history {
		if m == nil {
			continue
		}
		if m.Role == "user" || m.Role == "assistant" {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	if err := emitAgentEvent(conn, "run.started", agent.AgentKey, runID, p.SessionKey, nil); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	chatCtx, cancel := context.WithTimeout(ctx, llm.ChatTimeout())
	defer cancel()
	reply, _, chatErr := llm.ChatStream(chatCtx, resolved, msgs, func(delta string) {
		if strings.TrimSpace(delta) == "" {
			return
		}
		_ = emitAgentEvent(conn, "chunk", agent.AgentKey, runID, p.SessionKey, map[string]any{"content": delta})
	})
	if chatErr != nil {
		_ = emitAgentEvent(conn, "run.failed", agent.AgentKey, runID, p.SessionKey, map[string]any{"error": chatErr.Error()})
		return writeGoClawErr(conn, req.ID, "LLM_ERROR", chatErr.Error())
	}
	if _, err := st.AddMessage(store.Message{SessionID: sess.ID, Role: "assistant", Content: reply, AgentID: agent.ID, Model: agent.Model, Provider: agent.LLMProvider}); err != nil {
		return writeGoClawErr(conn, req.ID, "INTERNAL", err.Error())
	}
	if err := emitAgentEvent(conn, "run.completed", agent.AgentKey, runID, p.SessionKey, nil); err != nil {
		return err
	}
	return writeGoClaw(conn, req.ID, map[string]any{"runId": runID, "content": reply})
}

func goclawChatHistory(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		SessionKey string `json:"sessionKey"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &p)
	}
	sess := sessionByLabel(st, strings.TrimSpace(p.SessionKey))
	out := make([]map[string]any, 0)
	if sess != nil {
		msgs, _ := st.ListMessages(sess.ID)
		for _, m := range msgs {
			if m == nil || (m.Role != "user" && m.Role != "assistant") {
				continue
			}
			out = append(out, map[string]any{
				"role":       m.Role,
				"content":    m.Content,
				"created_at": m.CreatedAt.Format(time.RFC3339Nano),
			})
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"messages": out})
}

func goclawSessionsList(conn *websocket.Conn, st store.StoreIface, req goclawReq, tid string) error {
	var p struct {
		AgentID string `json:"agentId"`
		Channel string `json:"channel"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &p)
	}
	agent := findAgentByIDOrKey(st, p.AgentID, tid)
	prefix := ""
	if agent != nil {
		prefix = "agent:" + agent.AgentKey + ":"
	} else if strings.TrimSpace(p.AgentID) != "" {
		prefix = "agent:" + strings.TrimSpace(p.AgentID) + ":"
	}
	wantWS := strings.TrimSpace(p.Channel) == "" || p.Channel == "ws"
	sessions := make([]map[string]any, 0)
	if st == nil {
		return writeGoClaw(conn, req.ID, map[string]any{"sessions": sessions})
	}
	for _, s := range st.ListSessions() {
		if s == nil || s.Label == "" {
			continue
		}
		if tid != "" && !store.SameTenant(s.TenantID, tid) {
			continue
		}
		if prefix != "" && !strings.HasPrefix(s.Label, prefix) {
			continue
		}
		if wantWS && !strings.Contains(s.Label, ":ws:") && !strings.HasPrefix(s.Label, "ws:") {
			continue
		}
		msgs, _ := st.ListMessages(s.ID)
		updated := s.CreatedAt
		count := 0
		preview := ""
		for _, m := range msgs {
			if m == nil {
				continue
			}
			if m.Role == "user" || m.Role == "assistant" {
				count++
				if m.CreatedAt.After(updated) {
					updated = m.CreatedAt
				}
				if preview == "" && m.Role == "user" {
					preview = strings.TrimSpace(m.Content)
				}
			}
		}
		if len(preview) > 80 {
			preview = preview[:80]
		}
		item := map[string]any{
			"key":          s.Label,
			"messageCount": count,
			"created":      s.CreatedAt.Format(time.RFC3339Nano),
			"updated":      updated.Format(time.RFC3339Nano),
			"channel":      "ws",
		}
		if preview != "" {
			item["label"] = preview
		}
		if agent != nil {
			item["agentName"] = agent.DisplayName
			item["model"] = agent.Model
			item["provider"] = agent.LLMProvider
		}
		sessions = append(sessions, item)
	}
	return writeGoClaw(conn, req.ID, map[string]any{"sessions": sessions})
}

func goclawSessionsDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		Key string `json:"key"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &p)
	}
	sess := sessionByLabel(st, strings.TrimSpace(p.Key))
	if sess == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "session not found")
	}
	if err := st.DeleteSession(sess.ID); err != nil {
		return writeGoClawErr(conn, req.ID, "INTERNAL", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}
