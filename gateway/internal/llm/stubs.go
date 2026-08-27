// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"fmt"
)

func notConfigured(name string) error {
	return fmt.Errorf("%s: not_configured", name)
}

// ClaudeCLI is a stdio Claude Code adapter stub. No CLI is bundled.
type ClaudeCLI struct{}

func (ClaudeCLI) Name() string      { return "claude-cli" }
func (ClaudeCLI) ModelName() string { return "claude-cli" }

func (ClaudeCLI) Chat(_ context.Context, _ []Message) (string, error) {
	return "", notConfigured("claude-cli")
}

// Codex is an OAuth Responses API stub. Fail closed.
type Codex struct{}

func (Codex) Name() string      { return "codex" }
func (Codex) ModelName() string { return "codex" }

func (Codex) Chat(_ context.Context, _ []Message) (string, error) {
	return "", notConfigured("codex")
}

// ACP is a JSON-RPC subagent stub. Fail closed.
type ACP struct{}

func (ACP) Name() string      { return "acp" }
func (ACP) ModelName() string { return "acp" }

func (ACP) Chat(_ context.Context, _ []Message) (string, error) {
	return "", notConfigured("acp")
}
