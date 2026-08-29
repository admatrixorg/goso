// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/tenant"
)

// ListQuery is GET /api/traces filter + page parameters.
type ListQuery struct {
	Q       string
	Agent   string
	Channel string
	Status  string
	From    time.Time
	To      time.Time
	Limit   int
	Offset  int
}

// Item is one operator-facing trace summary (no prompt/tool payloads).
type Item struct {
	TraceID         string    `json:"trace_id"`
	Time            time.Time `json:"ts"`
	Status          string    `json:"status"`
	AgentID         string    `json:"agent_id,omitempty"`
	Channel         string    `json:"channel,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	LatencyMS       int64     `json:"latency_ms"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	CacheReadTokens int       `json:"cache_read_tokens"`
	Error           string    `json:"error,omitempty"`
	ErrorCount      int       `json:"error_count,omitempty"`
}

// ErrorGroup is one collapsed error message with a count.
type ErrorGroup struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func parseListQuery(r *http.Request) ListQuery {
	q := r.URL.Query()
	limit := DefaultTraceLimit
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > DefaultTraceCapacity {
		limit = DefaultTraceCapacity
	}
	offset := 0
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		offset = n
	}
	var from, to time.Time
	if s := strings.TrimSpace(q.Get("from")); s != "" {
		from, _ = time.Parse(time.RFC3339, s)
	}
	if s := strings.TrimSpace(q.Get("to")); s != "" {
		to, _ = time.Parse(time.RFC3339, s)
	}
	status := strings.ToLower(strings.TrimSpace(q.Get("status")))
	if status != "ok" && status != "error" {
		status = ""
	}
	agent := strings.TrimSpace(q.Get("agent"))
	if agent == "" {
		agent = strings.TrimSpace(q.Get("agent_id"))
	}
	return ListQuery{
		Q:       strings.TrimSpace(q.Get("q")),
		Agent:   agent,
		Channel: strings.TrimSpace(q.Get("channel")),
		Status:  status,
		From:    from,
		To:      to,
		Limit:   limit,
		Offset:  offset,
	}
}

func summarizeTree(t SpanTree) Item {
	it := Item{TraceID: t.TraceID, Status: "ok"}
	var errN int
	for _, s := range t.Spans {
		if it.Time.IsZero() || (!s.Start.IsZero() && s.Start.Before(it.Time)) {
			it.Time = s.Start
		}
		if s.Kind == KindAgent {
			if s.LatencyMS > it.LatencyMS {
				it.LatencyMS = s.LatencyMS
			}
			if aid := attrOf(s, "agent_id"); aid != "" {
				it.AgentID = aid
			}
			if ch := attrOf(s, "channel"); ch != "" {
				it.Channel = ch
			}
			if sid := attrOf(s, "session_id"); sid != "" {
				it.SessionID = sid
			}
		}
		if s.Kind == KindLLM {
			if it.Provider == "" {
				it.Provider = s.Name
			}
			if m := attrOf(s, "model"); m != "" {
				it.Model = m
			}
		}
		it.InputTokens += s.InputTokens
		it.OutputTokens += s.OutputTokens
		it.CacheReadTokens += s.CacheReadTokens
		if s.LatencyMS > it.LatencyMS {
			it.LatencyMS = s.LatencyMS
		}
		if s.Status == "error" || s.Error != "" {
			errN++
			if it.Error == "" {
				it.Error = redactText(s.Error)
				if it.Error == "" {
					it.Error = s.Status
				}
			}
			it.Status = "error"
		}
	}
	if it.Status == "" {
		it.Status = "ok"
	}
	it.ErrorCount = errN
	return it
}

func itemFromTrace(t Trace) Item {
	id := strings.TrimSpace(t.TraceID)
	if id == "" {
		id = strings.TrimSpace(t.RequestID)
	}
	if id == "" {
		id = "llm:" + t.Provider + ":" + t.Time.UTC().Format(time.RFC3339Nano)
	}
	status := "ok"
	if t.Error != "" {
		status = "error"
	} else if t.Status != "" {
		status = t.Status
	}
	in, out := t.InputTokens, t.OutputTokens
	if in == 0 && out == 0 && t.Tokens != nil {
		in = *t.Tokens
	}
	return Item{
		TraceID:         id,
		Time:            t.Time,
		Status:          status,
		AgentID:         t.AgentID,
		Provider:        t.Provider,
		Model:           t.Model,
		LatencyMS:       t.LatencyMS,
		InputTokens:     in,
		OutputTokens:    out,
		CacheReadTokens: t.CacheReadTokens,
		Error:           redactText(t.Error),
		ErrorCount:      boolCount(t.Error != ""),
	}
}

func boolCount(v bool) int {
	if v {
		return 1
	}
	return 0
}

func attrOf(s Span, key string) string {
	if s.Attributes == nil {
		return ""
	}
	return strings.TrimSpace(s.Attributes[key])
}

func matchItem(it Item, q ListQuery) bool {
	if q.Agent != "" && !strings.EqualFold(it.AgentID, q.Agent) {
		return false
	}
	if q.Channel != "" && !strings.EqualFold(it.Channel, q.Channel) {
		return false
	}
	if q.Status != "" && it.Status != q.Status {
		return false
	}
	if !q.From.IsZero() && it.Time.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && it.Time.After(q.To) {
		return false
	}
	if q.Q == "" {
		return true
	}
	needle := strings.ToLower(q.Q)
	hay := strings.ToLower(strings.Join([]string{
		it.TraceID, it.AgentID, it.Channel, it.SessionID, it.Provider, it.Model, it.Status, it.Error,
	}, " "))
	return strings.Contains(hay, needle)
}

func filterItems(items []Item, q ListQuery) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if matchItem(it, q) {
			out = append(out, it)
		}
	}
	return out
}

func pageItems(items []Item, offset, limit int) []Item {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []Item{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func groupErrors(items []Item) []ErrorGroup {
	counts := map[string]int{}
	for _, it := range items {
		if it.Error == "" {
			continue
		}
		counts[it.Error]++
	}
	out := make([]ErrorGroup, 0, len(counts))
	for msg, n := range counts {
		out = append(out, ErrorGroup{Message: msg, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Message < out[j].Message
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func treeTenant(t SpanTree) string {
	for _, s := range t.Spans {
		if v := attrOf(s, "tenant_id"); v != "" {
			return v
		}
	}
	return tenant.Default
}

func traceTenantID(t Trace) string {
	if strings.TrimSpace(t.TenantID) != "" {
		return t.TenantID
	}
	return tenant.Default
}

func sameTenantID(got, want string) bool {
	if strings.TrimSpace(got) == "" {
		got = tenant.Default
	}
	if strings.TrimSpace(want) == "" {
		want = tenant.Default
	}
	return got == want
}

func buildItems(trees []SpanTree, traces []Trace) []Item {
	seen := map[string]struct{}{}
	out := make([]Item, 0, len(trees)+len(traces))
	for _, t := range trees {
		it := summarizeTree(t)
		if it.TraceID != "" {
			seen[it.TraceID] = struct{}{}
		}
		out = append(out, it)
	}
	if len(trees) > 0 {
		return out
	}
	for _, t := range traces {
		it := itemFromTrace(t)
		if _, ok := seen[it.TraceID]; ok {
			continue
		}
		out = append(out, it)
	}
	return out
}
