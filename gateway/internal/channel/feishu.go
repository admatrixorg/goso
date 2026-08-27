// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Feishu handles a minimal Feishu/Lark im.message.receive_v1-shaped webhook.
type Feishu struct {
	AppSecret  string
	Store      store.StoreIface
	LLM        llm.Provider
	Meter      *billing.Store
	Sender     func(ctx context.Context, chatID, text string) error
	HTTPClient *http.Client
	APIBase    string
}

func (f *Feishu) Name() string { return "feishu" }

// FeishuUpdate is the inbound fixture:
//
//	{"event":{"message":{"chat_id":"oc1","content":"{\"text\":\"hello\"}"}}}
//
// or flat {"chat_id":"oc1","text":"hello"}.
type FeishuUpdate struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
	Event  *struct {
		Message *struct {
			ChatID  string `json:"chat_id"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"event"`
}

func (f *Feishu) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd FeishuUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	chatID, text := upd.ChatID, upd.Text
	if upd.Event != nil && upd.Event.Message != nil {
		if upd.Event.Message.ChatID != "" {
			chatID = upd.Event.Message.ChatID
		}
		if text == "" && upd.Event.Message.Content != "" {
			var inner struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(upd.Event.Message.Content), &inner); err == nil {
				text = inner.Text
			} else {
				text = upd.Event.Message.Content
			}
		}
	}
	sender := f.Sender
	if sender == nil {
		sender = f.sendMessage
	}
	replyInbound(w, r, inboundDeps{Store: f.Store, LLM: f.LLM, Meter: f.Meter, Sender: sender},
		"feishu", "Feishu", chatID, text)
}

func (f *Feishu) sendMessage(ctx context.Context, chatID, text string) error {
	secret := f.AppSecret
	if secret == "" {
		secret = os.Getenv("GOSO_FEISHU_APP_SECRET")
	}
	if secret == "" {
		return fmt.Errorf("feishu: missing app secret")
	}
	base := f.APIBase
	if base == "" {
		base = "https://open.feishu.cn/open-apis"
	}
	url := strings.TrimRight(base, "/") + "/im/v1/messages?receive_id_type=chat_id"
	content, _ := json.Marshal(map[string]string{"text": text})
	return postJSON(ctx, f.HTTPClient, url, "Authorization", "Bearer "+secret, map[string]any{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	})
}
