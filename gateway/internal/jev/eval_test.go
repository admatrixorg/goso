// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"context"
	"encoding/json"
	"io"
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

func TestTopKAndWrongConfidence(t *testing.T) {
	cases := []EvalCase{
		{ID: "a", Message: "m", Relevant: true},
		{ID: "b", Message: "m", Relevant: false},
		{ID: "c", Message: "m", Relevant: true},
	}
	results := []Result{
		{Have: true, Noul: 0.9},
		{Have: true, Noul: 0.8},
		{Have: true, Noul: 0.2},
	}
	groups, overlap := topKOverlap(cases, results)
	if groups != 1 || overlap != 0.5 {
		t.Fatalf("groups %d overlap %v", groups, overlap)
	}
	n, lo, hi := wrongConfidence(cases, results, 0.7, 0.3)
	// 0.8 is a confident yes on a negative label, and 0.2 is a confident no
	// on a positive label. Both have |2p-1| = 0.6.
	if n != 2 || lo < 0.59 || lo > 0.61 || hi < 0.59 || hi > 0.61 {
		t.Fatalf("wrong n=%d min=%v max=%v", n, lo, hi)
	}
}

func TestChoiceOrder_ReportsFirstOption(t *testing.T) {
	cases := []EvalCase{
		{ID: "a", Message: "Đại lý Bình Dương còn nợ bao nhiêu?", Name: "a", Passage: "Đại lý Bình Dương. Minh An (demo)", Relevant: true},
		{ID: "b", Message: "Đại lý Bình Dương còn nợ bao nhiêu?", Name: "b", Passage: "Họp kế hoạch nồi chiên — Minh An (demo)", Relevant: false},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Questions map[string]struct {
				Type     string          `json:"type"`
				Criteria json.RawMessage `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatal(err)
		}
		answers := map[string]any{}
		for id, q := range req.Questions {
			if q.Type != "choice" {
				t.Fatalf("type %s", q.Type)
			}
			first := firstJSONKey(q.Criteria)
			answers[id] = map[string]any{"type": "choice", "choice": first, "confidence": 0.2}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": defaultModel, "answers": answers,
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 2},
		})
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.URL = srv.URL
	cfg.APIKey = "k"
	cfg.Timeout = time.Second
	var rep EvalReport
	if err := measureChoiceOrder(context.Background(), New(cfg, srv.Client()), cases, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.ChoiceCases != 2 || rep.ChoiceFlips != 2 || rep.FirstForward != 2 || rep.FirstReversed != 2 {
		t.Fatalf("%+v", rep)
	}
}

func firstJSONKey(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) == 0 || s[0] != '{' {
		return ""
	}
	s = strings.TrimSpace(s[1:])
	if len(s) == 0 || s[0] != '"' {
		return ""
	}
	end := strings.Index(s[1:], `"`)
	if end < 0 {
		return ""
	}
	return s[1 : 1+end]
}

func TestRetry429ThenOK(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		var req apiRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		answers := map[string]any{}
		for id := range req.Questions {
			answers[id] = map[string]any{"type": "noul", "noul": 0.2}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.14.0", "answers": answers,
			"usage": map[string]int{"input_tokens": 4, "output_tokens": 1},
		})
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.URL = srv.URL
	cfg.APIKey = "k"
	cfg.Timeout = time.Second
	res, _, err := New(cfg, srv.Client()).Nouls(context.Background(), "m", []Passage{{ID: "a", Content: "x"}})
	if err != nil || hits != 2 || !res[0].Have || res[0].Noul != 0.2 {
		t.Fatalf("err %v hits %d %+v", err, hits, res)
	}
}

func TestRetryAfterTooLongFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(529)
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.URL = srv.URL
	cfg.APIKey = "k"
	cfg.Timeout = 80 * time.Millisecond
	start := time.Now()
	_, _, err := New(cfg, srv.Client()).Nouls(context.Background(), "m", []Passage{{ID: "a", Content: "x"}})
	if err == nil || err.Error() != "timeout" || time.Since(start) > time.Second {
		t.Fatalf("err %v waited %s", err, time.Since(start))
	}
}

func TestCacheKeyChangesWithContextVersion(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		var req apiRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		answers := map[string]any{}
		for id := range req.Questions {
			answers[id] = map[string]any{"type": "noul", "noul": 0.25}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": defaultModel, "answers": answers,
			"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.URL = srv.URL
	cfg.APIKey = "k"
	cfg.Timeout = time.Second
	c := New(cfg, srv.Client())
	ps := []Passage{{ID: "a", Name: "n", Content: "same text"}}
	if _, _, err := c.Nouls(context.Background(), "msg", ps); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Nouls(context.Background(), "msg", ps); err != nil || hits != 1 {
		t.Fatalf("cache hit missing hits=%d", hits)
	}
	c.cfg.ContextVersion = "2099-01-01.9"
	if _, _, err := c.Nouls(context.Background(), "msg", ps); err != nil || hits != 2 {
		t.Fatalf("version change hits=%d err %v", hits, err)
	}
	c.cfg.Model = "jev-1.13.0"
	if _, _, err := c.Nouls(context.Background(), "msg", ps); err != nil || hits != 3 {
		t.Fatalf("model change hits=%d err %v", hits, err)
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
