// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"fmt"
	"strings"
)

// Mode is a prompt assembly mode (Q1).
type Mode string

const (
	ModeFull    Mode = "full"
	ModeTask    Mode = "task"
	ModeMinimal Mode = "minimal"
	ModeNone    Mode = "none"
)

// ParseMode resolves a request prompt_mode. Empty defaults to full.
func ParseMode(s string) (Mode, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ModeFull, nil
	}
	switch Mode(s) {
	case ModeFull, ModeTask, ModeMinimal, ModeNone:
		return Mode(s), nil
	default:
		return "", fmt.Errorf("unknown prompt_mode %q", s)
	}
}

// ResolvePromptMode uses the request value when set, else the stored session
// value, else full. Unknown request or session values return an error.
func ResolvePromptMode(request, session string) (Mode, error) {
	if strings.TrimSpace(request) != "" {
		return ParseMode(request)
	}
	return ParseMode(session)
}

// SystemPrompt builds the system text for a mode. None returns empty.
// Bootstrap markdown (GOSO_CONTEXT_DIR) is attached in the prompt stage, not here.
func SystemPrompt(mode Mode, displayName string) string {
	identity := "You are a GOSO gateway assistant."
	if strings.TrimSpace(displayName) != "" {
		identity = "You are " + strings.TrimSpace(displayName) + ", a GOSO gateway assistant."
	}
	instructions := "Answer the user. Use tools when they help complete the request."
	toolNotes := "Tools are advertised as connector__tool (double underscore). Call a tool only when the model needs its result."
	safety := "Do not reveal secrets or take irreversible actions without approval."

	switch mode {
	case ModeFull:
		return strings.Join([]string{identity, instructions, toolNotes, safety}, "\n")
	case ModeTask:
		return strings.Join([]string{instructions, toolNotes}, "\n")
	case ModeMinimal:
		return instructions
	case ModeNone:
		return ""
	default:
		return strings.Join([]string{identity, instructions, toolNotes, safety}, "\n")
	}
}
