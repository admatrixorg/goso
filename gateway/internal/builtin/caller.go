// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"strings"
)

// Caller is the agent and session a builtin call runs for. The tenant travels
// separately (skill.WithTenant). A direct operator call has neither.
type Caller struct {
	AgentID   string
	SessionID string
}

type callerKey struct{}

// WithCaller attaches the calling agent and session. Empty values keep what
// the context already carries.
func WithCaller(ctx context.Context, agentID, sessionID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cur := CallerFrom(ctx)
	if v := strings.TrimSpace(agentID); v != "" {
		cur.AgentID = v
	}
	if v := strings.TrimSpace(sessionID); v != "" {
		cur.SessionID = v
	}
	return context.WithValue(ctx, callerKey{}, cur)
}

// CallerFrom returns the caller attached by WithCaller (zero when none).
func CallerFrom(ctx context.Context) Caller {
	if ctx == nil {
		return Caller{}
	}
	c, _ := ctx.Value(callerKey{}).(Caller)
	return c
}
