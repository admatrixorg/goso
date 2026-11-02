// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"context"
	"encoding/json"
	"fmt"
)

// measureChoiceOrder asks the same relevance decision as a Choice, once with
// "relevant" first and once with it last. Noul has no option order. The
// jev-1.13 jaggedness page documents a lean toward the first Choice option,
// and there is no jev-1.14 jaggedness page, so the eval measures the lean
// instead of assuming it. The production filter stays on noul.
func measureChoiceOrder(ctx context.Context, c *Client, cases []EvalCase, rep *EvalReport) error {
	forward, err := c.askChoice(ctx, cases, "relevant", "not_relevant")
	if err != nil {
		return err
	}
	reverse, err := c.askChoice(ctx, cases, "not_relevant", "relevant")
	if err != nil {
		return err
	}
	if len(forward) != len(cases) || len(reverse) != len(cases) {
		return fmt.Errorf("choice_len")
	}
	rep.ChoiceCases = len(cases)
	for i := range cases {
		if forward[i].choice != reverse[i].choice {
			rep.ChoiceFlips++
		}
		if forward[i].choice == forward[i].first {
			rep.FirstForward++
		}
		if reverse[i].choice == reverse[i].first {
			rep.FirstReversed++
		}
	}
	return nil
}

type choiceHit struct {
	choice string
	first  string
}

func (c *Client) askChoice(ctx context.Context, cases []EvalCase, first, second string) ([]choiceHit, error) {
	if c == nil {
		return nil, fmt.Errorf("no_client")
	}
	qs := make(map[string]apiQuestion, len(cases))
	for i, cs := range cases {
		qs[fmt.Sprintf("c%d", i)] = apiQuestion{
			Type:         "choice",
			Instructions: fillInstructions(c.cfg.Instructions, cs.Message, cs.Name, cs.Passage),
			Criteria:     choiceCriteria(first, second),
		}
	}
	body, err := json.Marshal(apiRequest{Model: c.cfg.Model, State: c.cfg.ContextJSON, Questions: qs})
	if err != nil {
		return nil, err
	}
	env, err := c.roundTrip(ctx, body)
	if err != nil {
		return nil, err
	}
	out := make([]choiceHit, len(cases))
	for i := range cases {
		raw, ok := env.Answers[fmt.Sprintf("c%d", i)]
		if !ok {
			return nil, fmt.Errorf("missing_choice")
		}
		var a struct {
			Choice string `json:"choice"`
		}
		if json.Unmarshal(raw, &a) != nil || (a.Choice != first && a.Choice != second) {
			return nil, fmt.Errorf("bad_choice")
		}
		out[i] = choiceHit{choice: a.Choice, first: first}
	}
	return out, nil
}

func choiceCriteria(first, second string) json.RawMessage {
	desc := map[string]string{
		"relevant":     "The passage would help answer the customer message.",
		"not_relevant": "The passage would not help answer the customer message.",
	}
	fk, _ := json.Marshal(first)
	fv, _ := json.Marshal(desc[first])
	sk, _ := json.Marshal(second)
	sv, _ := json.Marshal(desc[second])
	return json.RawMessage(`{` + string(fk) + `:` + string(fv) + `,` + string(sk) + `:` + string(sv) + `}`)
}
