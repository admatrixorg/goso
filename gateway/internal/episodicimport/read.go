// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package episodicimport

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// tableName is the only GoClaw table this importer reads.
const tableName = "episodic_summaries"

// row is one JSONL object. The decoder drops every field that is not listed,
// including l0_abstract, embedding, source_type, source_id, turn_count,
// token_count, search_vector, promoted_at, recall_count, recall_score, and
// last_recalled_at.
//
// team_id and custom_scope are not columns of episodic_summaries. They are
// decoded only so a non-empty value can increment the team/scope guard.
type row struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	AgentID     string          `json:"agent_id"`
	UserID      string          `json:"user_id"`
	SessionKey  string          `json:"session_key"`
	Summary     string          `json:"summary"`
	KeyTopics   json.RawMessage `json:"key_topics"`
	CreatedAt   string          `json:"created_at"`
	ExpiresAt   *string         `json:"expires_at"`
	TeamID      *string         `json:"team_id"`
	CustomScope *string         `json:"custom_scope"`
}

// Parsed is one decoded export line. Callers pass it to Import.
type Parsed = parsed

type parsed struct {
	row
	topics     []string
	created    time.Time
	expires    *time.Time
	badExpires bool
	badUser    bool
	userID     string
	scopeSet   bool
	line       int
}

// ReadExport reads a JSONL file of episodic_summaries objects. It does not
// open a database. A line over 8 MiB is an error. Blank lines are ignored.
func ReadExport(r io.Reader) ([]Parsed, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	var out []Parsed
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if line == "" {
			continue
		}
		p, err := parseLine(n, line)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, config("export read failed")
	}
	return out, nil
}

func parseLine(n int, line string) (parsed, error) {
	if !utf8.ValidString(line) {
		return parsed{}, configf("export line %d is not UTF-8", n)
	}
	var row row
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		return parsed{}, configf("export line %d is not JSON", n)
	}
	topics, err := parseTopics(row.KeyTopics)
	if err != nil {
		return parsed{}, configf("export line %d key_topics is not a string array", n)
	}
	p := parsed{
		row:      row,
		topics:   topics,
		line:     n,
		userID:   strings.TrimSpace(row.UserID),
		scopeSet: nonemptyPtr(row.TeamID) || nonemptyPtr(row.CustomScope),
	}
	if strings.ContainsAny(p.userID, "\n\r\x00") {
		p.badUser = true
	}
	if strings.TrimSpace(row.CreatedAt) != "" {
		t, err := parseStamp(row.CreatedAt)
		if err == nil {
			p.created = t
		}
	}
	if row.ExpiresAt != nil && strings.TrimSpace(*row.ExpiresAt) != "" {
		t, err := parseStamp(*row.ExpiresAt)
		if err != nil {
			p.badExpires = true
		} else {
			p.expires = &t
		}
	}
	return p, nil
}

func parseTopics(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var topics []string
	if err := json.Unmarshal(raw, &topics); err != nil {
		return nil, err
	}
	return topics, nil
}

func parseStamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, config("time")
}

func nonemptyPtr(p *string) bool {
	return p != nil && strings.TrimSpace(*p) != ""
}
