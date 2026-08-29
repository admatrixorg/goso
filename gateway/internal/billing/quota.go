// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package billing

import (
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
)

// QuotaWindow is the daily used/limit pair on GET /api/quota.
type QuotaWindow struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// QuotaStatus is the GET /api/quota payload (SPEC 027).
type QuotaStatus struct {
	Enabled           bool        `json:"enabled"`
	RequestsToday     int         `json:"requestsToday"`
	InputTokensToday  int         `json:"inputTokensToday"`
	OutputTokensToday int         `json:"outputTokensToday"`
	Day               QuotaWindow `json:"day"`
}

// DayLimit reads GOSO_QUOTA_DAY. 0, empty, unset, or invalid = unlimited.
func DayLimit() int {
	v := strings.TrimSpace(config.Lookup("GOSO_QUOTA_DAY"))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// TodayTotals aggregates usage for the UTC calendar day containing now.
func (s *Store) TodayTotals(now time.Time) Summary {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return s.Query(Query{From: from, To: from.Add(24 * time.Hour)})
}

// QuotaStatus builds the GET /api/quota payload for the UTC day of now.
func (s *Store) QuotaStatus(now time.Time) QuotaStatus {
	today := s.TodayTotals(now)
	limit := DayLimit()
	return QuotaStatus{
		Enabled:           limit > 0,
		RequestsToday:     today.Calls,
		InputTokensToday:  today.PromptTokens,
		OutputTokensToday: today.CompletionTokens,
		Day: QuotaWindow{
			Used:  today.TotalTokens,
			Limit: limit,
		},
	}
}

// Exceeded reports whether another chat should be rejected with HTTP 429.
//
// Rule (SPEC 027 AC-03): when limit > 0, enforce on total_tokens today.
// If a chat records 0 tokens (echo/estimate can be 0), also count Calls
// so a 1-request cap is still testable:
//
//	limit > 0 && (today.TotalTokens >= limit || (today.TotalTokens == 0 && today.Calls >= limit))
func Exceeded(today Summary, limit int) bool {
	if limit <= 0 {
		return false
	}
	if today.TotalTokens >= limit {
		return true
	}
	return today.TotalTokens == 0 && today.Calls >= limit
}

// SecondsUntilUTCMidnight is the Retry-After value (ceil seconds to next UTC midnight).
func SecondsUntilUTCMidnight(now time.Time) int {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	d := next.Sub(now)
	sec := int((d + time.Second - 1) / time.Second)
	if sec < 1 {
		sec = 1
	}
	return sec
}
