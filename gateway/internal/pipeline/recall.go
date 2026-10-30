// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Cross-session recall with admission control (SPEC 132).
//
// Before the prompt is built, the latest user text is matched against L1 notes
// of the same tenant + agent written in other sessions. Admission: skip
// trivial turns (greetings/acks), keep only candidates scoring at least
// recallRelative of the best one, cap items and runes. Zero injection is a
// valid outcome and is logged, never silent.
const (
	RecallMaxItems      = 5
	RecallBudgetRunes   = 1500
	recallItemRunes     = 400
	recallCandidates    = 20
	recallRelative      = 0.5
	recallMinMeaningful = 2
)

const recallHeader = "Ghi nhớ từ các phiên trước của agent này. Chỉ dùng khi liên quan tới câu hỏi hiện tại; nếu không liên quan thì bỏ qua. (Prior-session memory; use only when relevant.)"

// RecallEnabled is on unless GOSO_MEMORY_RECALL is 0/false/no/off.
func RecallEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(config.Lookup("GOSO_MEMORY_RECALL"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// RecallBudget is GOSO_MEMORY_RECALL_BUDGET runes, default RecallBudgetRunes.
func RecallBudget() int {
	if n, err := strconv.Atoi(strings.TrimSpace(config.Lookup("GOSO_MEMORY_RECALL_BUDGET"))); err == nil && n > 0 {
		return n
	}
	return RecallBudgetRunes
}

// recallStopwords: Vietnamese + English function words and chat filler that
// carry no retrieval signal.
var recallStopwords = map[string]bool{
	// vi
	"anh": true, "chị": true, "em": true, "bạn": true, "mình": true, "tôi": true, "tớ": true, "ạ": true, "dạ": true, "vâng": true,
	"chào": true, "xin": true, "cảm": true, "ơn": true, "ok": true, "oke": true, "okay": true, "ừ": true, "ừm": true, "uh": true,
	"là": true, "của": true, "và": true, "với": true, "cho": true, "thì": true, "mà": true, "này": true, "đó": true, "kia": true,
	"có": true, "không": true, "được": true, "đã": true, "sẽ": true, "đang": true, "rồi": true, "nhé": true, "nha": true, "nhỉ": true,
	"gì": true, "sao": true, "nào": true, "đâu": true, "thế": true, "vậy": true, "hả": true, "à": true, "ơi": true, "hi": true, "hello": true,
	"cái": true, "các": true, "những": true, "một": true, "để": true, "về": true, "trong": true, "trên": true, "như": true, "nếu": true,
	"tiếp": true, "tục": true, "đi": true, "làm": true, "giúp": true, "hỏi": true, "biết": true, "xem": true,
	// en
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true, "in": true, "on": true, "for": true, "with": true,
	"is": true, "are": true, "was": true, "be": true, "it": true, "this": true, "that": true, "i": true, "you": true, "we": true, "they": true,
	"me": true, "my": true, "your": true, "please": true, "thanks": true, "thank": true, "yes": true, "no": true, "yeah": true, "hey": true,
	"what": true, "how": true, "can": true, "do": true, "does": true, "did": true, "just": true, "so": true, "ok.": true,
}

// IsTrivialQuery reports greetings, acknowledgements and other turns with
// fewer than recallMinMeaningful non-stopword tokens.
func IsTrivialQuery(text string) bool {
	n := 0
	for _, t := range store.RecallTokens(text) {
		if !recallStopwords[t] {
			n++
		}
	}
	return n < recallMinMeaningful
}

// BuildRecallNote applies admission (relative threshold, item cap, rune
// budget) and formats the system note. Returns ("", 0) when nothing passes.
func BuildRecallNote(hits []store.RecallHit, budget int) (string, int) {
	if len(hits) == 0 {
		return "", 0
	}
	if budget <= 0 {
		budget = RecallBudgetRunes
	}
	best := 0.0
	for _, h := range hits {
		if h.Score > best {
			best = h.Score
		}
	}
	if best <= 0 {
		return "", 0
	}
	var b strings.Builder
	b.WriteString(recallHeader)
	used := utf8.RuneCountInString(recallHeader)
	n := 0
	for _, h := range hits {
		if h.Score < best*recallRelative {
			continue
		}
		body := TruncateRunes(strings.Join(strings.Fields(h.Memory.Body), " "), recallItemRunes)
		if body == "" {
			continue
		}
		line := fmt.Sprintf("\n- [%s] %s", h.Memory.CreatedAt.UTC().Format("2006-01-02"), body)
		ln := utf8.RuneCountInString(line)
		if used+ln > budget {
			break
		}
		b.WriteString(line)
		used += ln
		n++
		if n >= RecallMaxItems {
			break
		}
	}
	if n == 0 {
		return "", 0
	}
	return b.String(), n
}

// recall returns the admitted note for this turn and how many items it holds.
func (r *Runner) recall(st *State, userText string) (string, int) {
	if r == nil || r.Store == nil || st == nil || st.Mode == ModeNone || !RecallEnabled() {
		return "", 0
	}
	if IsTrivialQuery(userText) {
		return "", 0
	}
	hits, err := r.Store.RecallMemories(store.RecallQuery{
		TenantID:       st.TenantID,
		AgentID:        st.AgentID,
		ExcludeSession: st.SessionID,
		Query:          userText,
		Limit:          recallCandidates,
	})
	if err != nil {
		log.Printf("goso memory: recall error session=%s: %v", st.SessionID, err)
		return "", 0
	}
	note, n := BuildRecallNote(hits, RecallBudget())
	log.Printf("goso memory: recall session=%s agent=%s candidates=%d injected=%d runes=%d",
		st.SessionID, st.AgentID, len(hits), n, utf8.RuneCountInString(note))
	return note, n
}
