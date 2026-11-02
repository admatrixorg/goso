// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/replycheck"
)

// StopHeldForStaff is the stop reason of a customer reply the pre-send check
// held back. ReplyText is empty, so omni-core sends nothing to the customer;
// the bridge_reply_held log line carries the reason for staff.
const StopHeldForStaff = "held_for_staff"

// preSendCheck runs the reply check and logs the outcome. Logs carry only the
// anonymized draft (phones, e-mails and names masked), clipped.
func (s *Service) preSendCheck(ctx context.Context, in ReplyInput, reply, shopText, memoryNote string) replycheck.Verdict {
	customer := []string{in.Prompt}
	for _, m := range in.History {
		if m.Role == "user" {
			customer = append(customer, m.Content)
		}
	}
	ci := replycheck.Input{Reply: reply, Customer: customer}
	if strings.TrimSpace(shopText) != "" {
		ci.Shop = []string{shopText}
	}
	if strings.TrimSpace(memoryNote) != "" {
		ci.Memory = []string{memoryNote}
	}
	v := s.ReplyCheck.Check(ctx, ci)
	mode := s.ReplyCheck.Config().Mode.String()
	event := "bridge_reply_checked"
	if v.Hold {
		event = "bridge_reply_held"
		if mode != replycheck.ModeEnforce.String() {
			event = "bridge_reply_would_hold"
		}
	}
	fields := map[string]any{
		"event":       event,
		"mode":        mode,
		"tenant":      in.TenantID,
		"agent":       in.AgentID,
		"channel":     meta(in.Metadata, "channel_type", "channelType"),
		"hold":        v.Hold,
		"reasons":     v.Reasons,
		"clef_called": v.ClefCalled,
		"latency_ms":  v.Latency.Milliseconds(),
	}
	if v.ClefCalled && v.ClefError == "" {
		fields["offer"] = round2(v.Offer)
		fields["unsupported"] = round2(v.Unsupported)
		fields["leak"] = round2(v.Leak)
		fields["clef_input_tokens"] = v.InputTokens
	}
	if v.ClefError != "" {
		fields["clef_error"] = v.ClefError
	}
	if v.Hold {
		fields["draft_masked"] = clipRunes(v.Masked, 600)
	}
	logJSON(fields)
	return v
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
