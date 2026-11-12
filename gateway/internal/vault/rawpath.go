// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import "strings"

// UnderRaw reports a logical path inside a raw directory. raw.md is not under raw/.
// The directory name is the segment "raw", not a case-folded alias.
func UnderRaw(p string) bool {
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" || p == "raw" {
		return p == "raw"
	}
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return false
	}
	for _, seg := range parts[:len(parts)-1] {
		if seg == "raw" {
			return true
		}
	}
	return false
}
