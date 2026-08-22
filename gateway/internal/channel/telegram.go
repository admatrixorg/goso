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

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Telegram handles Telegram webhook updates.
type Telegram struct {
	BotToken string
	Store    store.StoreIface
	LLM      llm.Provider
	Meter    *billing.Store
	// Sender can be overridden in tests to capture sendMessage calls.
	Sender func(ctx context.Context, chatID int64, text string) error
}

// Name returns channel name.
func (t *Telegram) Name() string { return "telegram" }

// TelegramUpdate is a minimal Telegram update payload.
type TelegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

// HandleUpdate handles POST /api/channels/telegram/webhook.
func (t *Telegram) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd TelegramUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if upd.Message == nil || upd.Message.Text == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	text := upd.Message.Text
	chatID := upd.Message.Chat.ID

	// Find or create session keyed by telegram chat_id.
	// Use a synthetic agent for telegram.
	agent := t.ensureAgent()
	sess := t.ensureSession(agent.ID, chatID)

	// Persist user message.
	_, _ = t.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: text})

	// Call LLM.
	provider := t.LLM
	if provider == nil {
		provider = llm.Echo{}
	}
	// Build history for LLM.
	history, _ := t.Store.ListMessages(sess.ID)
	var msgs []llm.Message
	for _, m := range history {
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	reply, usage, err := llm.ChatUsage(r.Context(), provider, msgs)
	if err != nil {
		reply = fmt.Sprintf("LLM error: %v", err)
	} else {
		trackUsage(t.Meter, agent.ID, provider.Name(), usage)
	}
	_, _ = t.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "assistant", Content: reply})

	// Send reply to Telegram.
	sender := t.Sender
	if sender == nil {
		sender = t.sendMessage
	}
	if err := sender(r.Context(), chatID, reply); err != nil {
		// still return 200 to avoid Telegram retry storm; log via error body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (t *Telegram) ensureAgent() *store.Agent {
	// Find existing telegram agent or create one.
	for _, a := range t.Store.ListAgents() {
		if a.AgentKey == "telegram" {
			return a
		}
	}
	a, _ := t.Store.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	return a
}

func (t *Telegram) ensureSession(agentID string, chatID int64) *store.Session {
	label := fmt.Sprintf("telegram:%d", chatID)
	for _, s := range t.Store.ListSessions() {
		if s.AgentID == agentID && s.Label == label {
			return s
		}
	}
	sess, _ := t.Store.CreateSession(store.Session{AgentID: agentID, Label: label})
	return sess
}

func (t *Telegram) sendMessage(ctx context.Context, chatID int64, text string) error {
	token := t.BotToken
	if token == "" {
		token = os.Getenv("GOSO_TELEGRAM_BOT_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("telegram: missing bot token")
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(body))
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
		return fmt.Errorf("telegram sendMessage %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
