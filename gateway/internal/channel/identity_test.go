// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Each channel's identity gives the user key and session key GoClaw used for
// the same message, so data imported from GoClaw lines up. The want columns
// are GoClaw's formats:
//
//	user key:    <sender> | group:<instance>:<chat> | guild:<guild>:user:<sender>
//	session key: agent:<agent>:<instance>:<direct|group>:<chat>[:topic:<n>|:thread:<id>]
func TestIdentityMatchesGoclawKeys(t *testing.T) {
	topicMode := &store.ChannelConfig{Settings: map[string]string{SettingTopicSessionMode: "enabled"}}
	cases := []struct {
		name        string
		id          Identity
		userKey     string
		sessionKey  string
		gosoLabel   string
		replyThread int64 // Telegram only: message_thread_id of the answer
	}{
		{
			name:       "telegram private chat",
			id:         telegramIdentity("acme/telegram/shop", "direct", "4242", 4242, false, 0),
			userKey:    "4242",
			sessionKey: "agent:sales:shop:direct:4242",
			gosoLabel:  "acme/telegram/shop:4242",
		},
		{
			name:       "telegram private thread",
			id:         telegramIdentity("acme/telegram/shop", "direct", "4242", 4242, false, 3),
			userKey:    "4242",
			sessionKey: "agent:sales:shop:direct:4242:thread:3",
			gosoLabel:  "acme/telegram/shop:4242:thread:3", replyThread: 3,
		},
		{
			name:       "telegram group ignores reply threads",
			id:         telegramIdentity("acme/telegram/shop", "group", "4242", -100777, false, 5),
			userKey:    "group:shop:-100777",
			sessionKey: "agent:sales:shop:group:-100777",
			gosoLabel:  "acme/telegram/shop:-100777",
		},
		{
			name:       "telegram forum topic",
			id:         telegramIdentity("acme/telegram/shop", "group", "4242", -100777, true, 9),
			userKey:    "group:shop:-100777",
			sessionKey: "agent:sales:shop:group:-100777:topic:9",
			gosoLabel:  "acme/telegram/shop:-100777:topic:9", replyThread: 9,
		},
		{
			name:       "telegram forum General topic",
			id:         telegramIdentity("acme/telegram/shop", "group", "4242", -100777, true, 0),
			userKey:    "group:shop:-100777",
			sessionKey: "agent:sales:shop:group:-100777:topic:1",
			gosoLabel:  "acme/telegram/shop:-100777:topic:1",
		},
		{
			name:       "telegram single bot",
			id:         telegramIdentity("telegram", "direct", "4242|demo_user", 4242, false, 0),
			userKey:    "4242",
			sessionKey: "agent:sales:telegram:direct:4242",
			gosoLabel:  "telegram:4242",
		},
		{
			name:       "lark private chat",
			id:         larkIdentity("acme/feishu/team", topicMode, LarkMessage{ChatID: "oc_p2p", ChatType: "p2p", SenderID: "ou_demo"}),
			userKey:    "ou_demo",
			sessionKey: "agent:sales:team:direct:oc_p2p",
			gosoLabel:  "acme/feishu/team:oc_p2p",
		},
		{
			name:       "lark group",
			id:         larkIdentity("acme/feishu/team", nil, LarkMessage{ChatID: "oc_grp", ChatType: "group", SenderID: "ou_demo", RootID: "om_root"}),
			userKey:    "group:team:oc_grp",
			sessionKey: "agent:sales:team:group:oc_grp",
			gosoLabel:  "acme/feishu/team:oc_grp",
		},
		{
			name:       "lark topic in topic_session_mode",
			id:         larkIdentity("acme/feishu/team", topicMode, LarkMessage{ChatID: "oc_grp", ChatType: "group", SenderID: "ou_demo", RootID: "om_root"}),
			userKey:    "group:team:oc_grp:topic:om_root",
			sessionKey: "agent:sales:team:group:oc_grp:topic:om_root",
			gosoLabel:  "acme/feishu/team:oc_grp:topic:om_root",
		},
		{
			name:       "lark topic group chat is direct",
			id:         larkIdentity("acme/feishu/team", topicMode, LarkMessage{ChatID: "oc_tg", ChatType: "topic_group", SenderID: "ou_demo", RootID: "om_root"}),
			userKey:    "ou_demo",
			sessionKey: "agent:sales:team:direct:oc_tg:topic:om_root",
			gosoLabel:  "acme/feishu/team:oc_tg:topic:om_root",
		},
		{
			name:       "discord direct message",
			id:         discordIdentity("discord", discordMessage{ChannelID: "dm-1", Author: &discordAuthor{ID: "u-77"}}),
			userKey:    "u-77",
			sessionKey: "agent:sales:discord:direct:dm-1",
			gosoLabel:  "discord:dm-1",
		},
		{
			name:       "discord server channel",
			id:         discordIdentity("discord", discordMessage{ChannelID: "c-1", GuildID: "g-9", Author: &discordAuthor{ID: "u-77"}}),
			userKey:    "guild:g-9:user:u-77",
			sessionKey: "agent:sales:discord:group:c-1",
			gosoLabel:  "discord:c-1",
		},
		{
			name:       "slack direct message",
			id:         slackIdentity("slack", "message", "im", "U1", "D1", ""),
			userKey:    "U1",
			sessionKey: "agent:sales:slack:direct:D1",
			gosoLabel:  "slack:D1",
		},
		{
			name:       "slack mention in a direct message is a group",
			id:         slackIdentity("slack", "app_mention", "im", "U1", "D1", ""),
			userKey:    "group:slack:D1",
			sessionKey: "agent:sales:slack:group:D1",
			gosoLabel:  "slack:D1",
		},
		{
			name:       "slack channel thread",
			id:         slackIdentity("slack", "message", "channel", "U1", "C1", "1700000000.000100"),
			userKey:    "group:slack:C1",
			sessionKey: "agent:sales:slack:group:C1:thread:1700000000.000100",
			gosoLabel:  "slack:C1:thread:1700000000.000100",
		},
		{
			name:       "zalo bot is always direct",
			id:         zaloBotIdentity("acme/zalo-oa/shop", "zu-1", "zc-1"),
			userKey:    "zu-1",
			sessionKey: "agent:sales:shop:direct:zc-1",
			gosoLabel:  "acme/zalo-oa/shop:zc-1",
		},
		{
			name:       "zalo bot without a chat id",
			id:         zaloBotIdentity("acme/zalo-oa/shop", "zu-1", ""),
			userKey:    "zu-1",
			sessionKey: "agent:sales:shop:direct:zu-1",
			gosoLabel:  "acme/zalo-oa/shop:zu-1",
		},
		{
			name:       "zalocrm direct conversation",
			id:         zcrmIdentity("acme/zalo-personal/shop", zcrmEvent("conv-1", "u1", "")),
			userKey:    "u1",
			sessionKey: "agent:sales:shop:direct:conv-1",
			gosoLabel:  "acme/zalo-personal/shop:conv-1",
		},
		{
			name:       "zalocrm group conversation",
			id:         zcrmIdentity("acme/zalo-personal/shop", zcrmEvent("conv-g", "u1", "group")),
			userKey:    "group:shop:conv-g",
			sessionKey: "agent:sales:shop:group:conv-g",
			gosoLabel:  "acme/zalo-personal/shop:conv-g",
		},
		{
			// goso's user keys never hold '@' (store.UserKeyOK), so a WhatsApp
			// sender has no per-user files; the session still matches.
			name:       "whatsapp",
			id:         whatsappIdentity("whatsapp", "+15551234567"),
			userKey:    "",
			sessionKey: "agent:sales:whatsapp:direct:15551234567@s.whatsapp.net",
			gosoLabel:  "whatsapp:15551234567@s.whatsapp.net",
		},
		{
			name:       "group without a chat id has no user key",
			id:         Identity{Instance: "slack", Peer: "group", SenderID: "U1"},
			userKey:    "",
			sessionKey: "agent:sales:slack:group:",
			gosoLabel:  "slack:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.id.UserKey(); got != tc.userKey {
				t.Errorf("user key %q, want %q", got, tc.userKey)
			}
			if got := tc.id.SessionKey("sales"); got != tc.sessionKey {
				t.Errorf("session key %q, want %q", got, tc.sessionKey)
			}
			if got := tc.id.Label(); got != tc.gosoLabel {
				t.Errorf("label %q, want %q", got, tc.gosoLabel)
			}
			if got := telegramReplyThread(tc.id); got != tc.replyThread {
				t.Errorf("reply thread %d, want %d", got, tc.replyThread)
			}
		})
	}
}

func zcrmEvent(conv, sender, threadType string) ZCRMEvent {
	var ev ZCRMEvent
	ev.Data.ConversationID = conv
	ev.Data.SenderUID = sender
	ev.Data.ThreadType = threadType
	return ev
}

func TestTelegramGroupKinds(t *testing.T) {
	for _, tc := range []struct {
		kind string
		chat int64
		want bool
	}{
		{"private", 42, false},
		{"group", -42, true},
		{"supergroup", -10042, true},
		{"channel", -10042, false},
		{"", -42, true},
		{"", 42, false},
	} {
		if got := telegramGroup(tc.kind, tc.chat); got != tc.want {
			t.Errorf("telegramGroup(%q, %d) = %v", tc.kind, tc.chat, got)
		}
	}
}

func TestParseGoclawSessionKey(t *testing.T) {
	for _, tc := range []struct {
		key, inst, chat string
		ok              bool
	}{
		{"agent:sales:shop:direct:4242", "shop", "4242", true},
		{"agent:sales:shop:group:-100777:topic:9", "shop", "-100777:topic:9", true},
		{"agent:sales:slack:group:C1:thread:1700000000.000100", "slack", "C1:thread:1700000000.000100", true},
		{"agent:sales:shop:channel:4242", "", "", false},
		{"agent:sales:shop:direct:", "", "", false},
		{"tenant-a/telegram/shop:4242", "", "", false},
		{"agent:sales:shop", "", "", false},
	} {
		inst, chat, ok := parseGoclawSessionKey(tc.key)
		if inst != tc.inst || chat != tc.chat || ok != tc.ok {
			t.Errorf("%q: %q %q %v", tc.key, inst, chat, ok)
		}
	}
}

// A session imported from GoClaw (labelled with its GoClaw key) is continued,
// also for an agent renamed "<key>--<tenant>" on import; another agent's or a
// plain chat's session is not.
func TestEnsureChannelSessionContinuesImportedSession(t *testing.T) {
	st := store.New()
	agent, _ := st.CreateAgent(store.Agent{AgentKey: "sales--tenant-a", DisplayName: "Minh An", TenantID: "tenant-a"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "support", DisplayName: "Support", TenantID: "tenant-a"})
	id := telegramIdentity("tenant-a/telegram/shop", "group", "4242", -100777, true, 9)

	// Another agent's session under the same key is not taken.
	_, _ = st.CreateSession(store.Session{AgentID: other.ID, TenantID: "tenant-a", Label: "agent:sales:shop:group:-100777:topic:9"})
	// goso's own label exists too, but the imported session wins.
	_, _ = st.CreateSession(store.Session{AgentID: agent.ID, TenantID: "tenant-a", Label: id.Label()})
	imported, _ := st.CreateSession(store.Session{AgentID: agent.ID, TenantID: "tenant-a", Label: "agent:sales:shop:group:-100777:topic:9"})

	if got := ensureChannelSession(st, agent, id); got == nil || got.ID != imported.ID {
		t.Fatalf("session %+v, want the imported one", got)
	}
	// Another topic of the same group is its own, new session.
	id2 := telegramIdentity("tenant-a/telegram/shop", "group", "4242", -100777, true, 2)
	s2 := ensureChannelSession(st, agent, id2)
	if s2 == nil || s2.ID == imported.ID || s2.Label != "tenant-a/telegram/shop:-100777:topic:2" || s2.TenantID != "tenant-a" {
		t.Fatalf("topic 2 session %+v", s2)
	}
	if again := ensureChannelSession(st, agent, id2); again == nil || again.ID != s2.ID {
		t.Fatalf("topic 2 not reused: %+v", again)
	}
}

// A chat whose id changed shape (WhatsApp now uses the JID) keeps the session
// goso already had under the old label.
func TestEnsureChannelSessionKeepsOldLabel(t *testing.T) {
	st := store.New()
	agent, _ := st.CreateAgent(store.Agent{AgentKey: "whatsapp", DisplayName: "WhatsApp"})
	old, _ := st.CreateSession(store.Session{AgentID: agent.ID, Label: "whatsapp:15551234567"})
	id := whatsappIdentity("whatsapp", "15551234567")
	id.oldLabel = "whatsapp:15551234567"
	if got := ensureChannelSession(st, agent, id); got == nil || got.ID != old.ID {
		t.Fatalf("session %+v, want the old one", got)
	}
}

func TestGoclawAgentKeys(t *testing.T) {
	got := goclawAgentKeys(&store.Agent{AgentKey: "sales--tenant-a"})
	if len(got) != 2 || got[0] != "sales--tenant-a" || got[1] != "sales" {
		t.Fatalf("%q", got)
	}
	if got := goclawAgentKeys(&store.Agent{AgentKey: "sales"}); len(got) != 1 {
		t.Fatalf("%q", got)
	}
}
