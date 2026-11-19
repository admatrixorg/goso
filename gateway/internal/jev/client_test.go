// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFilter_OffWithoutKeyIsUnchanged(t *testing.T) {
	resetDefault()
	t.Cleanup(resetDefault)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		t.Error("jev was called")
	}))
	defer srv.Close()
	t.Setenv("GOSO_JEV_ENABLED", "1")
	t.Setenv("GOSO_JEV_API_KEY", "")
	t.Setenv("GOSO_JEV_URL", srv.URL)
	in := []Passage{{ID: "a", Name: "a", Content: "Đại lý Bình Dương"}}
	got := FilterPassages(context.Background(), "Đại lý Bình Dương còn nợ bao nhiêu?", in)
	if len(got) != 1 || got[0].ID != "a" || hits != 0 {
		t.Fatalf("got %+v hits %d", got, hits)
	}
	t.Setenv("GOSO_JEV_ENABLED", "")
	t.Setenv("GOSO_JEV_API_KEY", "present")
	got = FilterPassages(context.Background(), "x", in)
	if len(got) != 1 || hits != 0 {
		t.Fatalf("disabled %+v", got)
	}
}

func TestFilter_DropsOnlyConfidentNo(t *testing.T) {
	resetDefault()
	t.Cleanup(resetDefault)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	const key = "jev-test-key-do-not-log"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), key) {
			t.Error("key in body")
		}
		var req apiRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatal(err)
		}
		if req.Model != defaultModel {
			t.Fatalf("model %s", req.Model)
		}
		state := string(req.State)
		if strings.Contains(state, "UNIQUE_PASSAGE_DROP") || strings.Contains(state, "UNIQUE_PASSAGE_KEEP") {
			t.Fatalf("passage leaked into state: %s", state)
		}
		if !strings.Contains(state, "Minh An (demo)") {
			t.Fatalf("state %s", state)
		}
		if len(req.Questions) != 3 {
			t.Fatalf("questions %d", len(req.Questions))
		}
		answers := map[string]any{}
		for id, q := range req.Questions {
			if q.Type != "noul" {
				t.Fatalf("type %s", q.Type)
			}
			blob := q.Instructions["passage"]
			qtext := q.Instructions["question"]
			if !strings.Contains(qtext, "Does the passage") {
				t.Fatalf("question not from file: %s", qtext)
			}
			noul := 0.5
			switch {
			case strings.Contains(blob, "UNIQUE_PASSAGE_DROP"):
				noul = 0.05
			case strings.Contains(blob, "UNIQUE_PASSAGE_KEEP"):
				noul = 0.91
			case strings.Contains(blob, "UNIQUE_PASSAGE_MID"):
				noul = 0.5
			default:
				t.Fatalf("passage not in question %s: %+v", id, q.Instructions)
			}
			answers[id] = map[string]any{"type": "noul", "noul": noul}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   defaultModel,
			"answers": answers,
			"usage":   map[string]int{"input_tokens": 120, "output_tokens": 9},
		})
	}))
	defer srv.Close()
	t.Setenv("GOSO_JEV_ENABLED", "1")
	t.Setenv("GOSO_JEV_API_KEY", key)
	t.Setenv("GOSO_JEV_URL", srv.URL)
	t.Setenv("GOSO_JEV_MODEL", "")
	in := []Passage{
		{ID: "drop", Name: "noi-chien", Content: "UNIQUE_PASSAGE_DROP"},
		{ID: "keep", Name: "binh-duong", Content: "UNIQUE_PASSAGE_KEEP"},
		{ID: "mid", Name: "mid", Content: "UNIQUE_PASSAGE_MID"},
	}
	got := FilterPassages(context.Background(), "Đại lý Bình Dương còn nợ bao nhiêu?", in)
	if len(got) != 2 || got[0].ID != "keep" || got[1].ID != "mid" {
		t.Fatalf("got %+v", got)
	}
	logs := buf.String()
	if !strings.Contains(logs, "noul=0.0500") || !strings.Contains(logs, "noul=0.9100") || !strings.Contains(logs, "input_tokens=120") {
		t.Fatalf("logs %s", logs)
	}
	if strings.Contains(logs, key) {
		t.Fatal("key logged")
	}
}

func TestScore_ErrorTimeoutCacheAndCap(t *testing.T) {
	cfg := LoadConfig()
	cfg.Enabled = true
	cfg.APIKey = "k"
	cfg.Model = defaultModel
	cfg.Timeout = 30 * time.Millisecond
	cfg.MaxCalls = 1
	cfg.CacheTTL = time.Hour

	t.Run("http", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		cfg := cfg
		cfg.URL = srv.URL
		c := New(cfg, srv.Client())
		res, _, err := c.Nouls(context.Background(), "m", []Passage{{ID: "a", Content: "x"}})
		if err == nil || res[0].Have || res[0].Reason != "http_500" {
			t.Fatalf("err %v %+v", err, res)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}))
		defer srv.Close()
		cfg := cfg
		cfg.URL = srv.URL
		cfg.Timeout = 80 * time.Millisecond
		c := New(cfg, &http.Client{Timeout: time.Second, Transport: srv.Client().Transport})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		res, _, err := c.Nouls(ctx, "m", []Passage{{ID: "a", Content: "slow"}})
		if err == nil || err.Error() != "timeout" || res[0].Have {
			t.Fatalf("err %v %+v", err, res)
		}
	})

	t.Run("cache", func(t *testing.T) {
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
				"usage": map[string]int{"input_tokens": 10, "output_tokens": 1},
			})
		}))
		defer srv.Close()
		cfg := cfg
		cfg.URL = srv.URL
		cfg.Timeout = time.Second
		now := time.Now()
		c := New(cfg, srv.Client())
		c.now = func() time.Time { return now }
		ps := []Passage{{ID: "a", Name: "n", Content: "same"}, {ID: "b", Name: "n", Content: "same"}}
		res, usage, err := c.Nouls(context.Background(), "msg", ps)
		if err != nil || usage.Calls != 1 || hits != 1 {
			t.Fatalf("err %v usage %+v hits %d", err, usage, hits)
		}
		if !res[0].Have || !res[1].Have || res[0].Noul != 0.25 || res[1].Noul != 0.25 {
			t.Fatalf("%+v", res)
		}
		res, usage, err = c.Nouls(context.Background(), "msg", ps)
		if err != nil || usage.Calls != 0 || usage.CacheHits != 2 || hits != 1 {
			t.Fatalf("err %v usage %+v hits %d res %+v", err, usage, hits, res)
		}
		now = now.Add(cfg.CacheTTL + time.Second)
		_, usage, err = c.Nouls(context.Background(), "msg", ps[:1])
		if err != nil || usage.Calls != 1 || hits != 2 {
			t.Fatalf("after ttl err %v usage %+v hits %d", err, usage, hits)
		}
	})

	t.Run("cap", func(t *testing.T) {
		var mu sync.Mutex
		var bodies [][]byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, raw)
			mu.Unlock()
			var req apiRequest
			_ = json.Unmarshal(raw, &req)
			answers := map[string]any{}
			for id := range req.Questions {
				answers[id] = map[string]any{"type": "noul", "noul": 0.01}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"model": defaultModel, "answers": answers, "usage": map[string]int{"input_tokens": 3, "output_tokens": 1}})
		}))
		defer srv.Close()
		cfg := cfg
		cfg.URL = srv.URL
		cfg.Timeout = time.Second
		cfg.MaxCalls = 1
		one := []Passage{{ID: "a", Name: "a", Content: strings.Repeat("a", 80)}}
		cfg.PerRequestTokens = estimateTokens(cfg.ChannelContext) + questionTokens(cfg, "m", one[0])
		cfg.PerQuestionTokens = cfg.PerRequestTokens
		c := New(cfg, srv.Client())
		ps := []Passage{
			{ID: "a", Name: "a", Content: strings.Repeat("a", 80)},
			{ID: "b", Name: "b", Content: strings.Repeat("b", 80)},
			{ID: "c", Name: "c", Content: strings.Repeat("c", 80)},
		}
		res, usage, err := c.Nouls(context.Background(), "m", ps)
		if err != nil || usage.Calls != 1 {
			t.Fatalf("err %v calls %d", err, usage.Calls)
		}
		keptScored := 0
		capped := 0
		for _, r := range res {
			if r.Have {
				keptScored++
			}
			if r.Reason == "call_cap" {
				capped++
			}
		}
		if keptScored != 1 || capped != 2 {
			t.Fatalf("scored %d capped %d %+v", keptScored, capped, res)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(bodies) != 1 {
			t.Fatalf("bodies %d", len(bodies))
		}
		var req apiRequest
		if err := json.Unmarshal(bodies[0], &req); err != nil {
			t.Fatal(err)
		}
		if len(req.Questions) != 1 {
			t.Fatalf("batched %d", len(req.Questions))
		}
	})
}

func TestPack_SplitsAndCaps(t *testing.T) {
	items := []askItem{
		{idx: 0, tokens: 10},
		{idx: 1, tokens: 10},
		{idx: 2, tokens: 10},
		{idx: 3, tokens: 100},
	}
	batches, overflow, oversize := pack(items, 5, 30, 50, 2)
	if len(oversize) != 1 || oversize[0].idx != 3 {
		t.Fatalf("oversize %+v", oversize)
	}
	if len(batches) != 2 {
		t.Fatalf("batches %d %+v", len(batches), batches)
	}
	if len(batches[0]) != 2 || len(batches[1]) != 1 || len(overflow) != 0 {
		t.Fatalf("batches %+v overflow %+v", batches, overflow)
	}
	batches, overflow, oversize = pack(items[:3], 5, 20, 40, 1)
	if len(batches) != 1 || len(batches[0]) != 1 || len(overflow) != 2 || len(oversize) != 0 {
		t.Fatalf("cap batches %+v overflow %+v over %+v", batches, overflow, oversize)
	}
}

func TestQuestionsFileOverride(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/q.json"
	raw := bytes.Replace(questionsJSON, []byte("Does the passage"), []byte("Does THIS_CUSTOM_QUESTION the passage"), 1)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_JEV_QUESTIONS_FILE", path)
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req apiRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, q := range req.Questions {
			seen = q.Instructions["question"]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   defaultModel,
			"answers": map[string]any{"p0": map[string]any{"type": "noul", "noul": 0.2}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.URL = srv.URL
	cfg.APIKey = "k"
	cfg.Timeout = time.Second
	c := New(cfg, srv.Client())
	_, _, err := c.Nouls(context.Background(), "hello", []Passage{{ID: "a", Content: "body"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seen, "THIS_CUSTOM_QUESTION") {
		t.Fatalf("template %s", seen)
	}
}

func TestSSRFBlocksLoopbackWhenEnabled(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("dialed")
	}))
	defer srv.Close()
	cfg := LoadConfig()
	cfg.URL = srv.URL
	cfg.APIKey = "k"
	cfg.Timeout = time.Second
	c := New(cfg, srv.Client())
	res, _, err := c.Nouls(context.Background(), "m", []Passage{{ID: "a", Content: "x"}})
	if err == nil || err.Error() != "ssrf" || res[0].Have {
		t.Fatalf("err %v %+v", err, res)
	}
}
