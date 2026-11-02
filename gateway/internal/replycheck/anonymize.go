// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package replycheck

import (
	"regexp"
	"strings"
	"unicode"
)

// Placeholders the anonymizer writes. Clef only ever sees these, never a
// customer's real phone number, e-mail or name.
const (
	phShopPhone  = "[SĐT_SHOP]"
	phOwnPhone   = "[SĐT_KHÁCH]"
	phOtherPhone = "[SĐT_NGƯỜI_KHÁC]"
	phEmail      = "[EMAIL]"
	phOtherEmail = "[EMAIL_NGƯỜI_KHÁC]"
	phOwnName    = "[TÊN_KHÁCH]"
	phShopName   = "[TÊN_NHÂN_VIÊN]"
	phOtherName  = "[TÊN_NGƯỜI_KHÁC]"
)

var (
	// Vietnamese mobiles/landlines and hotlines: 0xxx xxx xxx, +84 xxx...,
	// 1900 xxxx / 1800 xxxx. Separators: space, dot, dash.
	rePhone = regexp.MustCompile(`(?:\+?84|0)(?:[ .\-]?\d){8,10}|1[89]00(?:[ .\-]?\d){4,6}`)
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// An honorific followed by 1–4 capitalised words is a person's name in a
	// Vietnamese chat reply ("chị Lan", "anh Nguyễn Văn Minh").
	reHonorName = regexp.MustCompile(`(?i:\b(anh|chị|chi|em|cô|chú|bác|ông|bà|bạn|cháu|khách|khách hàng|a\.|c\.)) ((?:\p{Lu}[\p{L}]*)(?: \p{Lu}[\p{L}]*){0,3})`)
)

// knownSides is what the anonymizer compares a phone/e-mail/name against to
// decide whose it is.
type knownSides struct {
	shop     string // agent instructions + knowledge, lower-cased
	customer string // this customer's own messages, memory and display name, lower-cased
}

func newKnownSides(shop, customer []string) knownSides {
	return knownSides{
		shop:     strings.ToLower(strings.Join(shop, "\n")),
		customer: strings.ToLower(strings.Join(customer, "\n")),
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "84") && len(d) >= 11 {
		d = "0" + d[2:]
	}
	return d
}

// anonResult is a masked text plus what the masking found.
type anonResult struct {
	Text         string
	OtherPhones  int // phone numbers that belong to neither the shop nor this customer
	OtherEmails  int
	OtherNames   int
	MaskedTokens int
}

// anonymize masks phone numbers, e-mails and personal names in text. A value
// found in the shop's own instructions/knowledge becomes a SHOP placeholder,
// one found in this customer's own messages/memory a KHÁCH placeholder, any
// other one a NGƯỜI_KHÁC placeholder (a possible leak). Masking happens in
// code before anything leaves the bridge.
func anonymize(text string, k knownSides) anonResult {
	var r anonResult
	digitIndexShop := digitsOnlyLines(k.shop)
	digitIndexCust := digitsOnlyLines(k.customer)
	text = reEmail.ReplaceAllStringFunc(text, func(m string) string {
		r.MaskedTokens++
		lm := strings.ToLower(m)
		switch {
		case strings.Contains(k.shop, lm), strings.Contains(k.customer, lm):
			return phEmail
		default:
			r.OtherEmails++
			return phOtherEmail
		}
	})
	text = replacePhones(text, func(m string) string {
		d := digitsOnly(m)
		r.MaskedTokens++
		switch {
		case strings.Contains(digitIndexShop, d):
			return phShopPhone
		case strings.Contains(digitIndexCust, d):
			return phOwnPhone
		default:
			r.OtherPhones++
			return phOtherPhone
		}
	})
	text = reHonorName.ReplaceAllStringFunc(text, func(m string) string {
		sub := reHonorName.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		name := strings.TrimSpace(sub[2])
		if isPlaceholderWord(name) {
			return m
		}
		r.MaskedTokens++
		ln := strings.ToLower(name)
		switch {
		case nameIn(k.customer, ln):
			return sub[1] + " " + phOwnName
		case nameIn(k.shop, ln):
			return sub[1] + " " + phShopName
		default:
			r.OtherNames++
			return sub[1] + " " + phOtherName
		}
	})
	r.Text = text
	return r
}

// digitsOnlyLines keeps line structure but strips non-digits inside numbers so
// "0909 123 456" and "0909.123.456" both index as 0909123456.
func digitsOnlyLines(s string) string {
	var b strings.Builder
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return unicode.IsLetter(r) || r == '\n' }) {
		b.WriteString(digitsOnly(f))
		b.WriteByte('|')
	}
	return b.String()
}

// nameIn matches the full name, or its last word (Vietnamese given name), as a
// whole word in hay.
func nameIn(hay, name string) bool {
	if hay == "" || name == "" {
		return false
	}
	if wordIn(hay, name) {
		return true
	}
	parts := strings.Fields(name)
	return len(parts) > 1 && wordIn(hay, parts[len(parts)-1])
}

func wordIn(hay, w string) bool {
	for i := 0; ; {
		j := strings.Index(hay[i:], w)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isWordRune(lastRune(hay[:j]))
		end := j + len(w)
		after := end >= len(hay) || !isWordRune(firstRune(hay[end:]))
		if before && after {
			return true
		}
		i = j + 1
		if i >= len(hay) {
			return false
		}
	}
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func lastRune(s string) rune {
	rs := []rune(s)
	if len(rs) == 0 {
		return ' '
	}
	return rs[len(rs)-1]
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return ' '
}

// Words that start a sentence after an honorific but are not names.
var notNames = map[string]bool{"shop": true, "Shop": true, "Dạ": true, "Vâng": true, "Ạ": true, "Mình": true}

func isPlaceholderWord(name string) bool {
	first := strings.Fields(name)[0]
	return strings.HasPrefix(name, "[") || notNames[first]
}

// replacePhones rewrites phone-number spans that stand alone (not the tail of
// a longer number such as a price "12.500.000") and have at least 8 digits.
func replacePhones(text string, fn func(string) string) string {
	var b strings.Builder
	last := 0
	for _, ix := range rePhone.FindAllStringIndex(text, -1) {
		if ix[0] > 0 {
			if p := text[ix[0]-1]; (p >= '0' && p <= '9') || p == '.' || p == ',' {
				continue
			}
		}
		if ix[1] < len(text) {
			if n := text[ix[1]]; n >= '0' && n <= '9' {
				continue
			}
		}
		m := text[ix[0]:ix[1]]
		if len(digitsOnly(m)) < 8 {
			continue
		}
		b.WriteString(text[last:ix[0]])
		b.WriteString(fn(m))
		last = ix[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

// stripPhones blanks phone numbers so their digits are not read as prices.
func stripPhones(text string) string {
	return replacePhones(text, func(string) string { return " " })
}
