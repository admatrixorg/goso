// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"errors"
	"log"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// SPEC 132 §bridge — per-customer memory for Omni-Core replies.
//
// Omni-Core sends the bridge only the last few turns of a conversation. The
// bridge keeps the whole conversation in a goso session (ensureSession /
// appendTurn) and writes an L1 note of it (store.SaveSummary): verbatim user
// lines while the conversation is short (no LLM, after every reply), then an
// LLM summary once it reached pipeline.SummarizeMessageMin turns and every
// bridgeSummaryEvery turns after. The next reply gets, inside the one system
// message and after the knowledge block (outside its byte cap), this
// customer's notes from other sessions (customer-scoped recall) and then this
// session's own note, so the model keeps names, numbers and decisions that
// already scrolled out of Omni's window.
//
// Customer key: Omni identifies the customer conversation with metadata
// channel_account_id + external_conversation_id (+ channel_type). With the
// authenticated tenant header that gives the session label
// "omni-chat:<tenant>:<channel>:<account>:<conversation>", which is also the
// memory customer key (store.CustomerKeyForLabel). Recall is scoped to the
// tenant's goso agent and that key, so a note of customer A never reaches
// customer B, another agent or another tenant.
//
// Older callers that only send conversation_id / session_id / contact_id keep
// the per-conversation note and get no cross-session recall. Without any id
// every customer lands in one shared session, and that session is never
// summarised nor injected.
//
// GOSO_BRIDGE_MEMORY=0 (or false/no/off) turns all of it off; so does
// GOSO_MEMORY_RECALL=0.

const (
	// bridgeSummaryEvery is how many new conversation turns (user+assistant) pass
	// between summary refreshes once the session reached
	// pipeline.SummarizeMessageMin.
	bridgeSummaryEvery   = 6
	bridgeSummaryTimeout = 60 * time.Second
	bridgeSummaryRunes   = 500
)

// BridgeMemoryEnabled is on unless GOSO_BRIDGE_MEMORY or GOSO_MEMORY_RECALL
// is 0/false/no/off.
func BridgeMemoryEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(config.Lookup("GOSO_BRIDGE_MEMORY"))) {
	case "0", "false", "no", "off":
		return false
	}
	return pipeline.RecallEnabled()
}

// customerLabel is the per-customer session label (and memory customer key)
// of in, or "" when Omni did not identify the customer conversation. The
// tenant comes from the authenticated header, never from metadata.
func customerLabel(in ReplyInput) string {
	tenant := strings.TrimSpace(in.TenantID)
	account := firstNonEmpty(meta(in.Metadata, "channel_account_id", "channelAccountId"), strings.TrimSpace(in.ChannelAccountID))
	conv := meta(in.Metadata, "external_conversation_id", "externalConversationId")
	if tenant == "" || account == "" || conv == "" {
		return ""
	}
	ch := meta(in.Metadata, "channel_type", "channelType")
	esc := url.QueryEscape
	return store.OmniCustomerPrefix + esc(tenant) + ":" + esc(ch) + ":" + esc(account) + ":" + esc(conv)
}

// memoryScoped reports whether in identifies one conversation (see gate).
func memoryScoped(in ReplyInput) bool {
	if customerLabel(in) != "" {
		return true
	}
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

// memorySession returns the goso session that carries this turn's memory,
// or nil when memory is off, the store is absent, or in is not scoped.
func (s *Service) memorySession(gosoAgentID string, in ReplyInput) *store.Session {
	if s == nil || s.Store == nil || gosoAgentID == "" || !BridgeMemoryEnabled() || !memoryScoped(in) {
		return nil
	}
	label := sessionLabel(in, in.AgentID)
	for _, sess := range s.Store.ListSessions() {
		if sess != nil && sess.AgentID == gosoAgentID && sess.Label == label {
			return sess
		}
	}
	return nil
}

// memoryNote is the memory block for this turn, or "": the customer's notes
// from other sessions (per-customer sessions only), then this session's own
// latest note. A store error drops that part and never fails the reply.
func (s *Service) memoryNote(sess *store.Session, prompt string) string {
	if s == nil || s.Store == nil || sess == nil {
		return ""
	}
	var parts []string
	if store.IsOmniCustomerLabel(sess.Label) {
		note, _ := pipeline.RecallFor(s.Store, store.RecallQuery{
			TenantID:       sess.TenantID,
			AgentID:        sess.AgentID,
			ExcludeSession: sess.ID,
			Query:          prompt,
			CustomerScoped: true,
			CustomerKey:    store.CustomerKeyForLabel(sess.Label),
		})
		if note = strings.TrimSpace(note); note != "" {
			parts = append(parts, note)
		}
	}
	// ErrNotFound is the normal first turn of a session: no note written yet.
	sum, err := s.Store.LatestSummary(sess.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("goso memory: bridge note session=%s error=%v", sess.ID, err)
	} else if err == nil && sum != nil && strings.TrimSpace(sum.Body) != "" {
		parts = append(parts, pipeline.PreviousSummaryPrefix+strings.TrimSpace(sum.Body))
	}
	if len(parts) == 0 {
		return ""
	}
	note := strings.Join(parts, "\n\n")
	log.Printf("goso memory: bridge inject session=%s customer=%t runes=%d", sess.ID, store.IsOmniCustomerLabel(sess.Label), utf8.RuneCountInString(note))
	return note
}

// scheduleSummary writes this session's note after a reply. While the
// conversation is short (pipeline.CompactNote) the user lines are saved
// verbatim, synchronously and without an LLM. From
// pipeline.SummarizeMessageMin conversation turns on, an LLM summary is
// refreshed in the background every bridgeSummaryEvery turns. The reply is
// never delayed by an LLM call nor failed by a write error.
func (s *Service) scheduleSummary(sid string, prov Provider, model string, ag Agent) {
	if s == nil || s.Store == nil || sid == "" {
		return
	}
	raw, err := s.Store.ListMessages(sid)
	if err != nil {
		return
	}
	n := pipeline.ConversationTurns(raw) // user/assistant turns only, like SummarizeStage
	if n < pipeline.SummarizeMessageMin {
		text, mode := pipeline.CompactNote(pipeline.ToLLM(raw))
		if mode != "compact" {
			return
		}
		text = pipeline.TruncateRunes(strings.TrimSpace(text), bridgeSummaryRunes)
		if text == "" {
			return
		}
		if _, err := s.Store.SaveSummary(sid, text); err != nil {
			log.Printf("goso memory: bridge compact session=%s save error=%v", sid, err)
			return
		}
		log.Printf("goso memory: bridge compact session=%s turns=%d runes=%d", sid, n, utf8.RuneCountInString(text))
		return
	}
	if (n-pipeline.SummarizeMessageMin)%bridgeSummaryEvery != 0 {
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
