// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package replycheck

import (
	"regexp"
	"strconv"
	"strings"
)

// fact is one concrete claim a reply makes that the shop must be able to back:
// a promo/voucher code, a money amount, a percentage, or a calendar date.
type fact struct {
	Kind string // code | money | percent | date
	Key  string // normalised value used for comparison
	Raw  string
}

var (
	// Codes: an upper-case token with a letter and a digit (VOUCHER-C-5508,
	// MOCAN11, GIAM50K), or an upper-case token after a code word.
	reCodeToken = regexp.MustCompile(`\b[A-Z0-9][A-Z0-9_\-]{3,}\b`)
	reCodeAfter = regexp.MustCompile(`(?i:mã(?: giảm giá| khuyến mãi| voucher| ưu đãi| quà tặng)?|voucher|code|coupon)\s*[:"“']?\s*([A-Z][A-Z0-9_\-]{3,})\b`)

	// Money: 1.500.000đ, 1,500,000 VND, 12.5 triệu, 1tr5, 500k, 500 nghìn.
	reMoneyTrX  = regexp.MustCompile(`(?i)\b(\d+)\s*(?:tr|triệu|củ)\s*(\d)\b`)
	reMoneyUnit = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(k\b|nghìn|ngàn|tr\b|triệu|củ|tỷ)`)
	reMoneyBig  = regexp.MustCompile(`(?i)\b(\d{1,3}(?:[.,]\d{3})+|\d{4,})\s*(đ|₫|đồng|vnđ|vnd|dong)?`)
	rePercent   = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*%`)

	// Dates: 15/11, 15/11/2026, 15-11, ngày 15 tháng 11.
	reDateNum = regexp.MustCompile(`\b(\d{1,2})[/\-](\d{1,2})(?:[/\-](\d{2,4}))?\b`)
	reDateVN  = regexp.MustCompile(`(?i)ngày\s+(\d{1,2})\s+tháng\s+(\d{1,2})`)
)

func extractFacts(text string) []fact {
	text = reEmail.ReplaceAllString(stripPhones(text), " ")
	var out []fact
	seen := map[string]bool{}
	add := func(f fact) {
		k := f.Kind + ":" + f.Key
		if f.Key == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, f)
	}

	// Money first, so money-looking tokens (500K, 1TR5) are not codes too.
	moneySpans := map[string]bool{}
	for _, m := range reMoneyTrX.FindAllStringSubmatch(text, -1) {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		add(fact{Kind: "money", Key: strconv.FormatInt(int64(a)*1_000_000+int64(b)*100_000, 10), Raw: m[0]})
		moneySpans[strings.ToUpper(strings.ReplaceAll(m[0], " ", ""))] = true
	}
	for _, ix := range reMoneyUnit.FindAllStringSubmatchIndex(text, -1) {
		// "củ" in "của", "k" in "kệ": the unit must end the word.
		if ix[1] < len(text) && isWordRune(firstRune(text[ix[1]:])) {
			continue
		}
		m := []string{text[ix[0]:ix[1]], text[ix[2]:ix[3]], text[ix[4]:ix[5]]}
		v, ok := parseNum(m[1])
		if !ok {
			continue
		}
		mult := 1.0
		switch strings.ToLower(m[2]) {
		case "k", "nghìn", "ngàn":
			mult = 1e3
		case "tr", "triệu", "củ":
			mult = 1e6
		case "tỷ":
			mult = 1e9
		}
		add(fact{Kind: "money", Key: strconv.FormatInt(int64(v*mult+0.5), 10), Raw: m[0]})
		moneySpans[strings.ToUpper(strings.ReplaceAll(m[0], " ", ""))] = true
	}
	for _, ix := range reMoneyBig.FindAllStringSubmatchIndex(text, -1) {
		// Digits inside a code ("MA-1043", "VIP2000") are not a price.
		if ix[0] > 0 {
			if p := lastRune(text[:ix[0]]); isWordRune(p) || p == '-' || p == '_' {
				continue
			}
		}
		m := []string{text[ix[0]:ix[1]], text[ix[2]:ix[3]], ""}
		// The currency mark must end the word: "đ" in "đã" is not one.
		if ix[4] >= 0 && (ix[5] >= len(text) || !isWordRune(firstRune(text[ix[5]:]))) {
			m[2] = text[ix[4]:ix[5]]
		}
		if m[2] == "" {
			m[0] = m[1]
		}
		d := strings.NewReplacer(".", "", ",", "").Replace(m[1])
		if len(d) < 4 {
			continue
		}
		// Years and phone-like runs are not prices.
		if n, _ := strconv.Atoi(d); m[2] == "" && n >= 1900 && n <= 2100 {
			continue
		}
		if m[2] == "" && strings.HasPrefix(d, "0") {
			continue
		}
		// A bare number needs thousand separators to read as a price
		// ("1.500.000"); "1205" or "2026" alone is an apartment, a year, a
		// quantity.
		if m[2] == "" && (len(d) > 10 || !strings.ContainsAny(m[1], ".,")) {
			continue
		}
		n, err := strconv.ParseInt(d, 10, 64)
		if err != nil || n < 1000 {
			continue
		}
		add(fact{Kind: "money", Key: strconv.FormatInt(n, 10), Raw: m[0]})
	}
	for _, m := range rePercent.FindAllStringSubmatch(text, -1) {
		v, ok := parseNum(m[1])
		if !ok {
			continue
		}
		add(fact{Kind: "percent", Key: strconv.FormatFloat(v, 'f', -1, 64), Raw: m[0]})
	}

	for _, m := range reCodeAfter.FindAllStringSubmatch(text, -1) {
		add(fact{Kind: "code", Key: strings.ToUpper(m[1]), Raw: m[1]})
	}
	for _, tok := range reCodeToken.FindAllString(text, -1) {
		if !hasLetter(tok) || !hasDigit(tok) || moneySpans[tok] || isMoneyToken(tok) {
			continue
		}
		add(fact{Kind: "code", Key: tok, Raw: tok})
	}

	for _, ix := range reDateNum.FindAllStringSubmatchIndex(text, -1) {
		m := []string{text[ix[0]:ix[1]], text[ix[2]:ix[3]], text[ix[4]:ix[5]]}
		// "2-4 ngày", "3-5 tuần" are durations, not dates.
		if rest := strings.TrimSpace(strings.ToLower(text[ix[1]:])); strings.Contains(m[0], "-") && startsWithAny(rest, "ngày", "tuần", "tiếng", "giờ", "h", "món", "cái", "chiếc", "bộ", "%") {
			continue
		}
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if d < 1 || d > 31 || mo < 1 || mo > 12 {
			continue
		}
		add(fact{Kind: "date", Key: strconv.Itoa(d) + "/" + strconv.Itoa(mo), Raw: m[0]})
	}
	for _, m := range reDateVN.FindAllStringSubmatch(text, -1) {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		add(fact{Kind: "date", Key: strconv.Itoa(d) + "/" + strconv.Itoa(mo), Raw: m[0]})
	}
	return out
}

var reMoneyTok = regexp.MustCompile(`^\d+(?:[.,]\d+)?(?:K|TR|TR\d)$`)

func isMoneyToken(tok string) bool { return reMoneyTok.MatchString(tok) }

func parseNum(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	// "12,5" and "12.5" are decimals; "1.500" (three digits after) is thousands.
	if i := strings.LastIndexAny(s, ".,"); i >= 0 {
		if len(s)-i-1 == 3 {
			s = strings.NewReplacer(".", "", ",", "").Replace(s)
		} else {
			s = strings.Replace(s, ",", ".", 1)
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func hasLetter(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func hasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// unverified returns the reply facts the grounding text does not contain.
// Grounding is the agent's instructions, its knowledge/vault and this
// customer's memory; a fact is verified only when the same normalised value
// appears there (a code literally, money/percent/date by value).
func unverified(reply []fact, grounding string) []fact {
	if len(reply) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, f := range extractFacts(grounding) {
		have[f.Kind+":"+f.Key] = true
	}
	upper := strings.ToUpper(grounding)
	var out []fact
	for _, f := range reply {
		if have[f.Kind+":"+f.Key] {
			continue
		}
		if f.Kind == "code" && strings.Contains(upper, f.Key) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func startsWithAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
