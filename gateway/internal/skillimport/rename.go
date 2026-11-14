// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/mqglobal/goso/gateway/internal/skill"
)

const maxNameLen = 64

// nonName is every run that is not a lowercase letter or digit.
var nonName = regexp.MustCompile(`[^a-z0-9]+`)

// slugify maps a GoClaw skill name or slug onto a Goso folder name:
// lowercase, hyphenated, and matching skill.NamePattern.
// ok is false when the result is empty.
func slugify(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	s = strings.NewReplacer("đ", "d", "Đ", "d").Replace(s)
	folded, _, err := transform.String(markStripper, s)
	if err != nil || folded == "" {
		folded = s
	}
	s = strings.ToLower(folded)
	s = nonName.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > maxNameLen {
		s = strings.Trim(s[:maxNameLen], "-")
	}
	if !skill.NamePattern.MatchString(s) {
		return "", false
	}
	return s, true
}

// withSuffix returns base for n <= 1, otherwise base + "-" + n, cut so the
// result still fits the 64-character folder name.
func withSuffix(base string, n int) (string, bool) {
	if n <= 1 {
		if skill.NamePattern.MatchString(base) {
			return base, true
		}
		return "", false
	}
	suf := "-" + itoa(n)
	room := maxNameLen - len(suf)
	if room < 1 {
		return "", false
	}
	b := base
	if len(b) > room {
		b = strings.TrimRight(b[:room], "-")
	}
	if b == "" || !skill.NamePattern.MatchString(b+suf) {
		return "", false
	}
	return b + suf, true
}

var markStripper = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
