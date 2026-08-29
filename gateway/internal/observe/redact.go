// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// AttrValueCap is the max stored/public attribute value length.
	AttrValueCap = 120
	// ErrorCap is the max public error string length.
	ErrorCap = 400
	// MaxPublicSpans is the max spans returned in one tree.
	MaxPublicSpans = 80
)

var dropAttr = map[string]struct{}{
	"prompt":      {},
	"messages":    {},
	"content":     {},
	"arguments":   {},
	"tool_input":  {},
	"tool_result": {},
	"result":      {},
	"input":       {},
	"output":      {},
	"system":      {},
	"body":        {},
	"reply":       {},
}

var secretAttr = []string{
	"token", "password", "secret", "authorization", "api_key", "apikey",
	"bearer", "credential", "hmac", "private_key", "bot_token", "access_token",
}

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|Bearer\s+[A-Za-z0-9._\-+=/]+|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|wh_[A-Za-z0-9]+)`)

// PublicSpan copies s without prompt/tool/result payloads or secret values.
func PublicSpan(s Span) Span {
	out := s
	out.Error = redactText(s.Error)
	if s.Attributes == nil {
		out.Attributes = nil
		return out
	}
	attrs := make(map[string]string, len(s.Attributes))
	for k, v := range s.Attributes {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "" {
			continue
		}
		if _, drop := dropAttr[lk]; drop {
			continue
		}
		if secretAttrKey(lk) {
			attrs[k] = "[redacted]"
			continue
		}
		attrs[k] = capRunes(redactText(v), AttrValueCap)
	}
	if len(attrs) == 0 {
		out.Attributes = nil
	} else {
		out.Attributes = attrs
	}
	return out
}

// PublicTree copies t with redacted spans. truncated is true when spans were capped.
func PublicTree(t SpanTree) (SpanTree, bool) {
	spans := t.Spans
	truncated := false
	if len(spans) > MaxPublicSpans {
		spans = spans[:MaxPublicSpans]
		truncated = true
	}
	out := make([]Span, 0, len(spans))
	for _, s := range spans {
		out = append(out, PublicSpan(s))
	}
	return SpanTree{TraceID: t.TraceID, Spans: out}, truncated
}

// PublicTrace copies t without secret-bearing error text.
func PublicTrace(t Trace) Trace {
	out := t
	out.Error = redactText(t.Error)
	out.TenantID = ""
	return out
}

func secretAttrKey(lk string) bool {
	for _, sk := range secretAttr {
		if lk == sk || strings.HasPrefix(lk, sk+"_") || strings.HasSuffix(lk, "_"+sk) || strings.Contains(lk, "_"+sk+"_") {
			return true
		}
	}
	return false
}

func redactText(s string) string {
	s = tokenShape.ReplaceAllString(s, "[redacted]")
	return capRunes(s, ErrorCap)
}

func capRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
