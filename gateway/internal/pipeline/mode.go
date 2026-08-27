// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"fmt"
	"os"
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

// SystemPrompt builds the system text for a mode. None returns empty.
// AGENTS.md at process cwd is appended for full/task when the file exists (Q4 stub).
func SystemPrompt(mode Mode, displayName string) string {
	identity := "You are a GOSO gateway assistant."
	if strings.TrimSpace(displayName) != "" {
		identity = "You are " + strings.TrimSpace(displayName) + ", a GOSO gateway assistant."
	}
	instructions := "Answer the user. Use tools when they help complete the request."
	toolNotes := "Tools are advertised as connector__tool (double underscore). Call a tool only when the model needs its result."
	safety := "Do not reveal secrets or take irreversible actions without approval."

	var body string
	switch mode {
	case ModeFull:
		body = strings.Join([]string{identity, instructions, toolNotes, safety}, "\n")
	case ModeTask:
		body = strings.Join([]string{instructions, toolNotes}, "\n")
	case ModeMinimal:
		body = instructions
	case ModeNone:
		return ""
	default:
		body = strings.Join([]string{identity, instructions, toolNotes, safety}, "\n")
	}
	if mode == ModeFull || mode == ModeTask {
		if extra := readAgentsFile(); extra != "" {
			body = body + "\n" + extra
		}
	}
	return body
}

func readAgentsFile() string {
	b, err := os.ReadFile("AGENTS.md")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
