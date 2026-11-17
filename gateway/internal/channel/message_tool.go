// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// MessageTool is the builtin message tool (builtin.Messenger). An agent may
// write only to a channel instance "<tenant>/<type>/<name>" that:
//   - belongs to the agent's own tenant (another tenant's channel reads as
//     "not found", so its existence does not leak),
//   - is bound to the calling agent, enabled and running here,
//   - is a type goso can send on (Telegram, Zalo Bot, Lark, Zalo through
//     the ZaloCRM bridge, Discord, Slack, WhatsApp),
//
// and only to a chat that already has a session with that agent (goso's own
// or one imported from GoClaw), so the tool cannot cold-message arbitrary ids. A bare catalog channel ("telegram") has
// no tenant and is refused. The approval gate sits in front of every send.
type MessageTool struct {
	Store   store.StoreIface
	Manager *Manager
	// Bridge sends on bridged Zalo channels (zalo-personal).
	Bridge *ZCRMBridge
	// HTTPClient and APIBase (catalog → base URL) replace the Discord, Slack
	// and WhatsApp endpoints in tests.
	HTTPClient *http.Client
	APIBase    map[string]string
}

// MessageMaxRunes caps one message tool call.
const MessageMaxRunes = 4000

const telegramMaxText = 4096

// sendableCatalogs are the channel types the message tool can write to.
var sendableCatalogs = map[string]bool{
	"telegram": true, "zalo-oa": true, "feishu": true,
	"zalo-personal": true, "discord": true, "slack": true, "whatsapp": true,
}

// SettingPhoneNumberID is the WhatsApp Cloud API number a tenant's WhatsApp
// channel sends from.
const SettingPhoneNumberID = "phone_number_id"

type messageTarget struct {
	name    string
	catalog string
	chat    string
	text    string
	agent   *store.Agent
	session *store.Session
}

func refuse(msg string) *connector.InvokeResult {
	return &connector.InvokeResult{
		Tool:      builtin.ToolMessage,
		Connector: builtin.ConnectorName,
		Status:    "error",
		Content:   map[string]any{"error": msg},
	}
}

func stringArg(args map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := args[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case int64:
			return strconv.FormatInt(v, 10)
		case int:
			return strconv.Itoa(v)
		}
	}
	return ""
}

// instanceOfLabel returns the tenant channel instance and chat of a session
// label: goso's "<instance>:<chat>" (the longest configured instance name of
// that tenant that prefixes the label), or a GoClaw session key
// "agent:<agent>:<name>:<peer>:<chat>" imported from GoClaw, whose short
// <name> maps back to the tenant's instance of that name (the one bound to
// agentID when several catalogs share it).
func instanceOfLabel(st store.StoreIface, tenant, agentID, label string) (name, chat string) {
	names := InstanceNames(st, tenant)
	for _, n := range names {
		if strings.HasPrefix(label, n+":") && len(label) > len(n)+1 && len(n) > len(name) {
			name, chat = n, label[len(n)+1:]
		}
	}
	if name != "" {
		return name, chat
	}
	short, c, ok := parseGoclawSessionKey(label)
	if !ok {
		return "", ""
	}
	for _, n := range names {
		if _, _, inst, ok := ParseInstance(n); !ok || inst != short {
			continue
		}
		if name == "" {
			name = n
			continue
		}
		if cfg, err := st.GetChannelConfig(n); err == nil && cfg != nil && strings.TrimSpace(cfg.AgentID) == agentID {
			name = n
		}
	}
	if name == "" {
		return "", ""
	}
	return name, c
}

// Resolve implements builtin.Messenger.
func (m *MessageTool) Resolve(ctx context.Context, args map[string]any) (map[string]any, *connector.InvokeResult) {
	tg, bad := m.resolve(ctx, args)
	if bad != nil {
		return args, bad
	}
	return map[string]any{"text": tg.text, "channel": tg.name, "chat_id": tg.chat}, nil
}

func (m *MessageTool) resolve(ctx context.Context, args map[string]any) (*messageTarget, *connector.InvokeResult) {
	if m == nil || m.Store == nil {
		return nil, refuse("not_configured")
	}
	caller := builtin.CallerFrom(ctx)
	if caller.AgentID == "" {
		return nil, refuse("message needs a calling agent")
	}
	agent, err := m.Store.GetAgent(caller.AgentID)
	if err != nil || agent == nil {
		return nil, refuse("message needs a calling agent")
	}
	tenant := store.NormalizeTenant(agent.TenantID)
	if skill.HasTenant(ctx) && !store.SameTenant(skill.TenantFrom(ctx), tenant) {
		return nil, refuse("message needs a calling agent")
	}
	text := stringArg(args, "text", "message")
	if text == "" {
		return nil, refuse("text is required")
	}
	if len([]rune(text)) > MessageMaxRunes {
		return nil, refuse(fmt.Sprintf("text is longer than %d characters", MessageMaxRunes))
	}
	name := stringArg(args, "channel")
	chat := stringArg(args, "chat_id", "to")
	if name == "" || chat == "" {
		// Default: the conversation this call came from.
		var curName, curChat string
		if caller.SessionID != "" {
			if sess, err := m.Store.GetSession(caller.SessionID); err == nil && sess != nil && store.SameTenant(sess.TenantID, tenant) {
				curName, curChat = instanceOfLabel(m.Store, tenant, agent.ID, sess.Label)
			}
		}
		switch {
		case name == "" && chat == "":
			name, chat = curName, curChat
		case name == "" && chat != "":
			name = curName
		case name == curName && chat == "":
			chat = curChat
		}
		if name == "" || chat == "" {
			return nil, refuse("channel and chat_id are required: this conversation is not on a channel")
		}
	}
	t, catalog, _, ok := ParseInstance(name)
	if !ok || !store.SameTenant(t, tenant) {
		return nil, refuse("channel not found")
	}
	cfg, err := m.Store.GetChannelConfig(name)
	if err != nil || cfg == nil {
		return nil, refuse("channel not found")
	}
	if strings.TrimSpace(cfg.AgentID) != agent.ID {
		return nil, refuse("channel is not bound to this agent")
	}
	if !cfg.Enabled {
		return nil, refuse("channel is disabled")
	}
	if !sendableCatalogs[catalog] {
		return nil, refuse("sending is not supported on " + catalog + " channels")
	}
	if !m.running(cfg, catalog) {
		return nil, refuse("channel is not running")
	}
	sess := m.knownChat(tenant, agent.ID, name, chat)
	if sess == nil {
		return nil, refuse("unknown chat: the message tool only writes to chats that already talked to this agent on this channel")
	}
	if catalog == "zalo-personal" {
		if _, ok := m.Bridge.lease(name, chat); !ok {
			return nil, refuse(noLeaseMsg)
		}
	}
	return &messageTarget{name: name, catalog: catalog, chat: chat, text: text, agent: agent, session: sess}, nil
}

// running reports whether goso can send on the channel now: a live bot for
// Telegram, Zalo Bot and Lark; a set-up bridge for Zalo through ZaloCRM; the
// send credentials for Discord, Slack and WhatsApp, which goso only sends on.
func (m *MessageTool) running(cfg *store.ChannelConfig, catalog string) bool {
	name := cfg.Name
	switch catalog {
	case "telegram", "zalo-oa", "feishu":
		if m.Manager == nil || !m.Manager.Running(name) {
			return false
		}
		switch catalog {
		case "telegram":
			return m.Manager.TelegramBot(name) != nil
		case "zalo-oa":
			return m.Manager.ZaloBot(name) != nil
		}
		return m.Manager.Lark(name) != nil
	case "zalo-personal":
		return m.Bridge != nil && m.Bridge.Ready(name)
	case "discord", "slack":
		return m.credential(name, KindBot) != ""
	case "whatsapp":
		return m.credential(name, KindAccess) != "" && strings.TrimSpace(cfg.Settings[SettingPhoneNumberID]) != ""
	}
	return false
}

// credential is a tenant channel's stored secret; tenant channels never read
// the environment.
func (m *MessageTool) credential(name, kind string) string {
	v, _, _ := Credential(m.Store, name, kind, nil)
	return strings.TrimSpace(v)
}

// knownChat is the agent's live session for chat on channel name: goso's
// label "<name>:<chat>" or a GoClaw session key for the same chat.
func (m *MessageTool) knownChat(tenant, agentID, name, chat string) *store.Session {
	label := name + ":" + chat
	_, _, short, _ := ParseInstance(name)
	for _, s := range m.Store.ListSessions() {
		if s == nil || s.AgentID != agentID || !store.SameTenant(s.TenantID, tenant) || s.Archived {
			continue
		}
		if s.Label == label {
			return s
		}
		if inst, c, ok := parseGoclawSessionKey(s.Label); ok && inst == short && c == chat {
			return s
		}
	}
	return nil
}

// noLeaseMsg is the refusal for a ZaloCRM conversation the agent holds no
// lease on.
const noLeaseMsg = "ZaloCRM has not handed this conversation to the agent since the gateway started: wait for the customer's next message"

// Send implements builtin.Messenger. It resolves again, so a channel that was
// disabled, unbound or stopped while the approval waited is refused.
func (m *MessageTool) Send(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	tg, bad := m.resolve(ctx, args)
	if bad != nil {
		return bad, nil
	}
	parts, err := m.deliver(ctx, tg)
	if err != nil {
		if errors.Is(err, errNoLease) {
			return refuse(noLeaseMsg), nil
		}
		// Channel API errors can carry a bot token in a URL: never echo them.
		return refuse("send failed: the channel did not accept the message"), nil
	}
	// Keep the thread history in step with what the customer saw.
	_, _ = m.Store.AddMessage(store.Message{SessionID: tg.session.ID, Role: "assistant", Content: tg.text, AgentID: tg.agent.ID})
	return &connector.InvokeResult{
		Tool:      builtin.ToolMessage,
		Connector: builtin.ConnectorName,
		Status:    "ok",
		Content:   map[string]any{"sent": true, "channel": tg.name, "chat_id": tg.chat, "parts": parts},
	}, nil
}

func (m *MessageTool) deliver(ctx context.Context, tg *messageTarget) (int, error) {
	switch tg.catalog {
	case "telegram":
		bot := m.Manager.TelegramBot(tg.name)
		if bot == nil {
			return 0, errors.New("not running")
		}
		chatID, thread, err := telegramChat(tg.chat)
		if err != nil {
			return 0, err
		}
		send := bot.Sender
		if send == nil {
			send = bot.sendMessage
		}
		ctx = withTelegramThread(ctx, thread)
		return sendParts(tg.text, telegramMaxText, func(p string) error { return send(ctx, chatID, p) })
	case "zalo-oa":
		bot := m.Manager.ZaloBot(tg.name)
		if bot == nil {
			return 0, errors.New("not running")
		}
		send := bot.Sender
		if send == nil {
			send = bot.sendMessage
		}
		return sendParts(tg.text, zaloBotMaxText, func(p string) error { return send(ctx, tg.chat, p) })
	case "feishu":
		bot := m.Manager.Lark(tg.name)
		if bot == nil {
			return 0, errors.New("not running")
		}
		return sendParts(tg.text, larkMaxText, func(p string) error { return bot.sendToChat(ctx, tg.chat, p) })
	case "zalo-personal":
		if m.Bridge == nil {
			return 0, errors.New("not running")
		}
		return sendParts(tg.text, zcrmMaxText, func(p string) error { return m.Bridge.SendText(ctx, tg.name, tg.chat, p) })
	case "discord":
		token := m.credential(tg.name, KindBot)
		return sendParts(tg.text, discordMaxText, func(p string) error {
			return discordSend(ctx, m.HTTPClient, m.APIBase["discord"], token, tg.chat, p)
		})
	case "slack":
		token := m.credential(tg.name, KindBot)
		channel, thread, _ := strings.Cut(tg.chat, ":thread:")
		return sendParts(tg.text, slackMaxText, func(p string) error {
			return slackSend(ctx, m.HTTPClient, m.APIBase["slack"], token, channel, thread, p)
		})
	case "whatsapp":
		token := m.credential(tg.name, KindAccess)
		cfg, err := m.Store.GetChannelConfig(tg.name)
		if err != nil || cfg == nil {
			return 0, errors.New("not running")
		}
		pnid := strings.TrimSpace(cfg.Settings[SettingPhoneNumberID])
		return sendParts(tg.text, whatsappMaxText, func(p string) error {
			return whatsappSend(ctx, m.HTTPClient, m.APIBase["whatsapp"], token, pnid, tg.chat, p)
		})
	}
	return 0, errors.New("unsupported channel")
}

// zcrmMaxText is the longest message ZaloCRM's reply endpoint takes.
const zcrmMaxText = 8000

// telegramChat reads a Telegram chat as the message tool gets it: "<chat>",
// "<chat>:topic:<n>" (a forum topic) or "<chat>:thread:<n>" (a private
// thread). thread is the message_thread_id to post with; none for the
// General topic.
func telegramChat(chat string) (chatID, thread int64, err error) {
	base, rest, hasRest := strings.Cut(chat, ":")
	chatID, err = strconv.ParseInt(base, 10, 64)
	if err != nil {
		return 0, 0, errors.New("bad chat id")
	}
	if !hasRest {
		return chatID, 0, nil
	}
	kind, n, ok := strings.Cut(rest, ":")
	if !ok || (kind != "topic" && kind != "thread") {
		return 0, 0, errors.New("bad chat id")
	}
	thread, err = strconv.ParseInt(n, 10, 64)
	if err != nil || thread <= 0 {
		return 0, 0, errors.New("bad chat id")
	}
	if kind == "topic" && thread == telegramGeneralTopic {
		thread = 0
	}
	return chatID, thread, nil
}

func sendParts(text string, max int, send func(string) error) (int, error) {
	parts := splitRunes(text, max)
	for i, p := range parts {
		if err := send(p); err != nil {
			return i, err
		}
	}
	return len(parts), nil
}

// sendToChat posts a new text message into a Lark chat. A topic chat
// "<chat_id>:topic:<root_id>" (topic_session_mode) is answered inside that
// topic, as a reply to its first message. In tests Sender stands in, called
// with only ChatID (and RootID for a topic) set.
func (l *Lark) sendToChat(ctx context.Context, chatID, text string) error {
	if base, root, ok := strings.Cut(chatID, ":topic:"); ok && root != "" {
		m := LarkMessage{ChatID: base, RootID: root}
		if l.Sender != nil {
			return l.Sender(ctx, m, text)
		}
		m.MessageID, m.ThreadID = root, root
		return l.reply(ctx, m, text)
	}
	if l.Sender != nil {
		return l.Sender(ctx, LarkMessage{ChatID: chatID}, text)
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	body := map[string]any{"receive_id": chatID, "msg_type": "text", "content": string(content)}
	for attempt := 0; attempt < 2; attempt++ {
		base, token, err := l.accessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		err = l.do(ctx, http.MethodPost, base+"/open-apis/im/v1/messages?receive_id_type=chat_id", token, body, nil)
		if err == nil || !larkTokenExpired(err) {
			return err
		}
	}
	return fmt.Errorf("lark: access token refused")
}
