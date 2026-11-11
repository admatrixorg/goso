// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

// composeBody stores the latest version text. Summary and non-secret metadata
// have no columns on vault_docs, so they ride in a stable trailer. The trailer
// is omitted when both are empty, and the body is then the version text alone.
func composeBody(content, summary string, meta map[string]any) string {
	content = normalizeNewlines(content)
	summary = strings.TrimSpace(normalizeNewlines(summary))
	if summary == "" && len(meta) == 0 {
		return content
	}
	var b strings.Builder
	b.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\n---\n")
	if summary != "" {
		b.WriteString("summary: ")
		b.WriteString(oneLine(summary))
		b.WriteByte('\n')
	}
	if len(meta) > 0 {
		raw, err := json.Marshal(meta)
		if err == nil && string(raw) != "{}" && string(raw) != "null" {
			b.WriteString("meta: ")
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func hashBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// importMarker records provenance in the existing body. vault_docs has no
// source-id column, and a new table would sit beside the vault the importer
// is not allowed to extend. The marker is the last line:
//
//	<!-- goso-import src=<id> sum=<sha256> -->
//
// sum is sha256(title + "\n" + canonical body prefix). A later run may
// overwrite the row only when the source id matches and that sum still
// matches the stored title and prefix. A hand edit, a goso-created note, or
// another source id at the same path fails the check.
func stampImport(title, plain, src string) string {
	prefix := canonicalPrefix(plain)
	sum := hashBody(title + "\n" + prefix)
	marker := "<!-- goso-import src=" + src + " sum=" + sum + " -->"
	return prefix + marker + "\n"
}

func canonicalPrefix(plain string) string {
	if plain == "" || strings.HasSuffix(plain, "\n") {
		return plain
	}
	return plain + "\n"
}

func splitImportMarker(body string) (prefix, src, sum string, ok bool) {
	if !strings.HasSuffix(body, "\n") {
		return "", "", "", false
	}
	trimmed := strings.TrimSuffix(body, "\n")
	line := trimmed
	prefix = ""
	if i := strings.LastIndex(trimmed, "\n"); i >= 0 {
		line = trimmed[i+1:]
		prefix = trimmed[:i+1]
	}
	src, sum, ok = parseMarkerLine(line)
	return prefix, src, sum, ok
}

func parseMarkerLine(line string) (src, sum string, ok bool) {
	const pre = "<!-- goso-import src="
	const mid = " sum="
	const suf = " -->"
	if !strings.HasPrefix(line, pre) || !strings.HasSuffix(line, suf) {
		return "", "", false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(line, pre), suf)
	src, sum, found := strings.Cut(inner, mid)
	if !found || !sourceIDOK(src) || len(sum) != 64 || !hexOnly(sum) {
		return "", "", false
	}
	return src, sum, true
}

func sourceIDOK(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == ':' || r == '-':
		default:
			return false
		}
	}
	return true
}

func hexOnly(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

// importUntouched reports that this importer wrote the row from srcID and
// neither the title nor the body prefix has changed since that write.
func importUntouched(title, body, srcID string) bool {
	prefix, src, sum, ok := splitImportMarker(body)
	if !ok || src != srcID {
		return false
	}
	return hashBody(title+"\n"+prefix) == sum
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

// scrubMeta drops secret-named keys, channel config, and credential-shaped
// values. Map key order is not significant; encoding/json sorts keys.
func scrubMeta(raw json.RawMessage, extra map[string]string) map[string]any {
	out := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil {
			if m, ok := v.(map[string]any); ok {
				out = scrubMap(m)
			}
		}
	}
	for k, val := range extra {
		if strings.TrimSpace(val) == "" || secretKey(k) {
			continue
		}
		if _, exists := out[k]; exists {
			continue
		}
		out[k] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func scrubMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if secretKey(k) || vectorKey(k) {
			continue
		}
		cleaned, keep := scrubValue(v)
		if !keep {
			continue
		}
		out[k] = cleaned
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func scrubValue(v any) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		m := scrubMap(t)
		if len(m) == 0 {
			return nil, false
		}
		return m, true
	case []any:
		var items []any
		for _, item := range t {
			cleaned, keep := scrubValue(item)
			if keep {
				items = append(items, cleaned)
			}
		}
		if len(items) == 0 {
			return nil, false
		}
		return items, true
	case string:
		if credentialShaped(t) {
			return nil, false
		}
		return t, true
	case nil:
		return nil, false
	default:
		return t, true
	}
}

func secretKey(k string) bool {
	low := strings.ToLower(strings.TrimSpace(k))
	if low == "" {
		return true
	}
	flat := strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', ' ', '.':
			return -1
		default:
			return r
		}
	}, low)
	for _, frag := range []string{
		"password", "passwd", "secret", "token", "credential", "apikey",
		"privatekey", "accesskey", "authorization", "cookie", "webhook",
		"hmac", "bearer", "signing", "channel",
	} {
		if strings.Contains(flat, frag) {
			return true
		}
	}
	for _, part := range strings.FieldsFunc(low, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '.'
	}) {
		switch part {
		case "env", "header", "headers", "auth":
			return true
		}
	}
	return false
}

// vectorKey drops embedding and tsv if they were nested inside metadata.
func vectorKey(k string) bool {
	flat := strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', ' ', '.':
			return -1
		default:
			return r
		}
	}, strings.ToLower(strings.TrimSpace(k)))
	return flat == "tsv" || flat == "tsvector" || strings.Contains(flat, "embedding")
}

func credentialShaped(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "bearer ") {
		return true
	}
	for _, prefix := range []string{"sk-", "ghp_", "xoxb-", "xoxp-", "xoxa-", "xoxr-", "xoxs-"} {
		if strings.Contains(low, prefix) {
			return true
		}
	}
	if strings.Contains(s, "://") && strings.Contains(s, "@") {
		for _, scheme := range []string{"postgres://", "postgresql://", "mysql://", "mongodb://", "redis://", "amqp://"} {
			if strings.Contains(low, scheme) {
				return true
			}
		}
	}
	return false
}

// linkInner maps a GoClaw link_type and context onto the inner text of goso's
// existing [[raw]] field. Resolution uses the goso id (the text before "|").
func linkInner(gosoID, linkType, context string) string {
	gosoID = strings.TrimSpace(gosoID)
	alias := sanitizeType(linkType)
	ctx := sanitizeContext(context)
	if ctx != "" {
		alias += ": " + ctx
	}
	return gosoID + "|" + alias
}

func storedRaw(inner string) string {
	return "[[" + strings.TrimSpace(inner) + "]]"
}

func sanitizeType(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	if b.Len() == 0 {
		return "wikilink"
	}
	return b.String()
}

func sanitizeContext(s string) string {
	s = scrubContext(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '[', ']', '|', '\n', '\r', '\t':
			return ' '
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 180 {
		r := []rune(s)
		s = string(r[:180])
	}
	return strings.TrimSpace(s)
}

func scrubContext(s string) string {
	if credentialShaped(s) {
		return ""
	}
	low := strings.ToLower(s)
	cut := len(s)
	for _, marker := range []string{"sk-", "ghp_", "xoxb-", "xoxp-", "xoxa-", "xoxr-", "xoxs-", "bearer ", "postgres://", "postgresql://", "mysql://"} {
		if i := strings.Index(low, marker); i >= 0 && i < cut {
			cut = i
		}
	}
	return strings.TrimSpace(s[:cut])
}

func parseTS(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05.999999Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
