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
//   - is a type goso can send on (Telegram, Zalo Bot, Lark),
//
// and only to a chat that already has a session with that agent, so the tool
// cannot cold-message arbitrary ids. A bare catalog channel ("telegram") has
// no tenant and is refused. The approval gate sits in front of every send.
type MessageTool struct {
	Store   store.StoreIface
	Manager *Manager
}

// MessageMaxRunes caps one message tool call.
const MessageMaxRunes = 4000

const telegramMaxText = 4096

// sendableCatalogs are the channel types the message tool can write to.
var sendableCatalogs = map[string]bool{"telegram": true, "zalo-oa": true, "feishu": true}

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

// instanceOfLabel returns the tenant channel instance a session label
// "<instance>:<chat>" belongs to, picking the longest configured instance
// name of that tenant that prefixes the label.
func instanceOfLabel(st store.StoreIface, tenant, label string) (name, chat string) {
	for _, n := range InstanceNames(st, tenant) {
		if strings.HasPrefix(label, n+":") && len(label) > len(n)+1 && len(n) > len(name) {
			name, chat = n, label[len(n)+1:]
		}
	}
	return name, chat
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
				curName, curChat = instanceOfLabel(m.Store, tenant, sess.Label)
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
	if !m.running(name, catalog) {
		return nil, refuse("channel is not running")
	}
	sess := m.knownChat(tenant, agent.ID, name+":"+chat)
	if sess == nil {
		return nil, refuse("unknown chat: the message tool only writes to chats that already talked to this agent on this channel")
	}
	return &messageTarget{name: name, catalog: catalog, chat: chat, text: text, agent: agent, session: sess}, nil
}

func (m *MessageTool) running(name, catalog string) bool {
	if m.Manager == nil || !m.Manager.Running(name) {
		return false
	}
	switch catalog {
	case "telegram":
		return m.Manager.TelegramBot(name) != nil
	case "zalo-oa":
		return m.Manager.ZaloBot(name) != nil
	case "feishu":
		return m.Manager.Lark(name) != nil
	}
	return false
}

func (m *MessageTool) knownChat(tenant, agentID, label string) *store.Session {
	for _, s := range m.Store.ListSessions() {
		if s != nil && s.Label == label && s.AgentID == agentID && store.SameTenant(s.TenantID, tenant) && !s.Archived {
			return s
		}
	}
	return nil
}

// Send implements builtin.Messenger. It resolves again, so a channel that was
// disabled, unbound or stopped while the approval waited is refused.
func (m *MessageTool) Send(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	tg, bad := m.resolve(ctx, args)
	if bad != nil {
		return bad, nil
	}
	parts, err := m.deliver(ctx, tg)
	if err != nil {
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
		chatID, err := strconv.ParseInt(tg.chat, 10, 64)
		if err != nil {
			return 0, errors.New("bad chat id")
		}
		send := bot.Sender
		if send == nil {
			send = bot.sendMessage
		}
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
	}
	return 0, errors.New("unsupported channel")
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

// sendToChat posts a new text message into a Lark chat. In tests Sender
// stands in, called with only ChatID set.
func (l *Lark) sendToChat(ctx context.Context, chatID, text string) error {
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
