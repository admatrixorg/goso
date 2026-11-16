// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"strings"
	"sync"

	"github.com/mqglobal/goso/gateway/internal/contextfile"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// TurnResult is what the agent runtime returned for one channel turn.
type TurnResult struct {
	Reply    string
	AgentID  string
	Provider string
	// Usage is the turn's model usage; zero when the provider reports none.
	Usage llm.Usage
	// Pending is true when a tool call stopped the turn to wait for an
	// operator approval.
	Pending bool
}

// TurnRunner runs one customer message through the full agent runtime (tools,
// connectors, context files, hooks, approvals, memory) on the channel session
// sessionID. It stores the user message and the reply itself. The context
// carries the context-file user key (contextfile.WithUser) when one applies.
type TurnRunner func(ctx context.Context, sessionID, text string) (TurnResult, error)

var (
	turnMu     sync.RWMutex
	turnRunner TurnRunner
)

// SetTurnRunner wires the agent runtime into every channel. serve sets it at
// start; nil (tests, minimal builds) keeps the direct model call
// (pipeline.ChannelSystem + one LLM call, no tools).
func SetTurnRunner(fn TurnRunner) {
	turnMu.Lock()
	turnRunner = fn
	turnMu.Unlock()
}

func currentTurnRunner() TurnRunner {
	turnMu.RLock()
	defer turnMu.RUnlock()
	return turnRunner
}

// pendingReply is sent when a tool call waits for an operator approval and the
// model said nothing the customer can read. The approval id and tool JSON stay
// in the session for operators; the customer never sees them.
const pendingReply = "Mình đã ghi nhận yêu cầu, nhân viên sẽ kiểm tra và phản hồi bạn sớm."

// customerReply is the text a channel may send for a runtime turn: internal
// markers ("pending_approval: <id>", "max_iterations") are removed, and a turn
// that stopped on an approval or on the loop cap with nothing left to say gets
// pendingReply.
func customerReply(res TurnResult) string {
	var kept []string
	for _, line := range strings.Split(res.Reply, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pending_approval:") {
			continue
		}
		kept = append(kept, line)
	}
	text := strings.TrimSpace(strings.Join(kept, "\n"))
	capped := text == "max_iterations"
	if capped {
		text = ""
	}
	if text == "" && (res.Pending || capped) {
		return pendingReply
	}
	return text
}

// ContextUserKey is the user key of user-scoped context files for one channel
// message. It uses GoClaw's user_context_files.user_id format so files imported
// from GoClaw match: the sender id for a direct chat (Telegram numeric user id,
// Zalo uid, Lark open_id "ou_…") and "group:<instance>:<chat id>" for a group
// chat, where <instance> is GoClaw's channel instance name — the last segment of
// goso's "<tenant>/<catalog>/<name>", or the catalog name for an older single
// bot. "" when the key would be invalid (store.UserKeyOK) or nothing is known.
func ContextUserKey(instance, peer, senderID, chatID string) string {
	senderID = strings.TrimSpace(senderID)
	chatID = strings.TrimSpace(chatID)
	var key string
	if peer == "group" {
		if chatID == "" {
			return ""
		}
		key = "group:" + goclawInstanceName(instance) + ":" + chatID
	} else {
		key = senderID
	}
	if !store.UserKeyOK(key) {
		return ""
	}
	return key
}

// goclawInstanceName maps a goso channel name to GoClaw's instance name.
func goclawInstanceName(name string) string {
	name = strings.TrimSpace(name)
	if _, _, inst, ok := ParseInstance(name); ok {
		return inst
	}
	return name
}

// withContextUser attaches the context-file user key for this message when
// context files are on and the key is valid.
func withContextUser(ctx context.Context, instance, peer, senderID, chatID string) context.Context {
	if !contextfile.Enabled() {
		return ctx
	}
	if key := ContextUserKey(instance, peer, senderID, chatID); key != "" {
		return contextfile.WithUser(ctx, key)
	}
	return ctx
}
