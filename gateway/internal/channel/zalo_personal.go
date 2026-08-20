// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ZaloPersonal handles Zalo Personal (profile) webhook.
type ZaloPersonal struct {
	Store  store.StoreIface
	LLM    llm.Provider
	Sender func(ctx context.Context, threadID string, text string) error
}

func (z *ZaloPersonal) Name() string { return "zalo-personal" }

// ZaloPersonalUpdate is the personal webhook payload.
type ZaloPersonalUpdate struct {
	ThreadID string `json:"thread_id"`
	FromID   string `json:"from_id"`
	Message  *struct {
		Text string `json:"text"`
	} `json:"message"`
}

func (z *ZaloPersonal) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd ZaloPersonalUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	threadID := upd.ThreadID
	if threadID == "" {
		threadID = upd.FromID
	}
	var text string
	if upd.Message != nil {
		text = upd.Message.Text
	}
	if threadID == "" || text == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}

	agent := z.ensureAgent()
	sess := z.ensureSession(agent.ID, threadID)

	_, _ = z.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: text})

	provider := z.LLM
	if provider == nil {
		provider = llm.Echo{}
	}
	history, _ := z.Store.ListMessages(sess.ID)
	var msgs []llm.Message
	for _, m := range history {
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	reply, err := provider.Chat(r.Context(), msgs)
	if err != nil {
		reply = fmt.Sprintf("LLM error: %v", err)
	}
	_, _ = z.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "assistant", Content: reply})

	sender := z.Sender
	if sender == nil {
		sender = z.sendMessage
	}
	if err := sender(r.Context(), threadID, reply); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (z *ZaloPersonal) ensureAgent() *store.Agent {
	for _, a := range z.Store.ListAgents() {
		if a.AgentKey == "zalo-personal" {
			return a
		}
	}
	a, _ := z.Store.CreateAgent(store.Agent{AgentKey: "zalo-personal", DisplayName: "Zalo Personal"})
	return a
}

func (z *ZaloPersonal) ensureSession(agentID, threadID string) *store.Session {
	label := "zalo-personal:" + threadID
	for _, s := range z.Store.ListSessions() {
		if s.AgentID == agentID && s.Label == label {
			return s
		}
	}
	sess, _ := z.Store.CreateSession(store.Session{AgentID: agentID, Label: label})
	return sess
}

func (z *ZaloPersonal) sendMessage(ctx context.Context, threadID, text string) error {
	token := os.Getenv("GOSO_ZALO_PERSONAL_TOKEN")
	if token == "" {
		return fmt.Errorf("zalo-personal: missing token (GOSO_ZALO_PERSONAL_TOKEN)")
	}
	_ = token
	body, _ := json.Marshal(map[string]any{"thread_id": threadID, "text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.zalo.example/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("zalo personal send %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
