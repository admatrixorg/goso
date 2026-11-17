// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
					Image    *WhatsAppMedia `json:"image"`
					Document *WhatsAppMedia `json:"document"`
					Audio    *WhatsAppMedia `json:"audio"`
					Video    *WhatsAppMedia `json:"video"`
					Sticker  *WhatsAppMedia `json:"sticker"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// WhatsAppMedia is the media object of an image, document, audio, video or
// sticker message. The bytes are fetched from the Graph API by id.
type WhatsAppMedia struct {
	ID       string `json:"id"`
	MIMEType string `json:"mime_type"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Voice    bool   `json:"voice"`
}

// whatsappAttachment maps one media object; ok is false when there is none.
func whatsappAttachment(kind string, m *WhatsAppMedia) (Attachment, bool) {
	if m == nil || strings.TrimSpace(m.ID) == "" {
		return Attachment{}, false
	}
	if kind == MediaAudio && m.Voice {
		kind = MediaVoice
	}
	if kind == MediaAudio && strings.Contains(strings.ToLower(m.MIMEType), "ogg") {
		kind = MediaVoice
	}
	return Attachment{Kind: kind, Name: m.Filename, MIME: m.MIMEType, Ref: m.ID}, true
}

func (wa *WhatsApp) token() string {
	if wa.AccessToken != "" {
		return wa.AccessToken
	}
	return os.Getenv("GOSO_WHATSAPP_ACCESS_TOKEN")
}

func (wa *WhatsApp) apiBase() string {
	if wa.APIBase != "" {
		return strings.TrimRight(wa.APIBase, "/")
	}
	return "https://graph.facebook.com/v21.0"
}

// fetchMedia resolves a media id to its short-lived download URL, then
// downloads it. Both calls carry the access token, so the download URL must
// be https (or on the configured API host) and the token is dropped on
// redirect. Errors never include either URL.
func (wa *WhatsApp) fetchMedia(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
	tok := wa.token()
	if tok == "" {
		return nil, errors.New("download failed: missing access token")
	}
	if strings.ContainsAny(a.Ref, "/?#") {
		return nil, errMediaNoRef
	}
	base := wa.apiBase()
	meta, err := fetchURL(ctx, wa.HTTPClient, base+"/"+url.PathEscape(a.Ref), 64<<10, "Authorization", "Bearer "+tok)
	if err != nil {
		return nil, err
	}
	var info struct {
		URL      string `json:"url"`
		FileSize int64  `json:"file_size"`
	}
	if json.Unmarshal(meta, &info) != nil || info.URL == "" {
		return nil, errMediaFetch
	}
	if info.FileSize > limit {
		return nil, errMediaTooLarge
	}
	u, err := url.Parse(info.URL)
	if err != nil {
		return nil, errMediaNoRef
	}
	if b, berr := url.Parse(base); u.Scheme != "https" && (berr != nil || u.Host != b.Host) {
		return nil, errors.New("download blocked")
	}
	return fetchURL(ctx, wa.HTTPClient, info.URL, limit, "Authorization", "Bearer "+tok)
}

func (wa *WhatsApp) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd WhatsAppUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	var from, text, pnid string
	var atts []Attachment
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
				for _, it := range []struct {
					kind string
					m    *WhatsAppMedia
				}{{MediaImage, m.Image}, {MediaDocument, m.Document}, {MediaAudio, m.Audio}, {MediaVideo, m.Video}, {MediaSticker, m.Sticker}} {
					if a, ok := whatsappAttachment(it.kind, it.m); ok {
						atts = append(atts, a)
						if text == "" && it.m.Caption != "" {
							text = it.m.Caption
						}
					}
				}
				if from != "" && (text != "" || len(atts) > 0) {
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
		"whatsapp", "WhatsApp", from, whatsappIdentity("whatsapp", from), text, atts, wa.fetchMedia)
}

// whatsappJIDServer is the WhatsApp user server GoClaw's ids carry.
const whatsappJIDServer = "@s.whatsapp.net"

// whatsappIdentity is GoClaw's identity of a WhatsApp direct message. GoClaw
// keys a person by their JID "<number>@s.whatsapp.net"; the Cloud API gives
// the bare number (wa_id), so it is turned into that JID.
func whatsappIdentity(name, waID string) Identity {
	jid := whatsappJID(waID)
	return Identity{Instance: name, Peer: "direct", SenderID: jid, ChatID: jid}
}

func whatsappJID(waID string) string {
	waID = strings.TrimPrefix(strings.TrimSpace(waID), "+")
	if waID == "" || strings.Contains(waID, "@") {
		return waID
	}
	return waID + whatsappJIDServer
}

// whatsappNumber is the Cloud API recipient for a GoClaw JID or a number.
func whatsappNumber(chat string) string {
	chat = strings.TrimSpace(chat)
	if i := strings.IndexByte(chat, '@'); i >= 0 {
		chat = chat[:i]
	}
	return strings.TrimPrefix(chat, "+")
}

func (wa *WhatsApp) sendMessage(ctx context.Context, to, text string) error {
	token := wa.token()
	if token == "" {
		return fmt.Errorf("whatsapp: missing access token")
	}
	return whatsappSend(ctx, wa.HTTPClient, wa.apiBase(), token, wa.PhoneNumberID, to, text)
}
