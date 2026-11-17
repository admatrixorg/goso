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

// ZaloPersonal handles Zalo Personal (profile) webhook.
type ZaloPersonal struct {
	Store  store.StoreIface
	LLM    llm.Provider
	Meter  *billing.Store
	Sender func(ctx context.Context, threadID string, text string) error
}

func (z *ZaloPersonal) Name() string { return "zalo-personal" }

// ZaloPersonalUpdate is the personal webhook payload.
type ZaloPersonalUpdate struct {
	ThreadID string `json:"thread_id"`
	FromID   string `json:"from_id"`
	// ThreadType is "group" (or 1) for a group thread; empty is a direct chat.
	ThreadType string `json:"thread_type"`
	Message    *struct {
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

	var cfg *store.ChannelConfig
	if z.Store != nil {
		cfg, _ = z.Store.GetChannelConfig("zalo-personal")
	}
	pol := MergePolicy("zalo-personal", cfg)
	from := upd.FromID
	if from == "" {
		from = threadID
	}
	in := Inbound{Channel: "zalo-personal", SenderID: from, ChatID: threadID, PeerKind: "direct", Text: text}
	paired := z.Store != nil && SenderPaired(z.Store, "zalo-personal", from, time.Time{})
	switch CheckPolicy("zalo-personal", pol, in, paired) {
	case PolicyReject, PolicyNeedMention, PolicyNeedPairing:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}

	agent := z.ensureAgent()
	id := Identity{Instance: "zalo-personal", Peer: peerKind(upd.ThreadType), SenderID: from, ChatID: threadID}
	sess := ensureChannelSession(z.Store, agent, id)

	ctx := withContextUser(r.Context(), id)
	reply := answerTurn(ctx, z.Store, agent, sess, z.LLM, z.Meter, text)

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
	if a := store.FindAgentByKey(z.Store, store.DefaultTenant, "zalo-personal"); a != nil {
		return a
	}
	a, _ := z.Store.CreateAgent(store.Agent{TenantID: store.DefaultTenant, AgentKey: "zalo-personal", DisplayName: "Zalo Personal"})
	return a
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

// ensureSession is the agent's session for a direct thread.
func (z *ZaloPersonal) ensureSession(agentID, threadID string) *store.Session {
	agent, err := z.Store.GetAgent(agentID)
	if err != nil {
		return nil
	}
	return ensureChannelSession(z.Store, agent, Identity{Instance: "zalo-personal", Peer: "direct", SenderID: threadID, ChatID: threadID})
}
