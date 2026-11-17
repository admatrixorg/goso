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

// Discord handles a minimal Discord MESSAGE_CREATE-shaped webhook.
type Discord struct {
	BotToken   string
	Store      store.StoreIface
	LLM        llm.Provider
	Meter      *billing.Store
	Sender     func(ctx context.Context, channelID, text string) error
	HTTPClient *http.Client
	APIBase    string
}

func (d *Discord) Name() string { return "discord" }

// DiscordUpdate is the inbound fixture:
//
//	{"channel_id":"c1","content":"hello"}
//
// or gateway wrap {"d":{"channel_id":"c1","content":"hello"}}.
type DiscordUpdate struct {
	discordMessage
	D *discordMessage `json:"d"`
}

// discordMessage is the part of a MESSAGE_CREATE goso reads.
type discordMessage struct {
	ChannelID   string              `json:"channel_id"`
	GuildID     string              `json:"guild_id"`
	Content     string              `json:"content"`
	Attachments []DiscordAttachment `json:"attachments"`
	Author      *discordAuthor      `json:"author"`
}

type discordAuthor struct {
	ID  string `json:"id"`
	Bot bool   `json:"bot"`
}

// discordIdentity is GoClaw's identity of a Discord message: the author's
// id, the channel as the chat, a group when it is in a server (guild), whose
// members each get a "guild:<guild>:user:<author>" user key.
func discordIdentity(name string, m discordMessage) Identity {
	id := Identity{Instance: name, Peer: "direct", ChatID: m.ChannelID, GuildID: m.GuildID}
	if m.Author != nil {
		id.SenderID = m.Author.ID
	}
	if m.GuildID != "" {
		id.Peer = "group"
	}
	return id
}

// DiscordAttachment is one file of a Discord message.
type DiscordAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}

// discordAttachments maps a message's files; the kind follows the content
// type (image, audio, video, else document).
func discordAttachments(list []DiscordAttachment) []Attachment {
	var out []Attachment
	for _, a := range list {
		if strings.TrimSpace(a.URL) == "" {
			continue
		}
		out = append(out, Attachment{Kind: mediaKindFromMIME(a.ContentType), Name: a.Filename, MIME: a.ContentType, Size: a.Size, Ref: a.URL})
	}
	return out
}

// fetchAttachment downloads a Discord CDN attachment (a signed public URL).
func (d *Discord) fetchAttachment(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
	return fetchURL(ctx, d.HTTPClient, a.Ref, limit, "", "")
}

func (d *Discord) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd DiscordUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	m := upd.discordMessage
	if upd.D != nil {
		if upd.D.ChannelID != "" {
			m.ChannelID = upd.D.ChannelID
		}
		if upd.D.Content != "" {
			m.Content = upd.D.Content
		}
		if len(upd.D.Attachments) > 0 {
			m.Attachments = upd.D.Attachments
		}
		if upd.D.GuildID != "" {
			m.GuildID = upd.D.GuildID
		}
		if upd.D.Author != nil {
			m.Author = upd.D.Author
		}
	}
	// A bot's message (this bot's own replies included) is never answered.
	if m.Author != nil && m.Author.Bot {
		writeOK(w)
		return
	}
	sender := d.Sender
	if sender == nil {
		sender = d.sendMessage
	}
	replyInbound(w, r, inboundDeps{Store: d.Store, LLM: d.LLM, Meter: d.Meter, Sender: sender},
		"discord", "Discord", m.ChannelID, discordIdentity("discord", m), m.Content, discordAttachments(m.Attachments), d.fetchAttachment)
}

func (d *Discord) sendMessage(ctx context.Context, channelID, text string) error {
	token := d.BotToken
	if token == "" {
		token = os.Getenv("GOSO_DISCORD_BOT_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("discord: missing bot token")
	}
	return discordSend(ctx, d.HTTPClient, d.APIBase, token, channelID, text)
}
