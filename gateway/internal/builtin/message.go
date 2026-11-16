// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/connector"
)

// ToolMessage sends a text to a chat on one of the calling agent's channels.
const ToolMessage = "message"

// Messenger runs the message tool. The channel package implements it; serve
// wires it into MessageSender. The caller comes from the context:
// skill.TenantFrom for the tenant, CallerFrom for the agent and session.
type Messenger interface {
	// Resolve checks the call for this caller and returns the arguments with
	// the target filled in (so the approver sees, and the executor sends to,
	// exactly one channel and chat), or a refusal result.
	Resolve(ctx context.Context, args map[string]any) (map[string]any, *connector.InvokeResult)
	// Send delivers resolved arguments. It resolves again first, so a channel
	// disabled or rebound while the approval waited is refused.
	Send(ctx context.Context, args map[string]any) (*connector.InvokeResult, error)
}

// MessageSender is the process Messenger. Nil keeps message not_configured.
var MessageSender Messenger

// Prepare runs before the approval gate. For message it resolves the target
// and refuses bad calls at once instead of queueing them for a human. Other
// tools pass through unchanged.
func Prepare(ctx context.Context, name string, args map[string]any) (map[string]any, *connector.InvokeResult) {
	if strings.TrimSpace(name) != ToolMessage {
		return args, nil
	}
	if MessageSender == nil {
		return args, notConfigured(ToolMessage)
	}
	return MessageSender.Resolve(ctx, args)
}

// invokeMessage sends. A refusal at send time (channel disabled or stopped
// while the approval waited, delivery failed) also comes back as an error, so
// the approval inbox shows the approved message did not go out.
func invokeMessage(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if MessageSender == nil {
		return notConfigured(ToolMessage), nil
	}
	res, err := MessageSender.Send(ctx, args)
	if err == nil && res != nil && res.Status == "error" {
		msg := "message: not sent"
		if c, ok := res.Content.(map[string]any); ok {
			if s, ok := c["error"].(string); ok && s != "" {
				msg = "message: " + s
			}
		}
		err = errors.New(msg)
	}
	return res, err
}
