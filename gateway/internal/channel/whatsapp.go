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

// WhatsApp is a Cloud-API-shaped webhook stub (not a native stack).
// Native vs Business protocol = DI-01 — this adapter is Business Cloud API only.
type WhatsApp struct {
	AccessToken   string
	PhoneNumberID string
	Store         store.StoreIface
	LLM           llm.Provider
	Meter         *billing.Store
	Sender        func(ctx context.Context, to, text string) error
	HTTPClient    *http.Client
	APIBase       string
}

func (w *WhatsApp) Name() string { return "whatsapp" }

// WhatsAppUpdate is a Cloud API inbound fixture:
//
//	{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{
//	  "metadata":{"phone_number_id":"pn1"},
//	  "messages":[{"from":"1555","type":"text","text":{"body":"hello"}}]
//	}}]}]}
type WhatsAppUpdate struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					From string `json:"from"`
					Type string `json:"type"`
					Text *struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func (wa *WhatsApp) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd WhatsAppUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	var from, text, pnid string
	for _, e := range upd.Entry {
		for _, ch := range e.Changes {
			if ch.Value.Metadata.PhoneNumberID != "" {
				pnid = ch.Value.Metadata.PhoneNumberID
			}
			for _, m := range ch.Value.Messages {
				if m.From != "" {
					from = m.From
				}
				if m.Text != nil {
					text = m.Text.Body
				}
				if from != "" && text != "" {
					break
				}
			}
		}
	}
	if pnid != "" && wa.PhoneNumberID == "" {
		wa.PhoneNumberID = pnid
	}
	sender := wa.Sender
	if sender == nil {
		sender = wa.sendMessage
	}
	replyInbound(w, r, inboundDeps{Store: wa.Store, LLM: wa.LLM, Meter: wa.Meter, Sender: sender},
		"whatsapp", "WhatsApp", from, text)
}

func (wa *WhatsApp) sendMessage(ctx context.Context, to, text string) error {
	token := wa.AccessToken
	if token == "" {
		token = os.Getenv("GOSO_WHATSAPP_ACCESS_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("whatsapp: missing access token")
	}
	base := wa.APIBase
	if base == "" {
		base = "https://graph.facebook.com/v21.0"
	}
	pnid := wa.PhoneNumberID
	if pnid == "" {
		pnid = "me"
	}
	url := strings.TrimRight(base, "/") + "/" + pnid + "/messages"
	return postJSON(ctx, wa.HTTPClient, url, "Authorization", "Bearer "+token, map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})
}
