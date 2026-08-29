// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package billing

import (
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
)

func TestDayLimit(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	if DayLimit() != 0 {
		t.Fatalf("unset want 0, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "0")
	if DayLimit() != 0 {
		t.Fatalf("0 want 0, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "  ")
	if DayLimit() != 0 {
		t.Fatalf("blank want 0, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "nope")
	if DayLimit() != 0 {
		t.Fatalf("invalid want 0, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "-3")
	if DayLimit() != 0 {
		t.Fatalf("negative want 0, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "1")
	if DayLimit() != 1 {
		t.Fatalf("1 want 1, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", " 42 ")
	if DayLimit() != 42 {
		t.Fatalf("42 want 42, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "")
	t.Cleanup(config.ResetOverlay)
	config.SetOverlay(map[string]string{"quota_day": "8"})
	if DayLimit() != 8 {
		t.Fatalf("overlay want 8, got %d", DayLimit())
	}
	t.Setenv("GOSO_QUOTA_DAY", "2")
	if DayLimit() != 2 {
		t.Fatalf("env must still win overlay, got %d", DayLimit())
	}
}

func TestTodayTotals(t *testing.T) {
	s := New()
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	s.Add(Record{AgentID: "a", Provider: "echo", PromptTokens: 10, CompletionTokens: 5, CreatedAt: now})
	s.Add(Record{AgentID: "a", Provider: "echo", PromptTokens: 2, CompletionTokens: 1, CreatedAt: now.Add(-24 * time.Hour)})
	s.Add(Record{AgentID: "a", Provider: "echo", PromptTokens: 99, CompletionTokens: 1, CreatedAt: now.Add(24 * time.Hour)})

	got := s.TodayTotals(now)
	if got.Calls != 1 || got.PromptTokens != 10 || got.CompletionTokens != 5 || got.TotalTokens != 15 {
		t.Fatalf("today %+v", got)
	}
	if got.From != "2026-08-26" || got.To != "2026-08-26" {
		t.Fatalf("labels %+v", got)
	}
}

func TestQuotaStatus_Disabled(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	s := New()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	s.Add(Record{PromptTokens: 3, CompletionTokens: 1, CreatedAt: now})
	st := s.QuotaStatus(now)
	if st.Enabled {
		t.Fatalf("enabled %+v", st)
	}
	if st.RequestsToday != 1 || st.InputTokensToday != 3 || st.OutputTokensToday != 1 {
		t.Fatalf("counts %+v", st)
	}
	if st.Day.Used != 4 || st.Day.Limit != 0 {
		t.Fatalf("day %+v", st.Day)
	}
}

func TestQuotaStatus_Enabled(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "100")
	s := New()
	now := time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC)
	s.Add(Record{PromptTokens: 7, CompletionTokens: 2, CreatedAt: now})
	st := s.QuotaStatus(now)
	if !st.Enabled {
		t.Fatalf("want enabled %+v", st)
	}
	if st.RequestsToday != 1 || st.InputTokensToday != 7 || st.OutputTokensToday != 2 {
		t.Fatalf("counts %+v", st)
	}
	if st.Day.Used != 9 || st.Day.Limit != 100 {
		t.Fatalf("day %+v", st.Day)
	}
}

func TestQuotaStatus_NilStore(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "10")
	var s *Store
	st := s.QuotaStatus(time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC))
	if !st.Enabled || st.RequestsToday != 0 || st.Day.Used != 0 || st.Day.Limit != 10 {
		t.Fatalf("nil store %+v", st)
	}
}

func TestExceeded(t *testing.T) {
	if Exceeded(Summary{TotalTokens: 99, Calls: 50}, 0) {
		t.Fatal("disabled must not exceed")
	}
	if Exceeded(Summary{TotalTokens: 0, Calls: 0}, 1) {
		t.Fatal("empty must not exceed")
	}
	if !Exceeded(Summary{TotalTokens: 1, Calls: 1}, 1) {
		t.Fatal("total_tokens >= limit")
	}
	if !Exceeded(Summary{TotalTokens: 10, Calls: 1}, 1) {
		t.Fatal("over limit")
	}
	if Exceeded(Summary{TotalTokens: 5, Calls: 3}, 10) {
		t.Fatal("under token cap")
	}
	// 0-token chats: count Calls so AC-03 is testable.
	if !Exceeded(Summary{TotalTokens: 0, Calls: 1}, 1) {
		t.Fatal("zero-token calls >= limit")
	}
	if Exceeded(Summary{TotalTokens: 0, Calls: 0}, 1) {
		t.Fatal("zero calls under limit")
	}
	if Exceeded(Summary{TotalTokens: 1, Calls: 5}, 10) {
		t.Fatal("nonzero tokens under limit must not use Calls fallback")
	}
}

func TestSecondsUntilUTCMidnight(t *testing.T) {
	now := time.Date(2026, 8, 26, 23, 59, 50, 0, time.UTC)
	got := SecondsUntilUTCMidnight(now)
	if got != 10 {
		t.Fatalf("got %d want 10", got)
	}
	now = time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	got = SecondsUntilUTCMidnight(now)
	if got != 86400 {
		t.Fatalf("midnight got %d want 86400", got)
	}
	frac := time.Date(2026, 8, 26, 23, 59, 59, 500e6, time.UTC)
	got = SecondsUntilUTCMidnight(frac)
	if got != 1 {
		t.Fatalf("frac got %d want 1", got)
	}
}
