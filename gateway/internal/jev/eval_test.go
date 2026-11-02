// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEvalCases_MinhAnOnly(t *testing.T) {
	cases, err := EvalCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 20 || len(cases) > 50 {
		t.Fatalf("cases %d", len(cases))
	}
	seen := map[string]bool{}
	blob := string(evalCasesJSON)
	for _, banned := range []string{"MQ Capital", "mqcapital", "Chủ tịch", "ARR", "@"} {
		if strings.Contains(blob, banned) {
			t.Fatalf("banned %q", banned)
		}
	}
	for _, cs := range cases {
		if cs.ID == "" || cs.Message == "" || cs.Passage == "" || seen[cs.ID] {
			t.Fatalf("bad case %+v", cs)
		}
		seen[cs.ID] = true
		if !strings.Contains(cs.Passage, "Minh An") && !strings.Contains(cs.Source, "mock.ts") && !strings.Contains(cs.Source, "queue.md") && !strings.Contains(cs.Source, "chat-attach") && !strings.Contains(cs.Passage, "Đại lý") && !strings.Contains(cs.Passage, "Deal") && !strings.Contains(cs.Passage, "Nick") && !strings.Contains(cs.Passage, "Việc") && !strings.Contains(cs.Passage, "Broadcast") && !strings.Contains(cs.Passage, "nick") && !strings.Contains(cs.Passage, "Cố vấn") && !strings.Contains(cs.Passage, "Cam kết") && !strings.Contains(cs.Passage, "Cuộc họp") {
			t.Fatalf("passage not from sample: %+v", cs)
		}
	}
}

func TestRunEval_SkipsWithoutKey(t *testing.T) {
	t.Setenv("GOSO_JEV_API_KEY", "")
	rep, err := RunEval(context.Background())
	if err != nil || !rep.Skipped || rep.Cases < 20 {
		t.Fatalf("err %v %+v", err, rep)
	}
	if !strings.Contains(rep.String(), "skipped") {
		t.Fatal(rep.String())
	}
}

func TestScoreCases_FakeAgrees(t *testing.T) {
	cases, err := EvalCases()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req apiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if len(req.Questions) != len(cases) {
			t.Fatalf("not one batch: %d", len(req.Questions))
		}
		answers := map[string]any{}
		for id, q := range req.Questions {
			rel := false
			for _, cs := range cases {
				if q.Instructions["passage"] == cs.Passage && q.Instructions["customer_message"] == cs.Message {
					rel = cs.Relevant
				}
			}
			noul := 0.05
			if rel {
				noul = 0.95
			}
			answers[id] = map[string]any{"type": "noul", "noul": noul}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": defaultModel, "answers": answers,
			"usage": map[string]int{"input_tokens": 400, "output_tokens": 40},
		})
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.Enabled = true
	cfg.APIKey = "k"
	cfg.URL = srv.URL
	cfg.Timeout = time.Second
	cfg.MaxCalls = 2
	rep, err := ScoreCases(context.Background(), New(cfg, srv.Client()), cases)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Calls != 1 || rep.Agree != len(cases) || rep.Disagree != 0 || rep.FalseDrop != 0 || rep.InputTokens != 400 {
		t.Fatalf("%+v", rep)
	}
}
