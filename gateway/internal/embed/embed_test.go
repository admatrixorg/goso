// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package embed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeKey is built at runtime so secret scanners see no key-shaped literal.
var fakeKey = strings.Repeat("z", 24)

type fakeServer struct {
	*httptest.Server
	calls  atomic.Int32
	inputs atomic.Int32
	status int
	dim    int
}

func newFake(t *testing.T, dim int) *fakeServer {
	f := &fakeServer{status: http.StatusOK, dim: dim}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer "+fakeKey {
			http.Error(w, "bad request", http.StatusUnauthorized)
			return
		}
		if f.status != http.StatusOK {
			// Some providers echo the key in an error body.
			http.Error(w, "invalid key "+fakeKey, f.status)
			return
		}
		var req struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions int      `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model != "fake-embed" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		f.inputs.Add(int32(len(req.Input)))
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		// Answer out of order to check the index mapping.
		for i := len(req.Input) - 1; i >= 0; i-- {
			v := make([]float32, f.dim)
			v[0] = float32(len(req.Input[i]))
			data = append(data, item{Index: i, Embedding: v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(f.Close)
	return f
}

func TestClient_BatchesOrderAndDim(t *testing.T) {
	f := newFake(t, 3)
	c := New(Config{URL: f.URL + "/v1/", Model: "fake-embed", APIKey: fakeKey, Batch: 2, RPS: 1000})
	vecs, err := c.Embed(context.Background(), []string{"a", "bb", "ccc", "dddd", "eeeee"})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls.Load() != 3 || f.inputs.Load() != 5 {
		t.Fatalf("calls=%d inputs=%d", f.calls.Load(), f.inputs.Load())
	}
	for i, v := range vecs {
		if int(v[0]) != i+1 {
			t.Fatalf("vector %d out of order: %v", i, v)
		}
	}
	if c.Dim() != 3 {
		t.Fatalf("learned dim %d", c.Dim())
	}
	// A configured dim that the endpoint does not honour is an error.
	strict := New(Config{URL: f.URL + "/v1", Model: "fake-embed", APIKey: fakeKey, Dim: 8, RPS: 1000})
	if _, err := strict.Embed(context.Background(), []string{"x"}); !errors.Is(err, ErrResponse) {
		t.Fatalf("dim mismatch: %v", err)
	}
}

func TestClient_ErrorNeverEchoesKey(t *testing.T) {
	f := newFake(t, 3)
	f.status = http.StatusUnauthorized
	c := New(Config{URL: f.URL + "/v1", Model: "fake-embed", APIKey: fakeKey, RPS: 1000})
	_, err := c.Embed(context.Background(), []string{"x"})
	if !errors.Is(err, ErrResponse) || strings.Contains(err.Error(), fakeKey) || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err %v", err)
	}
}

func TestClient_RateLimit(t *testing.T) {
	f := newFake(t, 2)
	c := New(Config{URL: f.URL + "/v1", Model: "fake-embed", APIKey: fakeKey, Batch: 1, RPS: 20})
	start := time.Now()
	if _, err := c.Embed(context.Background(), []string{"a", "b", "c", "d"}); err != nil {
		t.Fatal(err)
	}
	// Four requests at 20/s: three gaps of 50ms.
	if el := time.Since(start); el < 140*time.Millisecond {
		t.Fatalf("rate limit not applied: %v", el)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("GOSO_EMBED_URL", "")
	t.Setenv("GOSO_EMBED_MODEL", "")
	cfg, err := FromEnv()
	if err != nil || cfg.Configured() || New(cfg) != nil {
		t.Fatalf("unset env: %+v %v", cfg, err)
	}
	t.Setenv("GOSO_EMBED_URL", "http://127.0.0.1:1/v1")
	t.Setenv("GOSO_EMBED_MODEL", "fake-embed")
	t.Setenv("GOSO_EMBED_DIM", "abc")
	if _, err := FromEnv(); err == nil {
		t.Fatal("bad dim accepted")
	}
	t.Setenv("GOSO_EMBED_DIM", "768")
	cfg, err = FromEnv()
	if err != nil || !cfg.Configured() || cfg.Dim != 768 || cfg.RPS != DefaultRPS || cfg.Batch != DefaultBatch {
		t.Fatalf("cfg %+v %v", cfg, err)
	}
}
