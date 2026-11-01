// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// SummarizeMessageMin is the message-count threshold that triggers L1 summarize.
const SummarizeMessageMin = 12

const summaryRuneLimit = 500

// PreviousSummaryPrefix marks the injected L1 summary of the same session.
const PreviousSummaryPrefix = "Previous summary: "

// MemoryStage is the post-turn L0 hook. Working memory is session messages.
func MemoryStage(_ store.StoreIface) StageFunc {
	return func(context.Context, *State) error { return nil }
}

// SummarizeStage writes an L1 episodic summary when the session has at least
// SummarizeMessageMin messages or the caller set summarize=1.
func SummarizeStage(st store.StoreIface) StageFunc {
	return func(ctx context.Context, s *State) error {
		if st == nil || s == nil || s.SessionID == "" {
			return nil
		}
		raw, err := st.ListMessages(s.SessionID)
		if err != nil {
			return err
		}
		if !s.ForceSummarize && len(raw) < SummarizeMessageMin {
			return nil
		}
		msgs := ToLLM(raw)
		text := EchoSummary(msgs)
		if useLLMSummary(s.Provider) {
			if got, err := llmSummary(ctx, s.Provider, msgs); err == nil && strings.TrimSpace(got) != "" {
				text = got
			}
		}
		text = TruncateRunes(strings.TrimSpace(text), summaryRuneLimit)
		if text == "" {
			return nil
		}
		_, err = st.SaveSummary(s.SessionID, text)
		return err
	}
}

func useLLMSummary(p llm.Provider) bool {
	if p == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.Name())) {
	case "", "echo", "scripted":
		return false
	default:
		return true
	}
}

// SummaryPrompt is the L1 summarize request for msgs: the summary instruction
// followed by the user/assistant turns, each capped at 400 runes. Shared by
// the pipeline and the Omni bridge so both write the same kind of memory.
func SummaryPrompt(msgs []llm.Message) []llm.Message {
	prompt := []llm.Message{{
		Role: "system",
		Content: "Summarize this session in at most 500 characters, in the same language the user wrote in. " +
			"Keep what a future session would need: names, decisions, stated preferences, numbers, and open tasks. " +
			"Omit greetings and filler. Reply with the summary only.",
	}}
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		c := m.Content
		if c == "" {
			continue
		}
		prompt = append(prompt, llm.Message{Role: m.Role, Content: TruncateRunes(c, 400)})
	}
	return prompt
}

func llmSummary(ctx context.Context, p llm.Provider, msgs []llm.Message) (string, error) {
	return p.Chat(ctx, SummaryPrompt(msgs))
}

// EchoSummary concatenates the first and last user lines (deterministic).
func EchoSummary(msgs []llm.Message) string {
	var first, last string
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		line := strings.TrimSpace(m.Content)
		if line == "" {
			continue
		}
		if first == "" {
			first = line
		}
		last = line
	}
	if first == "" {
		return ""
	}
	if first == last {
		return TruncateRunes(first, summaryRuneLimit)
	}
	return TruncateRunes(first+"\n"+last, summaryRuneLimit)
}

// TruncateRunes keeps at most n runes of s.
func TruncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func prependSummary(st store.StoreIface, sessionID string, msgs []llm.Message) []llm.Message {
	if st == nil || sessionID == "" {
		return msgs
	}
	sum, err := st.LatestSummary(sessionID)
	if err != nil || sum == nil || strings.TrimSpace(sum.Body) == "" {
		return msgs
	}
	note := PreviousSummaryPrefix + sum.Body
	return append([]llm.Message{{Role: "system", Content: note}}, msgs...)
}
