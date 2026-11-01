// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
)

// SPEC 132 §bridge — per-conversation episodic memory for Omni-Core replies.
//
// Omni-Core sends the bridge only the last few turns of a conversation. The
// bridge keeps the whole conversation in a goso session (ensureSession /
// appendTurn) and, once that session is long enough, writes an L1 summary of
// it (store.SaveSummary). The next reply in the same conversation gets that
// summary as a system note right after the system prompt, so the model keeps
// names, numbers and decisions that already scrolled out of Omni's window.
//
// Scope is the conversation, never the agent: the bridge does not recall
// across sessions. One agent serves many customers, and a note from customer
// A's conversation inside customer B's reply would be a leak, not memory.
//
// Gate: memory runs only for sessions Omni identifies (metadata
// conversation_id / session_id, a session id distinct from the agent id, or a
// contact id). Without any of those every customer lands in one shared
// session, and that session is never summarised nor injected.

const (
	// bridgeSummaryEvery is how many new conversation turns (user+assistant) pass
	// between summary refreshes once the session reached
	// pipeline.SummarizeMessageMin.
	bridgeSummaryEvery   = 6
	bridgeSummaryTimeout = 60 * time.Second
	bridgeSummaryRunes   = 500
)

// memoryScoped reports whether in identifies one conversation (see gate).
func memoryScoped(in ReplyInput) bool {
	if meta(in.Metadata, "conversation_id", "conversationId") != "" {
		return true
	}
	if meta(in.Metadata, "session_id", "sessionId") != "" {
		return true
	}
	if sid := strings.TrimSpace(in.SessionID); sid != "" && sid != strings.TrimSpace(in.AgentID) {
		return true
	}
	return strings.TrimSpace(in.ContactID) != ""
}

// memorySession returns the goso session id that carries this turn's memory,
// or "" when memory is off, the store is absent, or in is not scoped.
func (s *Service) memorySession(gosoAgentID string, in ReplyInput) string {
	if s == nil || s.Store == nil || gosoAgentID == "" || !pipeline.RecallEnabled() || !memoryScoped(in) {
		return ""
	}
	label := sessionLabel(in, in.AgentID)
	for _, sess := range s.Store.ListSessions() {
		if sess != nil && sess.AgentID == gosoAgentID && sess.Label == label {
			return sess.ID
		}
	}
	return ""
}

// memoryNote is the latest L1 summary of sid as a system note, or "".
func (s *Service) memoryNote(sid string) string {
	if s == nil || s.Store == nil || sid == "" {
		return ""
	}
	sum, err := s.Store.LatestSummary(sid)
	if err != nil || sum == nil || strings.TrimSpace(sum.Body) == "" {
		return ""
	}
	note := pipeline.PreviousSummaryPrefix + strings.TrimSpace(sum.Body)
	log.Printf("goso memory: bridge inject session=%s runes=%d", sid, utf8.RuneCountInString(note))
	return note
}

// scheduleSummary refreshes the L1 summary of sid in the background when the
// session has at least pipeline.SummarizeMessageMin conversation turns, then
// every bridgeSummaryEvery turns. The reply is never delayed by it.
func (s *Service) scheduleSummary(sid string, prov Provider, model string, ag Agent) {
	if s == nil || s.Store == nil || sid == "" {
		return
	}
	raw, err := s.Store.ListMessages(sid)
	if err != nil {
		return
	}
	n := pipeline.ConversationTurns(raw) // user/assistant turns only, like SummarizeStage
	if n < pipeline.SummarizeMessageMin || (n-pipeline.SummarizeMessageMin)%bridgeSummaryEvery != 0 {
		return
	}
	prompt := pipeline.SummaryPrompt(pipeline.ToLLM(raw))
	if len(prompt) < 2 {
		return
	}
	fn := s.Complete
	if fn == nil {
		fn = DefaultComplete
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bridgeSummaryTimeout)
		defer cancel()
		res, err := fn(ctx, Completion{
			Type:          prov.Type,
			BaseURL:       prov.BaseURL,
			APIKey:        prov.APIKey,
			Model:         model,
			Messages:      prompt,
			FallbackModel: agentFallbackModel(ag),
		})
		if err != nil {
			log.Printf("goso memory: bridge summary session=%s turns=%d error=%v", sid, n, classifyTransport(err, prov.APIKey))
			return
		}
		text := pipeline.TruncateRunes(strings.TrimSpace(res.Text), bridgeSummaryRunes)
		if text == "" {
			return
		}
		if _, err := s.Store.SaveSummary(sid, text); err != nil {
			log.Printf("goso memory: bridge summary session=%s save error=%v", sid, err)
			return
		}
		log.Printf("goso memory: bridge summary session=%s turns=%d runes=%d", sid, n, utf8.RuneCountInString(text))
	}()
}
