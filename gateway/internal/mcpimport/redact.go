// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// digitRun matches a phone number or a long numeric customer id.
// UUIDs are left intact by redactText before this runs.
var digitRun = regexp.MustCompile(`\+?\d{8,}`)

// zaloID matches a Zalo-shaped customer id so the raw value is not printed.
var zaloID = regexp.MustCompile(`(?i)\bzalo[_:/.-]?[a-z0-9]{6,}\b`)

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func redactText(s string) string {
	if s == "" || uuidRe.MatchString(s) {
		return s
	}
	s = jwtRe.ReplaceAllStringFunc(s, func(m string) string {
		return "id:" + shortHash(m)
	})
	s = telegramRe.ReplaceAllStringFunc(s, func(m string) string {
		return "id:" + shortHash(m)
	})
	s = zaloID.ReplaceAllStringFunc(s, func(m string) string {
		return "id:" + shortHash(m)
	})
	s = digitRun.ReplaceAllStringFunc(s, func(m string) string {
		return "id:" + shortHash(m)
	})
	return s
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(sum[:])[:12]
}

func (r *Report) redact() {
	if r == nil {
		return
	}
	for i := range r.Tenants {
		r.Tenants[i].TenantID = redactText(r.Tenants[i].TenantID)
	}
	for i := range r.NeedsCredential {
		n := &r.NeedsCredential[i]
		n.TenantID = redactText(n.TenantID)
		n.SourceID = redactText(n.SourceID)
		n.SourceName = redactText(n.SourceName)
		n.GosoName = redactText(n.GosoName)
	}
}
