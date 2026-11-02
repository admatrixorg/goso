// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package replycheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

type evalFile struct {
	Shop      string `json:"shop"`
	Customers map[string]struct {
		Name   string `json:"name"`
		Memory string `json:"memory"`
	} `json:"customers"`
	Samples []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"` // invented | leak | clean
		Customer string `json:"customer"`
		Prompt   string `json:"prompt"`
		Reply    string `json:"reply"`
	} `json:"samples"`
}

func loadEval(t *testing.T) evalFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/eval.json")
	if err != nil {
		t.Fatal(err)
	}
	var ef evalFile
	if err := json.Unmarshal(raw, &ef); err != nil {
		t.Fatal(err)
	}
	return ef
}

func evalInput(ef evalFile, i int) Input {
	s := ef.Samples[i]
	c := ef.Customers[s.Customer]
	in := Input{Reply: s.Reply, Shop: []string{ef.Shop}, Customer: []string{s.Prompt}}
	if c.Memory != "" {
		in.Memory = []string{c.Memory}
	}
	if c.Name != "" {
		in.Customer = append(in.Customer, c.Name)
	}
	return in
}

func TestEvalSetShape(t *testing.T) {
	ef := loadEval(t)
	n := map[string]int{}
	for _, s := range ef.Samples {
		n[s.Kind]++
		if _, ok := ef.Customers[s.Customer]; !ok {
			t.Fatalf("%s: unknown customer %q", s.ID, s.Customer)
		}
	}
	if len(ef.Samples) < 50 || n["invented"] == 0 || n["leak"] == 0 || n["clean"] == 0 {
		t.Fatalf("eval set too small or unbalanced: %v", n)
	}
}

// TestEvalLive runs the synthetic eval set against the real Clef endpoint.
// Opt-in: GOSO_CLEF_EVAL=1 plus CLOUDFLARE_ACCOUNT_ID / CLOUDFLARE_AUTH_TOKEN.
// Only masked synthetic text is sent; nothing reaches a customer channel.
func TestEvalLive(t *testing.T) {
	if os.Getenv("GOSO_CLEF_EVAL") != "1" {
		t.Skip("set GOSO_CLEF_EVAL=1 to run the live Clef eval")
	}
	cfg := FromEnv()
	cfg.Mode = ModeEnforce
	if cfg.URL == "" || cfg.Token == "" {
		t.Skip("Clef credentials not set")
	}
	ck := New(cfg, nil)
	ef := loadEval(t)
	var (
		caught, bad, falseHold, clean, clefErr, tokens int
		lat, clefLat                                   []time.Duration
	)
	// Warm the connection the way a running bridge would be warm.
	ck.Check(context.Background(), Input{Reply: "Dạ em chào chị ạ.", Shop: []string{ef.Shop}})
	for i, s := range ef.Samples {
		v := ck.Check(context.Background(), evalInput(ef, i))
		lat = append(lat, v.Latency)
		if v.ClefCalled {
			clefLat = append(clefLat, v.Latency)
		}
		tokens += v.InputTokens
		if v.ClefError != "" {
			clefErr++
		}
		ok := v.Hold == (s.Kind != "clean")
		if s.Kind == "clean" {
			clean++
			if v.Hold {
				falseHold++
			}
		} else {
			bad++
			if v.Hold {
				caught++
			}
		}
		mark := "ok "
		if !ok {
			mark = "MISS"
		}
		t.Logf("%s %-6s %-8s hold=%-5v offer=%.2f unsup=%.2f leak=%.2f %4dms %s | %s",
			mark, s.ID, s.Kind, v.Hold, v.Offer, v.Unsupported, v.Leak, v.Latency.Milliseconds(),
			strings.Join(v.Reasons, ","), v.Masked)
	}
	p := func(d []time.Duration, q float64) time.Duration {
		if len(d) == 0 {
			return 0
		}
		s := append([]time.Duration(nil), d...)
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		return s[int(float64(len(s)-1)*q+0.5)]
	}
	summary := fmt.Sprintf("model=%s samples=%d catch=%d/%d (%.1f%%) false_hold=%d/%d (%.1f%%) latency_all p50=%v p95=%v max=%v clef_calls=%d p50=%v p95=%v clef_errors=%d input_tokens=%d",
		cfg.Model, len(ef.Samples), caught, bad, 100*float64(caught)/float64(bad), falseHold, clean, 100*float64(falseHold)/float64(clean),
		p(lat, .5).Round(time.Millisecond), p(lat, .95).Round(time.Millisecond), p(lat, 1).Round(time.Millisecond),
		len(clefLat), p(clefLat, .5).Round(time.Millisecond), p(clefLat, .95).Round(time.Millisecond), clefErr, tokens)
	t.Log(summary)
	if out := os.Getenv("GOSO_CLEF_EVAL_OUT"); out != "" {
		_ = os.WriteFile(out, []byte(summary+"\n"), 0o644)
	}
}
