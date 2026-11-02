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
	// Offer marks a fact that states an offer: every money amount and
	// percentage, a voucher/discount/gift code, a date stated as a promotion
	// deadline. Offers are verified only by SHOP FACTS. A non-offer code (an
	// order or reference number) or date (an appointment) may also be
	// verified by the customer's own words.
	Offer bool
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
		add(fact{Kind: "money", Offer: true, Key: strconv.FormatInt(int64(a)*1_000_000+int64(b)*100_000, 10), Raw: m[0]})
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
		add(fact{Kind: "money", Offer: true, Key: strconv.FormatInt(int64(v*mult+0.5), 10), Raw: m[0]})
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
		add(fact{Kind: "money", Offer: true, Key: strconv.FormatInt(n, 10), Raw: m[0]})
	}
	for _, m := range rePercent.FindAllStringSubmatch(text, -1) {
		v, ok := parseNum(m[1])
		if !ok {
			continue
		}
		add(fact{Kind: "percent", Offer: true, Key: strconv.FormatFloat(v, 'f', -1, 64), Raw: m[0]})
	}

	for _, ix := range reCodeAfter.FindAllStringSubmatchIndex(text, -1) {
		raw := text[ix[2]:ix[3]]
		key := strings.ToUpper(raw)
		add(fact{Kind: "code", Key: key, Raw: raw, Offer: codeIsOffer(text, ix[2], ix[3], key)})
	}
	for _, ix := range reCodeToken.FindAllStringIndex(text, -1) {
		tok := text[ix[0]:ix[1]]
		if !hasLetter(tok) || !hasDigit(tok) || moneySpans[tok] || isMoneyToken(tok) {
			continue
		}
		add(fact{Kind: "code", Key: tok, Raw: tok, Offer: codeIsOffer(text, ix[0], ix[1], tok)})
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
		add(fact{Kind: "date", Key: strconv.Itoa(d) + "/" + strconv.Itoa(mo), Raw: m[0], Offer: dateIsOffer(text, ix[0])})
	}
	for _, ix := range reDateVN.FindAllStringSubmatchIndex(text, -1) {
		d, _ := strconv.Atoi(text[ix[2]:ix[3]])
		mo, _ := strconv.Atoi(text[ix[4]:ix[5]])
		add(fact{Kind: "date", Key: strconv.Itoa(d) + "/" + strconv.Itoa(mo), Raw: text[ix[0]:ix[1]], Offer: dateIsOffer(text, ix[0])})
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

// unverified returns the reply facts that neither source backs.
//
// shop is SHOP FACTS: the agent's instructions, knowledge and vault. Any fact
// found there by value (a code literally, money/percent/date by value) is
// verified.
//
// customer is CUSTOMER WORDS: the customer's own messages in this
// conversation and their memory note. It verifies only non-offer facts, so a
// customer's order/reference number or appointment date may be echoed back,
// but a voucher, discount, gift, price or percentage the customer (or a
// memory note) mentions is never proof that the shop offers it. A code the
// customer used in an offer context ("voucher X giảm 200k") counts as an
// offer even if the reply mentions it bare.
func unverified(reply []fact, shop, customer string) []fact {
	if len(reply) == 0 {
		return nil
	}
	shopHave := map[string]bool{}
	for _, f := range extractFacts(shop) {
		shopHave[f.Kind+":"+f.Key] = true
	}
	shopUpper := strings.ToUpper(shop)
	custHave := map[string]bool{}  // non-offer facts the customer stated
	custOffer := map[string]bool{} // codes/dates the customer stated as an offer
	for _, f := range extractFacts(customer) {
		if f.Offer {
			custOffer[f.Kind+":"+f.Key] = true
		} else {
			custHave[f.Kind+":"+f.Key] = true
		}
	}
	custUpper := strings.ToUpper(customer)
	var out []fact
	for _, f := range reply {
		k := f.Kind + ":" + f.Key
		if shopHave[k] || (f.Kind == "code" && strings.Contains(shopUpper, f.Key)) {
			continue
		}
		if f.Offer || custOffer[k] {
			f.Offer = true
			out = append(out, f)
			continue
		}
		if custHave[k] || (f.Kind == "code" && strings.Contains(custUpper, f.Key)) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// Offer cues around a code or date. Codes whose own letters say "voucher"
// (VOUCHER-C-5508, GIAM50, SALE10, KM-…) are offers wherever they appear.
var (
	offerCodeParts   = []string{"VOUCHER", "COUPON", "PROMO", "DISCOUNT", "GIAM", "SALE", "GIFT", "FREESHIP", "UUDAI", "DEAL", "KHUYENMAI"}
	offerCodeSegs    = []string{"VC", "KM", "CK", "QUA", "TANG", "OFF", "FREE", "VIP", "GG"}
	offerCuesBefore  = []string{"voucher", "coupon", "mã giảm", "mã khuyến", "mã ưu đãi", "mã quà", "mã tặng", "mã km", "code giảm", "áp mã", "nhập mã", "dùng mã", "khuyến mãi", "khuyến mại", "ưu đãi", "giảm giá", "chiết khấu", "promo", "discount", "gift"}
	offerCuesAfter   = []string{"giảm", "tặng", "ưu đãi", "khuyến mãi", "khuyến mại", "chiết khấu", "%", "off", "freeship", "free ship", "miễn phí", "voucher", "coupon"}
	refCuesBefore    = []string{"đơn", "đơn hàng", "mã đơn", "vận đơn", "mã vận đơn", "order", "tham chiếu", "mã tham chiếu", "ref", "booking", "hóa đơn", "hoá đơn", "hợp đồng", "mã khách", "mã hồ sơ", "hồ sơ", "phiếu", "tracking", "mã kh", "lịch hẹn", "mã hẹn", "bảo hành"}
	dateOfferCues    = []string{"hạn", "đến hết", "hết ngày", "áp dụng đến", "áp dụng tới", "khuyến mãi", "khuyến mại", "ưu đãi", "giảm", "voucher", "sale", "chương trình", "trước ngày"}
	clauseBreakRunes = ".!?;\n"
)

// codeIsOffer reports whether the code at text[start:end] is stated as a
// voucher/discount/gift rather than an order or reference number.
func codeIsOffer(text string, start, end int, key string) bool {
	for _, p := range offerCodeParts {
		if strings.Contains(key, p) {
			return true
		}
	}
	for _, seg := range strings.FieldsFunc(key, func(r rune) bool { return r == '-' || r == '_' || (r >= '0' && r <= '9') }) {
		for _, p := range offerCodeSegs {
			if seg == p || (len(p) >= 3 && strings.HasPrefix(seg, p)) {
				return true
			}
		}
	}
	before := clauseTail(strings.ToLower(text[:start]), 40)
	// "đơn TEST-C-5508", "mã đơn …": the word right before names a reference.
	near := strings.TrimRight(before, " :#\"'“”(")
	for _, w := range refCuesBefore {
		if strings.HasSuffix(near, w) {
			return false
		}
	}
	if containsAny(before, offerCuesBefore) {
		return true
	}
	return containsAny(clauseHead(strings.ToLower(text[end:]), 25), offerCuesAfter)
}

// dateIsOffer reports whether a date is stated as a promotion deadline.
func dateIsOffer(text string, start int) bool {
	return containsAny(clauseTail(strings.ToLower(text[:start]), 45), dateOfferCues)
}

// clauseTail is the last n bytes of s, cut at the last sentence break.
func clauseTail(s string, n int) string {
	if len(s) > n {
		s = s[len(s)-n:]
	}
	if i := strings.LastIndexAny(s, clauseBreakRunes); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// clauseHead is the first n bytes of s, cut at the first sentence break.
func clauseHead(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	if i := strings.IndexAny(s, clauseBreakRunes); i >= 0 {
		s = s[:i]
	}
	return s
}

func containsAny(s string, subs []string) bool {
	for _, w := range subs {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func startsWithAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
