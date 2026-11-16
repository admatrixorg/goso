// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// Customer scope for L1 memory.
//
// One channel agent (zalo-personal, zalo-oa, telegram, …) serves every
// customer, so agent-scoped recall would carry customer A's notes into
// customer B's replies. Each channel thread gets its own session labelled
// "<channel>:<thread or user id>" (channel/*.go ensureSession), so that label
// is the customer key. It is stamped on the note when the note is written
// (memories.customer_key) and not read back from the session at recall time,
// because operators can rename a session label.

// OmniCustomerPrefix labels Omni bridge sessions of one identified customer
// conversation: "omni-chat:<tenant>:<channel type>:<channel account>:<external
// conversation>" (parts query-escaped). The bridge owns these sessions and
// writes their notes itself (see bridge/memory.go).
const OmniCustomerPrefix = "omni-chat:"

// ChannelLabelPrefixes mark sessions created by the older single-bot channels
// (catalog name + ":").
var ChannelLabelPrefixes = []string{
	"zalo-personal:", "zalo-oa:", "telegram:", "discord:", "slack:", "feishu:", "whatsapp:",
	OmniCustomerPrefix,
}

// ChannelCatalogs are goso's channel types. A tenant channel instance is named
// "<tenant>/<catalog>/<name>" (channel/instance.go) and labels its sessions
// "<tenant>/<catalog>/<name>:<chat id>". Must equal channel.Names.
var ChannelCatalogs = []string{
	"telegram", "zalo-personal", "zalo-oa", "discord", "slack", "feishu", "whatsapp",
}

// maxInstanceNameLen matches channel.maxInstanceLen.
const maxInstanceNameLen = 100

// IsOmniCustomerLabel reports an Omni bridge per-customer session label.
func IsOmniCustomerLabel(label string) bool {
	return strings.HasPrefix(label, OmniCustomerPrefix) && len(label) > len(OmniCustomerPrefix)
}

// IsChannelLabel reports a session label owned by an external channel thread:
// an older single-bot label ("telegram:…") or a tenant instance label
// ("<tenant>/<catalog>/<name>:…").
func IsChannelLabel(label string) bool {
	for _, p := range ChannelLabelPrefixes {
		if strings.HasPrefix(label, p) {
			return true
		}
	}
	_, ok := instanceLabelChat(label)
	return ok
}

// CustomerKeyForLabel returns the customer key of a channel session label
// (the label itself, e.g. "zalo-personal:123" or
// "demo/telegram/minh-an-bot:42"), or "" for web/API sessions and for a
// channel label with no thread id. Exact, case-sensitive match so it agrees
// with the backfill.
func CustomerKeyForLabel(label string) string {
	for _, p := range ChannelLabelPrefixes {
		if strings.HasPrefix(label, p) && len(label) > len(p) {
			return label
		}
	}
	if chat, ok := instanceLabelChat(label); ok && chat != "" {
		return label
	}
	return ""
}

// instanceLabelChat parses a tenant instance label
// "<tenant>/<catalog>/<name>:<chat>". ok is true when the label has that
// shape (tenant valid, catalog known, name valid); chat may then be empty.
// The name ends at its first ':'.
func instanceLabelChat(label string) (chat string, ok bool) {
	tenant, rest, found := strings.Cut(label, "/")
	if !found || !TenantOK(tenant) {
		return "", false
	}
	catalog, rest, found := strings.Cut(rest, "/")
	if !found || !isChannelCatalog(catalog) {
		return "", false
	}
	name, chat, found := strings.Cut(rest, ":")
	if !found || !instanceNameOK(name) {
		return "", false
	}
	return chat, true
}

func isChannelCatalog(name string) bool {
	for _, c := range ChannelCatalogs {
		if c == name {
			return true
		}
	}
	return false
}

// instanceNameOK mirrors channel.instanceNameOK.
func instanceNameOK(s string) bool {
	if s == "" || len(s) > maxInstanceNameLen || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r == '/' || r == '\\' || r == '%' || r == '?' || r == '#' || unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// customerKeyBackfillSQL stamps customer_key on legacy notes whose session is
// an older single-bot channel thread (backfillInstanceCustomerKeys covers
// tenant instance threads). Deterministic (key = session label, same rule as
// CustomerKeyForLabel) and idempotent (only rows with an empty key). Notes of
// sessions whose label no longer has a channel prefix stay unscoped, and
// unscoped notes are never recalled into channel sessions.
func customerKeyBackfillSQL() string {
	conds := make([]string, 0, len(ChannelLabelPrefixes))
	for _, p := range ChannelLabelPrefixes {
		if strings.ContainsAny(p, `'\`) {
			panic("channel prefix must not contain quotes")
		}
		conds = append(conds, fmt.Sprintf("(substr(s.label, 1, %d) = '%s' AND length(s.label) > %d)", len(p), p, len(p)))
	}
	return `UPDATE memories SET customer_key = (SELECT s.label FROM sessions s WHERE s.id = memories.session_id)
		WHERE customer_key = '' AND EXISTS (SELECT 1 FROM sessions s WHERE s.id = memories.session_id AND (` +
		strings.Join(conds, " OR ") + `))`
}

// backfillInstanceCustomerKeys stamps customer_key on unscoped notes whose
// session is a tenant channel instance thread
// ("<tenant>/<catalog>/<name>:<chat>"). The shape check needs Go
// (CustomerKeyForLabel), so candidates are read with a coarse LIKE and the key
// is decided here. Idempotent: only rows whose key is still empty are touched.
func (s *SQLiteStore) backfillInstanceCustomerKeys() (int64, error) {
	rows, err := s.db.Query(`SELECT m.id, s.label FROM memories m JOIN sessions s ON s.id = m.session_id
		WHERE m.customer_key = '' AND s.label LIKE '%/%/%:%'`)
	if err != nil {
		return 0, err
	}
	type stamp struct{ id, key string }
	var todo []stamp
	for rows.Next() {
		var id string
		var label sql.NullString
		if err := rows.Scan(&id, &label); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if key := CustomerKeyForLabel(label.String); key != "" {
			todo = append(todo, stamp{id: id, key: key})
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for _, t := range todo {
		res, err := s.db.Exec(`UPDATE memories SET customer_key = ? WHERE id = ? AND customer_key = ''`, t.key, t.id)
		if err != nil {
			return n, err
		}
		if k, _ := res.RowsAffected(); k > 0 {
			n += k
		}
	}
	return n, nil
}
