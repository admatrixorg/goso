// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parsed is a validated schedule: interval every:Nm|Nh, or a 5-field cron.
type Parsed struct {
	Interval time.Duration
	Fields   [5]Field
	Five     bool
}

// Field is one cron column: star, exact value, or */step.
type Field struct {
	Any   bool
	Step  int
	Exact int
}

var errInvalidSpec = errors.New("invalid spec")

// ParseSpec accepts `every:Nm|Nh` (n>=1) or a 5-field cron (`min hour dom month dow`).
// Five-field columns support `*`, a decimal, or `*/n` only.
func ParseSpec(s string) (Parsed, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Parsed{}, errInvalidSpec
	}
	if strings.HasPrefix(strings.ToLower(s), "every:") {
		return parseInterval(s)
	}
	return parseFive(s)
}

func parseInterval(s string) (Parsed, error) {
	rest := strings.TrimSpace(s[len("every:"):])
	if rest == "" {
		return Parsed{}, errInvalidSpec
	}
	unit := rest[len(rest)-1]
	num := rest[:len(rest)-1]
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 {
		return Parsed{}, errInvalidSpec
	}
	var d time.Duration
	switch unit {
	case 'm', 'M':
		d = time.Duration(n) * time.Minute
	case 'h', 'H':
		d = time.Duration(n) * time.Hour
	default:
		return Parsed{}, errInvalidSpec
	}
	return Parsed{Interval: d}, nil
}

func parseFive(s string) (Parsed, error) {
	parts := strings.Fields(s)
	if len(parts) != 5 {
		return Parsed{}, errInvalidSpec
	}
	bounds := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	var p Parsed
	p.Five = true
	for i, raw := range parts {
		f, err := parseField(raw, bounds[i][0], bounds[i][1])
		if err != nil {
			return Parsed{}, err
		}
		p.Fields[i] = f
	}
	return p, nil
}

func parseField(raw string, min, max int) (Field, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return Field{Any: true}, nil
	}
	if strings.HasPrefix(raw, "*/") {
		n, err := strconv.Atoi(raw[2:])
		if err != nil || n < 1 {
			return Field{}, errInvalidSpec
		}
		return Field{Any: true, Step: n}, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return Field{}, errInvalidSpec
	}
	if n < min || n > max {
		return Field{}, fmt.Errorf("%w: field out of range", errInvalidSpec)
	}
	if max == 7 && n == 7 {
		n = 0
	}
	return Field{Exact: n}, nil
}

func matchField(f Field, v int) bool {
	if f.Any {
		if f.Step > 0 {
			return v%f.Step == 0
		}
		return true
	}
	return v == f.Exact
}

// Due reports whether spec should fire at now given last_run (nil = never).
func Due(spec string, lastRun *time.Time, now time.Time) bool {
	p, err := ParseSpec(spec)
	if err != nil {
		return false
	}
	now = now.UTC()
	if p.Interval > 0 {
		if lastRun == nil || lastRun.IsZero() {
			return true
		}
		return !now.Before(lastRun.UTC().Add(p.Interval))
	}
	if !p.Five {
		return false
	}
	min := now.Truncate(time.Minute)
	if lastRun != nil && !lastRun.IsZero() && lastRun.UTC().Truncate(time.Minute).Equal(min) {
		return false
	}
	dow := int(now.Weekday()) // Sunday = 0
	return matchField(p.Fields[0], now.Minute()) &&
		matchField(p.Fields[1], now.Hour()) &&
		matchField(p.Fields[2], now.Day()) &&
		matchField(p.Fields[3], int(now.Month())) &&
		matchField(p.Fields[4], dow)
}
