// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package providerimport loads GoClaw llm_providers rows into goso's
// llm_providers table. It reads an offline JSON or JSONL export and never
// opens a live GoClaw database.
//
// GoClaw stores api_base and has no model column; goso stores base_url and
// model. A GoClaw provider with no model is imported with goso's default for
// its type, because goso needs a model to address a request.
//
// The api_key is never stored or printed. goso keeps a provider key in the
// secrets table through the providers UI, and an offline import is the wrong
// place to move a credential.
package providerimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxJSONLLine = 4 << 20

// ConfigError is a bad export or denylist. The text never quotes a body.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

// Row is one llm_providers row from the GoClaw export.
type Row struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	APIBase      string `json:"api_base"`
	APIKey       string `json:"api_key"`
	Enabled      *bool  `json:"enabled"`
}

// Export is every row read from the file.
type Export struct {
	Rows []Row
}

// ReadExport reads a JSON object with an llm_providers array, a bare JSON
// array, or JSONL. A blank line is ignored.
func ReadExport(r io.Reader) (*Export, error) {
	if r == nil {
		return nil, errConfig("export is required")
	}
	raw, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, errConfig("export is unreadable")
	}
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	if len(raw) == 0 {
		return &Export{}, nil
	}
	// A JSON object may be the documented {"llm_providers":[...]} document or
	// the first line of JSONL, and both start with '{'. Try the document
	// first; a JSONL body fails that unmarshal and falls through.
	if raw[0] == '{' {
		var doc struct {
			Providers []Row `json:"llm_providers"`
		}
		if err := json.Unmarshal(raw, &doc); err == nil && doc.Providers != nil {
			return &Export{Rows: doc.Providers}, nil
		}
	}
	if raw[0] == '[' {
		var rows []Row
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, errConfig("export is not the documented JSON")
		}
		return &Export{Rows: rows}, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), maxJSONLLine)
	line := 0
	out := &Export{}
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var row Row
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, errConfig(fmt.Sprintf("line %d is not JSON", line))
		}
		out.Rows = append(out.Rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("export is unreadable")
	}
	return out, nil
}
