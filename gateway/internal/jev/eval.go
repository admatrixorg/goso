// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EvalCase is one labeled (message, passage) pair from the Minh An sample.
type EvalCase struct {
	ID       string `json:"id"`
	Relevant bool   `json:"relevant"`
	Message  string `json:"message"`
	Name     string `json:"name"`
	Passage  string `json:"passage"`
	Source   string `json:"source"`
}

// EvalReport is the agreement summary. Skipped is set when no API key is set.
type EvalReport struct {
	Skipped      bool
	Model        string
	Cases        int
	Calls        int
	CacheHits    int
	InputTokens  int
	OutputTokens int
	Agree        int
	Uncertain    int
	Disagree     int
	FalseDrop    int
}

func (r EvalReport) String() string {
	if r.Skipped {
		return "jev-eval: skipped (GOSO_JEV_API_KEY unset)"
	}
	return fmt.Sprintf("jev-eval: model=%s cases=%d calls=%d cache_hits=%d input_tokens=%d output_tokens=%d agree=%d uncertain=%d disagree=%d false_drop=%d",
		r.Model, r.Cases, r.Calls, r.CacheHits, r.InputTokens, r.OutputTokens, r.Agree, r.Uncertain, r.Disagree, r.FalseDrop)
}

// EvalCases is the embedded labeled set.
func EvalCases() ([]EvalCase, error) {
	var file struct {
		Cases []EvalCase `json:"cases"`
	}
	if err := json.Unmarshal(evalCasesJSON, &file); err != nil {
		return nil, err
	}
	return file.Cases, nil
}

// RunEval scores the embedded labels. With no API key it returns a skipped
// report and a nil error so the script can exit 0.
func RunEval(ctx context.Context) (EvalReport, error) {
	cases, err := EvalCases()
	if err != nil {
		return EvalReport{}, err
	}
	if strings.TrimSpace(os.Getenv("GOSO_JEV_API_KEY")) == "" {
		return EvalReport{Skipped: true, Cases: len(cases)}, nil
	}
	cfg := LoadConfig()
	cfg.Enabled = true
	if cfg.QuestionsErr != "" {
		return EvalReport{Cases: len(cases)}, fmt.Errorf("%s", cfg.QuestionsErr)
	}
	return ScoreCases(ctx, New(cfg, nil), cases)
}

// ScoreCases sends every labeled pair in as few calls as the caps allow.
// A noul at or above yes_at agrees with relevant=true. A noul at or below
// no_at agrees with relevant=false. The band between them is uncertain, which
// is the same band the reply path refuses to drop.
func ScoreCases(ctx context.Context, c *Client, cases []EvalCase) (EvalReport, error) {
	if c == nil {
		return EvalReport{}, fmt.Errorf("no_client")
	}
	rep := EvalReport{Cases: len(cases), Model: c.cfg.Model}
	items := make([]Item, len(cases))
	for i, cs := range cases {
		items[i] = Item{Message: cs.Message, Passage: Passage{ID: cs.ID, Name: cs.Name, Content: cs.Passage}}
	}
	results, usage, err := c.Score(ctx, items)
	rep.Calls = usage.Calls
	rep.CacheHits = usage.CacheHits
	rep.InputTokens = usage.InputTokens
	rep.OutputTokens = usage.OutputTokens
	if usage.Model != "" {
		rep.Model = usage.Model
	}
	if err != nil {
		return rep, err
	}
	for i, cs := range cases {
		r := results[i]
		if !r.Have {
			return rep, fmt.Errorf("unscored %s", cs.ID)
		}
		switch {
		case r.Noul >= c.cfg.YesAt && cs.Relevant:
			rep.Agree++
		case r.Noul <= c.cfg.NoAt && !cs.Relevant:
			rep.Agree++
		case r.Noul > c.cfg.NoAt && r.Noul < c.cfg.YesAt:
			rep.Uncertain++
		default:
			rep.Disagree++
			if cs.Relevant && r.Noul <= c.cfg.NoAt {
				rep.FalseDrop++
			}
		}
	}
	return rep, nil
}
