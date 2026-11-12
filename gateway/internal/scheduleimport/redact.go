// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// identDigits matches a phone number or a long numeric customer id.
// Shorter numbers and ids that contain letters stay as they are.
var identDigits = regexp.MustCompile(`\+?\d{8,}`)

// maskIdent replaces phone numbers and long numeric ids with a short hash.
// The same input always produces the same hash. The raw digits are not kept.
func maskIdent(s string) string {
	if s == "" {
		return s
	}
	return identDigits.ReplaceAllStringFunc(s, func(m string) string {
		sum := sha256.Sum256([]byte(m))
		return "id#" + hex.EncodeToString(sum[:4])
	})
}

func maskReport(rep *Report) {
	if rep == nil {
		return
	}
	for i := range rep.Tenants {
		rep.Tenants[i].TenantID = maskIdent(rep.Tenants[i].TenantID)
	}
	for i := range rep.NeedsCredential {
		n := &rep.NeedsCredential[i]
		n.TenantID = maskIdent(n.TenantID)
		n.SourceID = maskIdent(n.SourceID)
		n.Name = maskIdent(n.Name)
		n.ChannelType = maskIdent(n.ChannelType)
	}
	for i := range rep.NotImported {
		d := &rep.NotImported[i]
		d.TenantID = maskIdent(d.TenantID)
		d.SourceID = maskIdent(d.SourceID)
		d.Column = maskIdent(d.Column)
		d.Detail = maskIdent(d.Detail)
		d.Reason = maskIdent(d.Reason)
	}
}

func cronSame(j *store.CronJob, spec, message, sessionID string) bool {
	if j == nil || j.Enabled {
		return false
	}
	return j.Spec == spec && j.Message == message && j.SessionID == sessionID
}

func channelSame(got *store.ChannelConfig, want store.ChannelConfig) bool {
	if got == nil || got.Enabled || got.Name != want.Name || got.AgentID != want.AgentID {
		return false
	}
	if got.DMPolicy != want.DMPolicy || got.GroupPolicy != want.GroupPolicy || got.RequireMention != want.RequireMention {
		return false
	}
	return sameList(got.AllowFrom, want.AllowFrom)
}

func sameList(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
