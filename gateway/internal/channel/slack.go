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

// Slack handles a minimal Slack Events API message fixture.
type Slack struct {
	BotToken   string
	Store      store.StoreIface
	LLM        llm.Provider
	Meter      *billing.Store
	Sender     func(ctx context.Context, channelID, text string) error
	HTTPClient *http.Client
	APIBase    string
}

func (s *Slack) Name() string { return "slack" }

// SlackUpdate is the inbound fixture:
//
//	{"event":{"type":"message","channel":"C1","text":"hello"}}
type SlackUpdate struct {
	Event *struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		Text    string `json:"text"`
	} `json:"event"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func (s *Slack) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd SlackUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	channelID, text := upd.Channel, upd.Text
	if upd.Event != nil {
		if upd.Event.Channel != "" {
			channelID = upd.Event.Channel
		}
		if upd.Event.Text != "" {
			text = upd.Event.Text
		}
	}
	sender := s.Sender
	if sender == nil {
		sender = s.sendMessage
	}
	replyInbound(w, r, inboundDeps{Store: s.Store, LLM: s.LLM, Meter: s.Meter, Sender: sender},
		"slack", "Slack", channelID, text)
}

func (s *Slack) sendMessage(ctx context.Context, channelID, text string) error {
	token := s.BotToken
	if token == "" {
		token = os.Getenv("GOSO_SLACK_BOT_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("slack: missing bot token")
	}
	base := s.APIBase
	if base == "" {
		base = "https://slack.com/api"
	}
	url := strings.TrimRight(base, "/") + "/chat.postMessage"
	return postJSON(ctx, s.HTTPClient, url, "Authorization", "Bearer "+token, map[string]any{
		"channel": channelID,
		"text":    text,
	})
}
