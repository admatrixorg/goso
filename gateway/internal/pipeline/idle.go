// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Idle memory flush.
//
// SummarizeStage writes the L1 note only once a session reaches
// SummarizeMessageMin messages or the caller sends summarize=1, so short
// sessions were never remembered. The idle flush writes the note for a session
// that has been quiet for GOSO_MEMORY_IDLE_FLUSH, whatever its length.
//
// Idempotent and restart-safe without extra state: a session needs a flush
// only while its last message is newer than its episodic note (SaveSummary
// keeps one note per session and stamps it with the write time). After a
// flush the note is newer, so the next sweep skips it until a new message
// arrives. A restart simply re-derives that from the database.
//
// Cost: a session with at most compactUserTurns user turns whose user text
// fits in summaryRuneLimit is stored as those user lines verbatim, with no LLM
// call. Longer sessions use the same LLM summary as SummarizeStage (fallback:
// EchoSummary).
//
// Scope: external channel threads (zalo-personal:, zalo-oa:, telegram:, …)
// are skipped. One channel agent serves every customer, and cross-session
// recall is agent-scoped, so a note from customer A would reach customer B.
// The Omni bridge has its own per-conversation memory and does not run this.
const (
	IdleFlushDefault = 10 * time.Minute
	// idleSweepEvery caps the sweep interval; shorter idle values sweep at idle/2.
	idleSweepEvery = time.Minute
	// idleLookbackMin bounds how far back a sweep looks, so the first sweep
	// after deploy does not summarise the whole history at once.
	idleLookbackMin = 24 * time.Hour
	// idleBatchMax caps flushes per sweep (LLM calls are sequential).
	idleBatchMax       = 20
	idleSummaryTimeout = 60 * time.Second
	compactUserTurns   = 3
)

// channelLabelPrefixes mark sessions created by external channels.
var channelLabelPrefixes = []string{
	"zalo-personal:", "zalo-oa:", "telegram:", "discord:", "slack:", "feishu:", "whatsapp:",
}

// IdleFlushAfter is GOSO_MEMORY_IDLE_FLUSH: a Go duration ("10m", "90s") or
// whole minutes ("10"). Empty means IdleFlushDefault; 0/off/false disables.
func IdleFlushAfter() time.Duration {
	v := strings.ToLower(strings.TrimSpace(config.Lookup("GOSO_MEMORY_IDLE_FLUSH")))
	switch v {
	case "":
		return IdleFlushDefault
	case "0", "off", "false", "no":
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n <= 0 {
			return 0
		}
		return time.Duration(n) * time.Minute
	}
	if d, err := time.ParseDuration(v); err == nil {
		if d <= 0 {
			return 0
		}
		return d
	}
	log.Printf("goso memory: invalid GOSO_MEMORY_IDLE_FLUSH=%q, using %s", v, IdleFlushDefault)
	return IdleFlushDefault
}

// IsChannelSession reports sessions owned by an external channel thread.
func IsChannelSession(sess *store.Session) bool {
	if sess == nil {
		return false
	}
	for _, p := range channelLabelPrefixes {
		if strings.HasPrefix(sess.Label, p) {
			return true
		}
	}
	return false
}

// IdleFlusher writes L1 notes for sessions that went idle.
type IdleFlusher struct {
	Store store.StoreIface
	// Fallback is the gateway default provider (used when the agent names none).
	Fallback llm.Provider
	Idle     time.Duration
	// Now is the clock (tests); nil means time.Now.
	Now func() time.Time
	// Provider resolves the summariser for a session; nil uses the agent's
	// provider/model like a chat turn does.
	Provider func(*store.Session) llm.Provider

	mu      sync.Mutex
	checked map[string]time.Time // session → last message time already verified flushed
}

func (f *IdleFlusher) now() time.Time {
	if f.Now != nil {
		return f.Now().UTC()
	}
	return time.Now().UTC()
}

// Sweep flushes every session idle for at least f.Idle whose note is older
// than its last message. Returns how many notes it wrote. Sweeps never
// overlap.
func (f *IdleFlusher) Sweep(ctx context.Context) int {
	if f == nil || f.Store == nil || f.Idle <= 0 {
		return 0
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.checked == nil {
		f.checked = map[string]time.Time{}
	}
	now := f.now()
	cutoff := now.Add(-f.Idle)
	lookback := 3 * f.Idle
	if lookback < idleLookbackMin {
		lookback = idleLookbackMin
	}
	since := now.Add(-lookback)
	acts, err := f.activity(since)
	if err != nil {
		log.Printf("goso memory: idle flush list error: %v", err)
		return 0
	}
	seen := make(map[string]bool, len(acts))
	wrote := 0
	for _, a := range acts {
		seen[a.SessionID] = true
		if ctx.Err() != nil || wrote >= idleBatchMax {
			break
		}
		if a.LastMessageAt.After(cutoff) {
			continue // still active
		}
		// Prefilter with one second of slack (text MAX in SQL is only
		// second-exact); the exact check happens in flush.
		if !a.SummaryAt.IsZero() && a.SummaryAt.After(a.LastMessageAt.Add(time.Second)) {
			continue
		}
		if t, ok := f.checked[a.SessionID]; ok && t.Equal(a.LastMessageAt) {
			continue
		}
		ok, err := f.flush(ctx, a.SessionID, cutoff)
		if err != nil {
			log.Printf("goso memory: idle flush session=%s error=%v", a.SessionID, err)
			continue // retried next sweep
		}
		f.checked[a.SessionID] = a.LastMessageAt
		if ok {
			wrote++
		}
	}
	for id := range f.checked {
		if !seen[id] {
			delete(f.checked, id)
		}
	}
	return wrote
}

func (f *IdleFlusher) activity(since time.Time) ([]store.SessionActivity, error) {
	if al, ok := f.Store.(store.ActivityLister); ok {
		return al.SessionActivity(since)
	}
	var out []store.SessionActivity
	for _, sess := range f.Store.ListSessions() {
		if sess == nil {
			continue
		}
		raw, err := f.Store.ListMessages(sess.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		a := store.SessionActivity{SessionID: sess.ID, LastMessageAt: lastMessageAt(raw)}
		if a.LastMessageAt.Before(since) {
			continue
		}
		if sum, err := f.Store.LatestSummary(sess.ID); err == nil && sum != nil {
			a.SummaryAt = sum.CreatedAt
		}
		out = append(out, a)
	}
	return out, nil
}

// flush re-checks one session from its messages and writes its note.
// Returns true when a note was written.
func (f *IdleFlusher) flush(ctx context.Context, sessionID string, cutoff time.Time) (bool, error) {
	sess, err := f.Store.GetSession(sessionID)
	if err != nil || sess == nil || sess.Archived || IsChannelSession(sess) {
		return false, nil
	}
	raw, err := f.Store.ListMessages(sessionID)
	if err != nil {
		return false, err
	}
	last := lastMessageAt(raw)
	if last.IsZero() || last.After(cutoff) {
		return false, nil
	}
	if sum, err := f.Store.LatestSummary(sessionID); err == nil && sum != nil && !sum.CreatedAt.Before(last) {
		return false, nil // note already covers the last message
	}
	msgs := ToLLM(raw)
	text, mode := compactNote(msgs)
	if mode == "" {
		return false, nil // no user text: nothing worth remembering
	}
	if mode == "llm" {
		text = EchoSummary(msgs)
		mode = "echo"
		if p := f.provider(sess); useLLMSummary(p) {
			cctx, cancel := context.WithTimeout(ctx, idleSummaryTimeout)
			got, err := llmSummary(cctx, p, msgs)
			cancel()
			if err == nil && strings.TrimSpace(got) != "" {
				text, mode = got, "llm"
			} else if err != nil {
				log.Printf("goso memory: idle flush session=%s llm error, using echo: %v", sessionID, err)
			}
		}
	}
	text = TruncateRunes(strings.TrimSpace(text), summaryRuneLimit)
	if text == "" {
		return false, nil
	}
	if _, err := f.Store.SaveSummary(sessionID, text); err != nil {
		return false, err
	}
	log.Printf("goso memory: idle flush session=%s agent=%s messages=%d mode=%s runes=%d",
		sessionID, sess.AgentID, len(raw), mode, utf8.RuneCountInString(text))
	return true, nil
}

func (f *IdleFlusher) provider(sess *store.Session) llm.Provider {
	if f.Provider != nil {
		return f.Provider(sess)
	}
	name, model := "", ""
	if a, err := f.Store.GetAgent(sess.AgentID); err == nil && a != nil {
		name, model = a.LLMProvider, a.Model
	}
	if m := strings.TrimSpace(sess.ModelOverride); m != "" {
		model = m
		if p := strings.TrimSpace(sess.ProviderOverride); p != "" {
			name = p
		}
	}
	p, err := llm.Resolve(f.Store, name, model, f.Fallback)
	if err != nil {
		return nil
	}
	return p
}

// compactNote returns the user lines of a short session verbatim (mode
// "compact"), "llm" when the session is too long for that, or "" when it has
// no user text at all.
func compactNote(msgs []llm.Message) (string, string) {
	var lines []string
	runes := 0
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		line := strings.Join(strings.Fields(m.Content), " ")
		if line == "" {
			continue
		}
		lines = append(lines, line)
		runes += utf8.RuneCountInString(line) + 1
	}
	if len(lines) == 0 {
		return "", ""
	}
	if len(lines) > compactUserTurns || runes > summaryRuneLimit {
		return "", "llm"
	}
	return strings.Join(lines, "\n"), "compact"
}

func lastMessageAt(raw []*store.Message) time.Time {
	var last time.Time
	for _, m := range raw {
		if m != nil && m.CreatedAt.After(last) {
			last = m.CreatedAt
		}
	}
	return last.UTC()
}

// IdleFlushLoop sweeps until ctx ends. Off when GOSO_MEMORY_IDLE_FLUSH is 0.
// One goroutine, one ticker, both stopped with ctx.
func IdleFlushLoop(ctx context.Context, st store.StoreIface, fallback llm.Provider) {
	idle := IdleFlushAfter()
	if st == nil || idle <= 0 {
		return
	}
	every := idle / 2
	if every > idleSweepEvery {
		every = idleSweepEvery
	}
	if every < time.Second {
		every = time.Second
	}
	f := &IdleFlusher{Store: st, Fallback: fallback, Idle: idle}
	log.Printf("goso memory: idle flush on, idle=%s sweep=%s", idle, every)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n := f.Sweep(ctx); n > 0 {
				log.Printf("goso memory: idle flush wrote %d notes", n)
			}
		}
	}
}
