// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package cron

import (
	"testing"
	"time"
)

func TestParseSpec_Interval(t *testing.T) {
	p, err := ParseSpec("every:5m")
	if err != nil || p.Interval != 5*time.Minute || p.Five {
		t.Fatalf("every:5m %+v %v", p, err)
	}
	p, err = ParseSpec("every:1h")
	if err != nil || p.Interval != time.Hour {
		t.Fatalf("every:1h %+v %v", p, err)
	}
	if _, err := ParseSpec("every:0m"); err == nil {
		t.Fatal("every:0m")
	}
	if _, err := ParseSpec("every:1"); err == nil {
		t.Fatal("every:1")
	}
	if _, err := ParseSpec("every:"); err == nil {
		t.Fatal("every:")
	}
	if _, err := ParseSpec("not-a-spec"); err == nil {
		t.Fatal("garbage")
	}
}

func TestParseSpec_FiveField(t *testing.T) {
	p, err := ParseSpec("*/5 * * * *")
	if err != nil || !p.Five || p.Fields[0].Step != 5 {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := ParseSpec("* * *"); err == nil {
		t.Fatal("3-field")
	}
	if _, err := ParseSpec("60 * * * *"); err == nil {
		t.Fatal("minute 60")
	}
}

func TestDue_Interval(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	if !Due("every:1h", nil, now) {
		t.Fatal("nil last_run should be due")
	}
	last := now.Add(-30 * time.Minute)
	if Due("every:1h", &last, now) {
		t.Fatal("30m ago not due for 1h")
	}
	last = now.Add(-time.Hour)
	if !Due("every:1h", &last, now) {
		t.Fatal("1h ago should be due")
	}
	if Due("bogus", nil, now) {
		t.Fatal("invalid spec is never due")
	}
}

func TestDue_FiveFieldSameMinute(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 30, 0, time.UTC)
	if !Due("* * * * *", nil, now) {
		t.Fatal("star should match")
	}
	last := now.Add(-10 * time.Second)
	if Due("* * * * *", &last, now) {
		t.Fatal("same truncated minute must not refire")
	}
	last = now.Add(-time.Minute)
	if !Due("* * * * *", &last, now) {
		t.Fatal("previous minute should be due")
	}
}
