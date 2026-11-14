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
	Skipped          bool
	Model            string
	ContextVersion   string
	QuestionsVersion string
	Cases            int
	Calls            int
	CacheHits        int
	InputTokens      int
	OutputTokens     int
	Agree            int
	Uncertain        int
	Disagree         int
	FalseDrop        int
	// Top-K overlap of the noul ranking against the labeled relevant set,
	// grouped by customer message. K is the number of relevant labels.
	TopKGroups  int
	TopKOverlap float64
	// Confidence on wrong noul answers is |2p-1|, the docs' noul confidence.
	WrongN        int
	WrongConfMin  float64
	WrongConfMax  float64
	ChoiceCases   int
	ChoiceFlips   int
	FirstForward  int
	FirstReversed int
	BiasError     string
}

func (r EvalReport) String() string {
	if r.Skipped {
		return "jev-eval: skipped (GOSO_JEV_API_KEY unset)"
	}
	return fmt.Sprintf("jev-eval: model=%s context=%s questions=%s cases=%d calls=%d cache_hits=%d input_tokens=%d output_tokens=%d agree=%d uncertain=%d disagree=%d false_drop=%d topk_groups=%d topk_overlap=%.3f wrong_n=%d wrong_conf_min=%.3f wrong_conf_max=%.3f choice_cases=%d choice_flips=%d first_option_forward=%d first_option_reversed=%d bias_error=%s",
		r.Model, r.ContextVersion, r.QuestionsVersion, r.Cases, r.Calls, r.CacheHits, r.InputTokens, r.OutputTokens, r.Agree, r.Uncertain, r.Disagree, r.FalseDrop,
		r.TopKGroups, r.TopKOverlap, r.WrongN, r.WrongConfMin, r.WrongConfMax, r.ChoiceCases, r.ChoiceFlips, r.FirstForward, r.FirstReversed, r.BiasError)
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
	c := New(cfg, nil)
	rep, err := ScoreCases(ctx, c, cases)
	if err != nil {
		return rep, err
	}
	if berr := measureChoiceOrder(ctx, c, cases, &rep); berr != nil {
		// The noul scores still stand. A bias-check failure must not hide them.
		rep.BiasError = berr.Error()
	}
	return rep, nil
}

// ScoreCases sends every labeled pair in as few calls as the caps allow.
// A noul at or above yes_at agrees with relevant=true. A noul at or below
// no_at agrees with relevant=false. The band between them is uncertain, which
// is the same band the reply path refuses to drop.
func ScoreCases(ctx context.Context, c *Client, cases []EvalCase) (EvalReport, error) {
	if c == nil {
		return EvalReport{}, fmt.Errorf("no_client")
	}
	rep := EvalReport{Cases: len(cases), Model: c.cfg.Model, ContextVersion: c.cfg.ContextVersion, QuestionsVersion: c.cfg.QuestionsVersion}
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
	rep.TopKGroups, rep.TopKOverlap = topKOverlap(cases, results)
	rep.WrongN, rep.WrongConfMin, rep.WrongConfMax = wrongConfidence(cases, results, c.cfg.YesAt, c.cfg.NoAt)
	return rep, nil
}

func topKOverlap(cases []EvalCase, results []Result) (int, float64) {
	type row struct {
		relevant bool
		noul     float64
	}
	groups := map[string][]row{}
	order := []string{}
	for i, cs := range cases {
		if _, ok := groups[cs.Message]; !ok {
			order = append(order, cs.Message)
		}
		noul := 0.0
		if i < len(results) && results[i].Have {
			noul = results[i].Noul
		}
		groups[cs.Message] = append(groups[cs.Message], row{relevant: cs.Relevant, noul: noul})
	}
	var sum float64
	n := 0
	for _, msg := range order {
		rows := groups[msg]
		k := 0
		for _, r := range rows {
			if r.relevant {
				k++
			}
		}
		if k == 0 || len(rows) < 2 {
			continue
		}
		sorted := append([]row(nil), rows...)
		for i := 1; i < len(sorted); i++ {
			j := i
			for j > 0 && sorted[j].noul > sorted[j-1].noul {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
				j--
			}
		}
		if k > len(sorted) {
			k = len(sorted)
		}
		hit := 0
		for _, r := range sorted[:k] {
			if r.relevant {
				hit++
			}
		}
		sum += float64(hit) / float64(k)
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return n, sum / float64(n)
}

func wrongConfidence(cases []EvalCase, results []Result, yes, no float64) (int, float64, float64) {
	var vals []float64
	for i, cs := range cases {
		if i >= len(results) || !results[i].Have {
			continue
		}
		p := results[i].Noul
		wrong := (p >= yes && !cs.Relevant) || (p <= no && cs.Relevant)
		if !wrong {
			continue
		}
		d := p*2 - 1
		if d < 0 {
			d = -d
		}
		vals = append(vals, d)
	}
	if len(vals) == 0 {
		return 0, 0, 0
	}
	min, max := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return len(vals), min, max
}
