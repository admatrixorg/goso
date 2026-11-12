// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

// MaxExportBytes is the largest export this tool will read.
const MaxExportBytes = 32 << 20

// MaxFrontmatterBytes is the largest agents.frontmatter stored in instructions.
const MaxFrontmatterBytes = 32 << 10

type rawRow struct {
	table  string
	fields map[string]json.RawMessage
}

// Export is one offline file. Column values stay raw until a row is imported.
type Export struct {
	rows []rawRow
}

// ReadExport parses a JSON object of table arrays, or JSONL with a table name
// on each line. A JSON error is not returned: the message would quote the file.
func ReadExport(r io.Reader) (*Export, error) {
	body, err := readLimited(r)
	if err != nil {
		return nil, err
	}
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return nil, &ConfigError{Msg: "export is empty"}
	}
	if trim[0] == '[' {
		return nil, &ConfigError{Msg: "export must be a JSON object or JSONL"}
	}
	if trim[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trim, &obj); err == nil && documentShape(obj) {
			return exportFromDocument(obj)
		}
	}
	return exportFromJSONL(trim)
}

func readLimited(r io.Reader) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(r, MaxExportBytes+1))
	if err != nil {
		return nil, &ConfigError{Msg: "export is not valid JSON"}
	}
	if len(buf) > MaxExportBytes {
		return nil, &ConfigError{Msg: "export is too large"}
	}
	return buf, nil
}

func documentShape(obj map[string]json.RawMessage) bool {
	if len(obj) == 0 {
		return true
	}
	for _, raw := range obj {
		trim := bytes.TrimSpace(raw)
		if len(trim) > 0 && trim[0] == '[' {
			return true
		}
	}
	return false
}

func exportFromDocument(obj map[string]json.RawMessage) (*Export, error) {
	ex := &Export{}
	// Map iteration order does not matter. Each array keeps its own order.
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		raw := bytes.TrimSpace(obj[name])
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			continue
		}
		if raw[0] != '[' {
			return nil, &ConfigError{Msg: "export must be a JSON object or JSONL"}
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, &ConfigError{Msg: "export is not valid JSON"}
		}
		for _, fields := range rows {
			if fields == nil {
				return nil, &ConfigError{Msg: "export is not valid JSON"}
			}
			ex.rows = append(ex.rows, rawRow{table: name, fields: fields})
		}
	}
	return ex, nil
}

func exportFromJSONL(data []byte) (*Export, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), MaxExportBytes)
	ex := &Export{}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
			return nil, &ConfigError{Msg: "export is not valid JSON"}
		}
		table := jsonlTable(fields)
		if table == "" {
			return nil, &ConfigError{Msg: "export must be a JSON object or JSONL"}
		}
		delete(fields, "_table")
		delete(fields, "table")
		ex.rows = append(ex.rows, rawRow{table: table, fields: fields})
	}
	if err := sc.Err(); err != nil {
		return nil, &ConfigError{Msg: "export is not valid JSON"}
	}
	if len(ex.rows) == 0 {
		return nil, &ConfigError{Msg: "export is empty"}
	}
	return ex, nil
}

func jsonlTable(fields map[string]json.RawMessage) string {
	if s, ok := jsonString(fields, "_table"); ok && s != "" {
		return s
	}
	s, _ := jsonString(fields, "table")
	return s
}

func (e *Export) rowsOf(table string) []rawRow {
	if e == nil {
		return nil
	}
	var out []rawRow
	for _, row := range e.rows {
		if row.table == table {
			out = append(out, row)
		}
	}
	return out
}

func jsonString(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", true
	}
	if raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}
