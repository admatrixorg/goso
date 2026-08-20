// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package billing

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUsageQuery(t *testing.T) {
	s := New()
	day := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	s.Add(Record{AgentID: "ag-1", Provider: "echo", PromptTokens: 10, CompletionTokens: 4, CreatedAt: day})
	s.Add(Record{AgentID: "ag-1", Provider: "openai", PromptTokens: 20, CompletionTokens: 8, CreatedAt: day})
	s.Add(Record{AgentID: "ag-2", Provider: "echo", PromptTokens: 5, CompletionTokens: 1, CreatedAt: day})

	all := s.Query(Query{})
	if all.Calls != 3 || all.TotalTokens != 10+4+20+8+5+1 {
		t.Fatalf("all %+v", all)
	}
	if len(all.ByDay) != 1 || all.ByDay[0].Date != "2026-08-20" || all.ByDay[0].Calls != 3 {
		t.Fatalf("by_day %+v", all.ByDay)
	}

	one := s.Query(Query{AgentID: "ag-1"})
	if one.Calls != 2 || one.PromptTokens != 30 || one.CompletionTokens != 12 || one.TotalTokens != 42 {
		t.Fatalf("agent %+v", one)
	}

	echo := s.Query(Query{AgentID: "ag-1", Provider: "echo"})
	if echo.Calls != 1 || echo.Provider != "echo" || echo.TotalTokens != 14 {
		t.Fatalf("provider %+v", echo)
	}

	none := s.Query(Query{Provider: "anthropic"})
	if none.Calls != 0 || none.TotalTokens != 0 || none.ByDay == nil {
		t.Fatalf("empty %+v", none)
	}
}

func TestUsageQuery_DateRange(t *testing.T) {
	s := New()
	s.Add(Record{AgentID: "a", Provider: "echo", PromptTokens: 4, CompletionTokens: 0, CreatedAt: time.Date(2026, 8, 19, 23, 0, 0, 0, time.UTC)})
	s.Add(Record{AgentID: "a", Provider: "echo", PromptTokens: 8, CompletionTokens: 0, CreatedAt: time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)})
	s.Add(Record{AgentID: "a", Provider: "echo", PromptTokens: 16, CompletionTokens: 0, CreatedAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)})

	from := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC) // exclusive
	got := s.Query(Query{From: from, To: to})
	if got.Calls != 1 || got.PromptTokens != 8 {
		t.Fatalf("range %+v", got)
	}
	if got.From != "2026-08-20" || got.To != "2026-08-20" {
		t.Fatalf("from/to labels %+v", got)
	}
}

func TestUsageSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer s.Close()

	day := time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC)
	s.Add(Record{AgentID: "ag-sq", Provider: "anthropic", PromptTokens: 12, CompletionTokens: 3, Estimated: false, CreatedAt: day})
	s.Add(Record{AgentID: "ag-sq", Provider: "echo", PromptTokens: 4, CompletionTokens: 2, Estimated: true, CreatedAt: day})

	got := s.Query(Query{AgentID: "ag-sq", Provider: "anthropic"})
	if got.Calls != 1 || got.PromptTokens != 12 || got.CompletionTokens != 3 || got.TotalTokens != 15 {
		t.Fatalf("sqlite query %+v", got)
	}

	// reopen
	_ = s.Close()
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	all := s2.Query(Query{AgentID: "ag-sq"})
	if all.Calls != 2 || all.TotalTokens != 21 {
		t.Fatalf("persist %+v", all)
	}
}

func TestUsageNilStore(t *testing.T) {
	var s *Store
	s.AddCall("a", "echo", 1, 1, true)
	sum := s.Query(Query{})
	if sum.Calls != 0 {
		t.Fatalf("nil store %+v", sum)
	}
}
