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
		Type        string      `json:"type"`
		Channel     string      `json:"channel"`
		ChannelType string      `json:"channel_type"`
		User        string      `json:"user"`
		BotID       string      `json:"bot_id"`
		Text        string      `json:"text"`
		ThreadTS    string      `json:"thread_ts"`
		Files       []SlackFile `json:"files"`
	} `json:"event"`
	Channel string      `json:"channel"`
	Text    string      `json:"text"`
	Files   []SlackFile `json:"files"`
}

// SlackFile is one file shared in a Slack message.
type SlackFile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Mimetype           string `json:"mimetype"`
	Size               int64  `json:"size"`
	URLPrivate         string `json:"url_private"`
	URLPrivateDownload string `json:"url_private_download"`
}

// slackAttachments maps shared files. Slack serves them from private URLs
// that need the bot token.
func slackAttachments(files []SlackFile) []Attachment {
	var out []Attachment
	for _, f := range files {
		ref := f.URLPrivateDownload
		if ref == "" {
			ref = f.URLPrivate
		}
		if strings.TrimSpace(ref) == "" {
			continue
		}
		out = append(out, Attachment{Kind: mediaKindFromMIME(f.Mimetype), Name: f.Name, MIME: f.Mimetype, Size: f.Size, Ref: ref})
	}
	return out
}

// slackFileHost reports a Slack file host: the bot token is only ever sent
// to these, over https.
func slackFileHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".slack.com", ".slack-edge.com", ".slack-files.com"} {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

func (s *Slack) token() string {
	if s.BotToken != "" {
		return s.BotToken
	}
	return os.Getenv("GOSO_SLACK_BOT_TOKEN")
}

// fetchFile downloads a private Slack file with the bot token. The token
// goes only to Slack's file hosts and is dropped on redirect.
func (s *Slack) fetchFile(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
	if !slackFileHost(a.Ref) {
		return nil, errors.New("download blocked: not a Slack file host")
	}
	tok := s.token()
	if tok == "" {
		return nil, errors.New("download failed: missing bot token")
	}
	return fetchURL(ctx, s.HTTPClient, a.Ref, limit, "Authorization", "Bearer "+tok)
}

// slackIdentity is GoClaw's identity of a Slack event: the user as sender,
// the channel as the chat, a direct chat only for a "message" event in an
// "im" channel (an app_mention is always in a group), and a session per
// thread when the message is in one.
func slackIdentity(name, eventType, channelType, user, channel, threadTS string) Identity {
	id := Identity{Instance: name, Peer: "group", SenderID: user, ChatID: channel, ThreadKey: threadTS}
	if eventType != "app_mention" && channelType == "im" {
		id.Peer = "direct"
	}
	return id
}

func (s *Slack) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var upd SlackUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	channelID, text := upd.Channel, upd.Text
	files := upd.Files
	var evType, chType, user, thread string
	if upd.Event != nil {
		if upd.Event.BotID != "" {
			// A bot's message (this bot's own replies included) is never answered.
			writeOK(w)
			return
		}
		if upd.Event.Channel != "" {
			channelID = upd.Event.Channel
		}
		if upd.Event.Text != "" {
			text = upd.Event.Text
		}
		if len(upd.Event.Files) > 0 {
			files = upd.Event.Files
		}
		evType, chType, user, thread = upd.Event.Type, upd.Event.ChannelType, upd.Event.User, upd.Event.ThreadTS
	}
	sender := s.Sender
	if sender == nil {
		sender = s.sendMessage
	}
	replyInbound(w, r, inboundDeps{Store: s.Store, LLM: s.LLM, Meter: s.Meter, Sender: sender},
		"slack", "Slack", channelID, slackIdentity("slack", evType, chType, user, channelID, thread), text, slackAttachments(files), s.fetchFile)
}

func (s *Slack) sendMessage(ctx context.Context, channelID, text string) error {
	token := s.token()
	if token == "" {
		return fmt.Errorf("slack: missing bot token")
	}
	return slackSend(ctx, s.HTTPClient, s.APIBase, token, channelID, slackThreadFrom(ctx), text)
}
