// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Identity is who wrote one channel message and where, in the shape GoClaw
// keys its data by. Every channel fills one, so goso's session and user keys
// line up with what was imported from GoClaw:
//
//   - user key (user_context_files.user_id and other per-user data):
//     the sender id in a direct chat; "group:<instance>:<chat>" in a group;
//     "guild:<guild>:user:<sender>" in a Discord server channel.
//   - session key: "agent:<agent>:<instance>:<peer>:<chat>", with
//     ":topic:<n>" for a Telegram forum topic and ":thread:<id>" for a
//     Telegram private-chat thread or a Slack thread.
//
// <instance> is GoClaw's channel instance name: the last segment of goso's
// "<tenant>/<catalog>/<name>", or the catalog name for an older single bot.
type Identity struct {
	// Instance is goso's channel name.
	Instance string
	// Peer is "direct" or "group" as GoClaw labels the chat; it can differ
	// from the peer goso's policy uses (a Zalo Bot group is "direct" here).
	Peer     string
	SenderID string
	// ChatID is GoClaw's chat id. A Lark topic in topic_session_mode is
	// "<chat_id>:topic:<root_id>".
	ChatID string
	// GuildID is the Discord server of a server channel.
	GuildID string
	// TopicID is a Telegram forum topic (1 is General); it splits the session
	// of a group, not its user key.
	TopicID int
	// DMThreadID is a Telegram private-chat thread.
	DMThreadID int
	// ThreadKey is a Slack thread_ts.
	ThreadKey string
	// oldLabel is a label goso used for this chat before it followed GoClaw's
	// chat id; an existing session under it is kept.
	oldLabel string
}

func (id Identity) peer() string {
	if id.Peer == "group" {
		return "group"
	}
	return "direct"
}

// UserKey is the per-user key for this message, "" when nothing valid is
// known (store.UserKeyOK).
func (id Identity) UserKey() string {
	sender := strings.TrimSpace(id.SenderID)
	// GoClaw keeps only the id part of a "id|username" sender.
	if i := strings.IndexByte(sender, '|'); i >= 0 {
		sender = sender[:i]
	}
	chat := strings.TrimSpace(id.ChatID)
	key := sender
	if id.peer() == "group" {
		// A group without a chat id never falls back to one member's key.
		if chat == "" {
			return ""
		}
		if g := strings.TrimSpace(id.GuildID); g != "" && sender != "" {
			key = "guild:" + g + ":user:" + sender
		} else {
			key = "group:" + goclawInstanceName(id.Instance) + ":" + chat
		}
	}
	if !store.UserKeyOK(key) {
		return ""
	}
	return key
}

// suffix is the topic or thread part shared by the session key and the goso
// label, "" for a plain chat.
func (id Identity) suffix() string {
	switch {
	case id.TopicID > 0 && id.peer() == "group":
		return ":topic:" + strconv.Itoa(id.TopicID)
	case id.DMThreadID > 0 && id.peer() == "direct":
		return ":thread:" + strconv.Itoa(id.DMThreadID)
	case strings.TrimSpace(id.ThreadKey) != "":
		return ":thread:" + strings.TrimSpace(id.ThreadKey)
	}
	return ""
}

// SessionKey is GoClaw's session key for this message under agentKey.
func (id Identity) SessionKey(agentKey string) string {
	return "agent:" + agentKey + ":" + goclawInstanceName(id.Instance) + ":" + id.peer() + ":" + strings.TrimSpace(id.ChatID) + id.suffix()
}

// Label is goso's own session label, "<instance>:<chat>" plus the topic or
// thread part. The message tool reads it back (instanceOfLabel).
func (id Identity) Label() string {
	return strings.TrimSpace(id.Instance) + ":" + strings.TrimSpace(id.ChatID) + id.suffix()
}

// goclawAgentKeys are the agent keys GoClaw may have used for agent: the
// key itself and, for an agent renamed "<key>--<tenant>" on import, <key>.
func goclawAgentKeys(agent *store.Agent) []string {
	keys := []string{agent.AgentKey}
	if i := strings.LastIndex(agent.AgentKey, "--"); i > 0 {
		keys = append(keys, agent.AgentKey[:i])
	}
	return keys
}

// ensureChannelSession finds or creates the agent's session for a channel
// message. A session imported from GoClaw (labelled with its GoClaw session
// key) wins, then goso's own label; a new session gets goso's label.
func ensureChannelSession(st store.StoreIface, agent *store.Agent, id Identity) *store.Session {
	if st == nil || agent == nil {
		return nil
	}
	want := map[string]bool{}
	for _, k := range goclawAgentKeys(agent) {
		if k != "" {
			want[id.SessionKey(k)] = true
		}
	}
	label := id.Label()
	var own, old *store.Session
	for _, s := range st.ListSessions() {
		if s == nil || s.AgentID != agent.ID {
			continue
		}
		if want[s.Label] {
			return s
		}
		if own == nil && s.Label == label {
			own = s
		}
		if old == nil && id.oldLabel != "" && s.Label == id.oldLabel {
			old = s
		}
	}
	if own != nil {
		return own
	}
	if old != nil {
		return old
	}
	sess, _ := st.CreateSession(store.Session{AgentID: agent.ID, TenantID: agent.TenantID, Label: label})
	return sess
}

// parseGoclawSessionKey splits a GoClaw channel session key
// "agent:<agent>:<instance>:<peer>:<chat>[:topic:<n>|:thread:<id>]" into the
// instance name and the chat (with its topic or thread part). ok is false
// for any other label.
func parseGoclawSessionKey(key string) (instance, chat string, ok bool) {
	parts := strings.SplitN(key, ":", 5)
	if len(parts) != 5 || parts[0] != "agent" || parts[2] == "" || parts[4] == "" {
		return "", "", false
	}
	if parts[3] != "direct" && parts[3] != "group" {
		return "", "", false
	}
	return parts[2], parts[4], true
}
