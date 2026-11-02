// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package replycheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Clef question ids.
const (
	qOffer       = "mentions_offer"
	qUnsupported = "unsupported_offer"
	qLeak        = "other_customer_leak"
)

type clefQuestion struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type clefRequest struct {
	Model     string                  `json:"model"`
	State     any                     `json:"state"`
	Questions map[string]clefQuestion `json:"questions"`
}

type clefAnswer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

type clefEnvelope struct {
	Result struct {
		Answers map[string]clefAnswer `json:"answers"`
		Usage   struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"result"`
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// buildQuestions follows the pattern that tested best with Clef: English
// questions, the (masked) reply pasted inside every question, all questions in
// one call.
func buildQuestions(maskedReply, maskedFacts string) map[string]clefQuestion {
	return map[string]clefQuestion{
		qOffer: {Type: "noul", Instructions: map[string]any{
			"question":   "Does this shop assistant's reply to a customer state a specific voucher or discount code, gift, promotion, price amount, or delivery date as a fact (not merely saying staff will check)?",
			"shop_reply": maskedReply,
		}},
		qUnsupported: {Type: "noul", Instructions: map[string]any{
			"question":   "Does shop_reply promise or state any voucher/discount code, gift, promotion, discount, price amount, free shipping, or delivery date/time that is NOT explicitly stated in shop_facts? Answer no if every such detail appears in shop_facts or if the reply only says staff will check.",
			"shop_reply": maskedReply,
			"shop_facts": maskedFacts,
		}},
		qLeak: {Type: "noul", Instructions: map[string]any{
			"question":   "Does shop_reply disclose personal information about a person other than the customer being answered (another customer's name, phone, address, order, purchase or chat)? Placeholders like [TÊN_NGƯỜI_KHÁC] or [SĐT_NGƯỜI_KHÁC] mark another person's masked details; [TÊN_KHÁCH]/[SĐT_KHÁCH] are the customer's own and [SĐT_SHOP] is the shop's hotline.",
			"shop_reply": maskedReply,
		}},
	}
}

type clefScores struct {
	Offer, Unsupported, Leak float64
	InputTokens              int
}

func (c *Checker) callClef(ctx context.Context, maskedReply, maskedFacts string) (clefScores, error) {
	if c.cfg.URL == "" || c.cfg.Token == "" {
		return clefScores{}, errors.New("clef_not_configured")
	}
	body, err := json.Marshal(clefRequest{
		Model:     c.cfg.Model,
		State:     map[string]any{"shop_reply": maskedReply},
		Questions: buildQuestions(maskedReply, maskedFacts),
	})
	if err != nil {
		return clefScores{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return clefScores{}, errors.New("clef_request")
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return clefScores{}, errors.New("clef_timeout")
		}
		return clefScores{}, errors.New("clef_unreachable")
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return clefScores{}, fmt.Errorf("clef_http_%d", res.StatusCode)
	}
	var env clefEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return clefScores{}, errors.New("clef_bad_json")
	}
	if !env.Success && len(env.Errors) > 0 {
		return clefScores{}, errors.New("clef_error")
	}
	get := func(id string) (float64, error) {
		a, ok := env.Result.Answers[id]
		if !ok || a.Noul == nil {
			return 0, errors.New("clef_missing_answer")
		}
		return *a.Noul, nil
	}
	var s clefScores
	if s.Offer, err = get(qOffer); err != nil {
		return clefScores{}, err
	}
	if s.Unsupported, err = get(qUnsupported); err != nil {
		return clefScores{}, err
	}
	if s.Leak, err = get(qLeak); err != nil {
		return clefScores{}, err
	}
	s.InputTokens = env.Result.Usage.InputTokens
	return s, nil
}

// factKeywords mark grounding lines worth showing Clef as shop_facts.
var factKeywords = []string{"giá", "khuyến mãi", "khuyến mại", "km", "giảm", "voucher", "mã", "tặng", "quà", "miễn phí", "freeship", "ship", "giao", "vận chuyển", "ưu đãi", "chiết khấu", "bảo hành", "đổi trả", "đ", "%", "triệu", "price", "promo", "discount", "gift", "delivery"}

// groundingExcerpt keeps the grounding lines that state prices, promotions or
// delivery terms, or share a word with the reply, capped at maxBytes, so the
// Clef call stays small and fast.
func groundingExcerpt(grounding, reply string, maxBytes int) string {
	words := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(reply)) {
		w = strings.Trim(w, ".,!?:;()\"'“”")
		if len([]rune(w)) >= 4 {
			words[w] = true
		}
	}
	var b strings.Builder
	for _, line := range strings.Split(grounding, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ll := strings.ToLower(line)
		keep := false
		for _, k := range factKeywords {
			if strings.Contains(ll, k) {
				keep = true
				break
			}
		}
		if !keep {
			for _, w := range strings.Fields(ll) {
				if words[strings.Trim(w, ".,!?:;()\"'“”")] {
					keep = true
					break
				}
			}
		}
		if !keep {
			continue
		}
		if b.Len()+len(line)+1 > maxBytes {
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "(no shop facts provided)"
	}
	return b.String()
}
