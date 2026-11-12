// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package kgimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const maxJSONLLine = 16 << 20

// row is one JSONL object. Field names are the GoClaw columns from
// \d kg_entities and \d kg_relations (2026-11-12). embedding, tsv, and
// any other column are not fields, so the decoder drops them.
//
// kg_relations has no description and no custom_scope. custom_scope is
// accepted on a line anyway: a non-empty value is the team/scope guard.
type row struct {
	Table          string          `json:"table"`
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	AgentID        string          `json:"agent_id"`
	UserID         *string         `json:"user_id"`
	TeamID         *string         `json:"team_id"`
	CustomScope    *string         `json:"custom_scope"`
	Name           string          `json:"name"`
	EntityType     string          `json:"entity_type"`
	Description    string          `json:"description"`
	Properties     json.RawMessage `json:"properties"`
	CreatedAt      *string         `json:"created_at"`
	ValidFrom      *string         `json:"valid_from"`
	ValidUntil     *string         `json:"valid_until"`
	SourceEntityID string          `json:"source_entity_id"`
	RelationType   string          `json:"relation_type"`
	TargetEntityID string          `json:"target_entity_id"`
}

// Export is a JSONL file of kg_entities and kg_relations. It is not a
// database connection.
type Export struct {
	Entities  []row
	Relations []row
}

// ReadExport reads JSONL. A blank line is ignored. A line that is not
// JSON fails the read before any write. embedding, tsv, and any other
// column that is not on row are ignored.
func ReadExport(r io.Reader) (*Export, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxJSONLLine)
	var ex Export
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		if !utf8.ValidString(text) {
			return nil, fmt.Errorf("line %d: invalid json", line)
		}
		var item row
		if err := json.Unmarshal([]byte(text), &item); err != nil {
			return nil, fmt.Errorf("line %d: invalid json", line)
		}
		switch item.Table {
		case "kg_entities":
			ex.Entities = append(ex.Entities, item)
		case "kg_relations":
			ex.Relations = append(ex.Relations, item)
		default:
			return nil, fmt.Errorf("line %d: unknown table", line)
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, errors.New("export line is too long")
		}
		return nil, errors.New("read export failed")
	}
	return &ex, nil
}

func (r row) userID() string      { return deref(r.UserID) }
func (r row) teamID() string      { return deref(r.TeamID) }
func (r row) customScope() string { return deref(r.CustomScope) }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// compactProperties returns canonical compact JSON. null and {} are empty.
// Invalid JSON is an error. The result is not an embedding.
func compactProperties(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	if m, ok := v.(map[string]any); ok && len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func entityBody(description, props string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return props
	}
	if props == "" {
		return description
	}
	return description + "\n" + props
}

func parseStamp(p *string) (time.Time, bool, error) {
	if p == nil {
		return time.Time{}, false, nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return t.UTC(), true, nil
}
