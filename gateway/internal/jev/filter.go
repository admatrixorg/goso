// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// FilterPassages returns the passages to inject. When Jev is off, or the key
// is empty, in is returned unchanged. A scored noul at or below no_at is
// dropped. A noul between the thresholds, a missing answer, a timeout, an
// HTTP error, or a passage that does not fit the call cap is kept — that is
// today's behavior for that passage.
func FilterPassages(ctx context.Context, message string, in []Passage) []Passage {
	if EnabledFlag() && !KeySet() {
		noKeyOnce.Do(func() {
			log.Printf("goso jev: GOSO_JEV_ENABLED is set but GOSO_JEV_API_KEY is empty; knowledge injection unchanged")
		})
		return in
	}
	if !Active() || len(in) == 0 {
		return in
	}
	if strings.TrimSpace(message) == "" {
		log.Printf("goso jev: skip reason=empty_message passages=%d", len(in))
		return in
	}
	c := defaultClient()
	results, usage, err := c.Nouls(ctx, message, in)
	kept := make([]Passage, 0, len(in))
	dropped := 0
	fallback := 0
	for _, r := range results {
		keep, decision := keepPassage(r, c.cfg.YesAt, c.cfg.NoAt)
		noul := "-"
		if r.Have {
			noul = fmt.Sprintf("%.4f", r.Noul)
		} else {
			fallback++
		}
		log.Printf("goso jev: passage id=%s file=%s noul=%s decision=%s reason=%s cached=%t",
			r.Passage.ID, r.Passage.Name, noul, decision, r.Reason, r.Cached)
		if keep {
			kept = append(kept, r.Passage)
		} else {
			dropped++
		}
	}
	fallbackReason := ""
	if err != nil {
		fallbackReason = err.Error()
	}
	log.Printf("goso jev: summary model=%s response_model=%s calls=%d cache_hits=%d input_tokens=%d output_tokens=%d kept=%d dropped=%d unscored=%d fallback=%s",
		c.cfg.Model, usage.Model, usage.Calls, usage.CacheHits, usage.InputTokens, usage.OutputTokens, len(kept), dropped, fallback, fallbackReason)
	return kept
}

func keepPassage(r Result, yes, no float64) (bool, string) {
	if !r.Have {
		return true, "keep"
	}
	if r.Noul <= no {
		return false, "drop"
	}
	if r.Noul >= yes {
		return true, "keep"
	}
	return true, "uncertain"
}
