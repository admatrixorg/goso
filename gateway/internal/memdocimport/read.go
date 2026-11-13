// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// row is one JSONL object. Chunks and documents share the reader so a chunk
// file cannot be stored as a document. Columns match memory_documents and the
// chunk markers that keep memory_chunks out.
type row struct {
	line      int
	chunk     bool
	id        string
	agentID   string
	userID    string
	userSet   bool
	path      string
	content   string
	hash      string
	createdAt string
	updatedAt string
	teamSet   bool
	tenantID  string
	scopeSet  bool
}

// ReadJSONL reads a memory_documents export. It does not open a database.
// A line that is not a JSON object fails the read. The line text is not
// included in the error.
func ReadJSONL(r io.Reader) ([]row, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), security.MaxAPIBody+64*1024)
	var out []row
	n := 0
	seen := false
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if !seen {
			line = bytes.TrimPrefix(line, []byte{0xEF, 0xBB, 0xBF})
			seen = true
		}
		n++
		if len(line) > 0 && line[0] == '[' {
			return nil, errConfig("export must be JSONL, one object per line")
		}
		item, err := parseLine(line)
		if err != nil {
			return nil, errConfig("export line is not a JSON object")
		}
		item.line = n
		out = append(out, item)
	}
	if err := sc.Err(); err != nil {
		if strings.Contains(err.Error(), "token too long") {
			return nil, errConfig("export line exceeds 1 MiB")
		}
		return nil, errConfig("export could not be read")
	}
	return out, nil
}

func parseLine(line []byte) (row, error) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return row{}, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return row{}, errConfig("export line is not a JSON object")
	}
	item := row{chunk: isChunk(raw)}
	var err error
	if item.id, err = jsonString(raw, "id"); err != nil {
		return row{}, err
	}
	if item.agentID, err = jsonString(raw, "agent_id"); err != nil {
		return row{}, err
	}
	if item.userID, item.userSet, err = jsonNullable(raw, "user_id"); err != nil {
		return row{}, err
	}
	if item.path, err = jsonString(raw, "path"); err != nil {
		return row{}, err
	}
	if item.content, err = jsonString(raw, "content"); err != nil {
		return row{}, err
	}
	if item.hash, err = jsonString(raw, "hash"); err != nil {
		return row{}, err
	}
	if item.createdAt, err = jsonString(raw, "created_at"); err != nil {
		return row{}, err
	}
	if item.updatedAt, err = jsonString(raw, "updated_at"); err != nil {
		return row{}, err
	}
	if _, item.teamSet, err = jsonNullable(raw, "team_id"); err != nil {
		return row{}, err
	}
	if item.tenantID, err = jsonString(raw, "tenant_id"); err != nil {
		return row{}, err
	}
	if _, item.scopeSet, err = jsonNullable(raw, "custom_scope"); err != nil {
		return row{}, err
	}
	item.userID = strings.TrimSpace(item.userID)
	if item.userID == "" {
		item.userSet = false
	}
	return item, nil
}

func isChunk(raw map[string]json.RawMessage) bool {
	for _, key := range []string{"document_id", "start_line", "end_line", "embedding", "tsv", "text"} {
		if _, ok := raw[key]; ok {
			return true
		}
	}
	return false
}

func jsonString(raw map[string]json.RawMessage, key string) (string, error) {
	v, ok := raw[key]
	if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", errConfig("export line is not a JSON object")
	}
	return s, nil
}

// jsonNullable reports whether a SQL NULL-able column is set to a non-empty string.
// JSON null, a missing key, and "" are empty. Any other JSON type fails the line.
func jsonNullable(raw map[string]json.RawMessage, key string) (string, bool, error) {
	v, ok := raw[key]
	if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", false, errConfig("export line is not a JSON object")
	}
	s = strings.TrimSpace(s)
	return s, s != "", nil
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func textOK(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
