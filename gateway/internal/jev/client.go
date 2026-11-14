// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// Passage is one knowledge chunk. The text is pasted into its own question.
// The id is for logs and for mapping the answer back; the model is not asked
// to look the chunk up somewhere else in the state.
type Passage struct {
	ID      string
	Name    string
	Content string
}

// Item is one question: a customer message plus the passage to judge.
type Item struct {
	Message string
	Passage Passage
}

// Result is one passage after a call, a cache hit, or a fallback.
type Result struct {
	Passage Passage
	Noul    float64
	Have    bool
	Reason  string
	Cached  bool
}

// Usage is token accounting from the Jev responses in one Score call.
type Usage struct {
	Calls        int
	CacheHits    int
	Deduped      int
	InputTokens  int
	OutputTokens int
	Model        string
}

// Client calls POST /v1/systemone.
type Client struct {
	cfg   Config
	http  *http.Client
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]cacheEntry
	order []string
}

type cacheEntry struct {
	noul  float64
	until time.Time
}

type askItem struct {
	idx     int
	hash    string
	tokens  int
	message string
	p       Passage
}

// New builds a client. httpClient may be nil.
func New(cfg Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout + time.Second}
		security.GuardClient(httpClient)
	}
	if cfg.PerRequestTokens <= 0 {
		cfg.PerRequestTokens = defaultPerRequestTok
	}
	if cfg.PerQuestionTokens <= 0 {
		cfg.PerQuestionTokens = defaultPerQuestionTok
	}
	if cfg.MaxCalls <= 0 {
		cfg.MaxCalls = defaultMaxCalls
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	return &Client{
		cfg:   cfg,
		http:  httpClient,
		now:   time.Now,
		cache: map[string]cacheEntry{},
	}
}

var (
	sharedMu  sync.Mutex
	shared    *Client
	sharedFP  string
	aliasOnce sync.Once
	noKeyOnce sync.Once
)

func resetDefault() {
	sharedMu.Lock()
	shared = nil
	sharedFP = ""
	sharedMu.Unlock()
}

// ResetForTest drops the process-wide client so a test's env does not leak
// into the next test in the same process.
func ResetForTest() { resetDefault() }

func defaultClient() *Client {
	cfg := LoadConfig()
	fp := fingerprint(cfg)
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared == nil || sharedFP != fp {
		shared = New(cfg, nil)
		sharedFP = fp
		if cfg.Model == "jev-latest" || cfg.Model == "jev-preview" {
			aliasOnce.Do(func() {
				log.Printf("goso jev: model %s is an alias; the models page lists jev-latest as jev-1.14.0 and aliases move. Pin GOSO_JEV_MODEL=jev-1.14.0 once thresholds are tuned", cfg.Model)
			})
		}
	}
	return shared
}

func fingerprint(cfg Config) string {
	keySum := sha256.Sum256([]byte(cfg.APIKey))
	h := sha256.New()
	fmt.Fprintf(h, "%v %s %s %s %s %s %s %s %d %d %d %d %s %s",
		cfg.Enabled, hex.EncodeToString(keySum[:]), cfg.Model, cfg.URL,
		cfg.ContextVersion, cfg.QuestionsVersion, cfg.ContextHash, cfg.QuestionsHash,
		cfg.Timeout, cfg.MaxCalls, cfg.PerRequestTokens, cfg.PerQuestionTokens, cfg.CacheTTL, cfg.ConfigErr)
	return hex.EncodeToString(h.Sum(nil))
}

// Nouls scores every passage against one customer message.
func (c *Client) Nouls(ctx context.Context, message string, passages []Passage) ([]Result, Usage, error) {
	items := make([]Item, len(passages))
	for i, p := range passages {
		items[i] = Item{Message: message, Passage: p}
	}
	return c.Score(ctx, items)
}

// Score asks one relevance noul per item. Identical items share one question
// and one cache entry. A transport error, timeout, or call cap leaves
// Have=false on the items that were not scored.
func (c *Client) Score(ctx context.Context, items []Item) ([]Result, Usage, error) {
	if c == nil {
		return nil, Usage{}, errors.New("no_client")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	out := make([]Result, len(items))
	var usage Usage
	usage.Model = c.cfg.Model
	if c.cfg.QuestionsErr != "" {
		for i, it := range items {
			out[i] = Result{Passage: it.Passage, Reason: c.cfg.QuestionsErr}
		}
		return out, usage, errors.New(c.cfg.QuestionsErr)
	}
	pending := make([]askItem, 0, len(items))
	stateTok := estimateTokens(c.cfg.ChannelContext)
	for i, it := range items {
		out[i] = Result{Passage: it.Passage}
		msg := strings.TrimSpace(it.Message)
		h := c.hash(msg, it.Passage)
		if noul, ok := c.cached(h); ok {
			out[i].Noul = noul
			out[i].Have = true
			out[i].Reason = "cache"
			out[i].Cached = true
			usage.CacheHits++
			continue
		}
		pending = append(pending, askItem{
			idx: i, hash: h, message: msg, p: it.Passage,
			tokens: questionTokens(c.cfg, msg, it.Passage),
		})
	}
	if len(pending) == 0 {
		return out, usage, nil
	}
	batches, overflow, oversize := pack(pending, stateTok, c.cfg.PerRequestTokens, c.cfg.PerQuestionTokens, c.cfg.MaxCalls)
	for _, it := range oversize {
		out[it.idx].Reason = "oversize"
	}
	for _, it := range overflow {
		out[it.idx].Reason = "call_cap"
	}
	var callErr error
	for _, batch := range batches {
		u, answered, err := c.post(ctx, batch)
		usage.Calls++
		usage.Deduped += u.Deduped
		usage.InputTokens += u.InputTokens
		usage.OutputTokens += u.OutputTokens
		if u.Model != "" {
			usage.Model = u.Model
		}
		if err != nil {
			callErr = err
			for _, it := range batch {
				out[it.idx].Reason = err.Error()
				out[it.idx].Have = false
			}
			log.Printf("goso jev: fallback reason=%s", err.Error())
			continue
		}
		for _, it := range batch {
			if noul, ok := answered[it.idx]; ok {
				c.store(it.hash, noul)
				out[it.idx].Noul = noul
				out[it.idx].Have = true
				out[it.idx].Reason = "scored"
			} else {
				out[it.idx].Reason = "missing_answer"
			}
		}
	}
	return out, usage, callErr
}

func (c *Client) hash(message string, p Passage) string {
	h := sha256.New()
	// Versions and content hashes are both included so a context or question
	// edit invalidates cached nouls even when the version string was not bumped.
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s",
		c.cfg.Model, c.cfg.ContextVersion, c.cfg.QuestionsVersion,
		c.cfg.ContextHash, c.cfg.QuestionsHash,
		normKey(message), normKey(p.Name), normKey(p.Content))
	return hex.EncodeToString(h.Sum(nil))
}

func normKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func (c *Client) cached(h string) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[h]
	if !ok || c.now().After(e.until) {
		return 0, false
	}
	return e.noul, true
}

func (c *Client) store(h string, noul float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.cache[h]; !ok {
		c.order = append(c.order, h)
	}
	c.cache[h] = cacheEntry{noul: noul, until: c.now().Add(c.cfg.CacheTTL)}
	for len(c.cache) > cacheMaxEntries && len(c.order) > 0 {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.cache, old)
	}
}

type apiQuestion struct {
	Type         string            `json:"type"`
	Instructions map[string]string `json:"instructions"`
	Criteria     json.RawMessage   `json:"criteria"`
}

type apiRequest struct {
	Model     string                 `json:"model"`
	State     json.RawMessage        `json:"state"`
	Questions map[string]apiQuestion `json:"questions"`
}

type apiResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (c *Client) post(ctx context.Context, batch []askItem) (Usage, map[int]float64, error) {
	if err := security.CheckURL(c.cfg.URL); err != nil {
		return Usage{}, nil, errors.New("ssrf")
	}
	qs := make(map[string]apiQuestion, len(batch))
	primary := map[string]int{}
	idToIdx := map[string]int{}
	for _, it := range batch {
		if _, ok := primary[it.hash]; ok {
			continue
		}
		primary[it.hash] = it.idx
		id := fmt.Sprintf("p%d", it.idx)
		idToIdx[id] = it.idx
		qs[id] = apiQuestion{
			Type:         "noul",
			Instructions: fillInstructions(c.cfg.Instructions, it.message, it.p.Name, it.p.Content),
			Criteria:     noulCriteria(c.cfg.CriteriaTrue, c.cfg.CriteriaFalse),
		}
	}
	body, err := json.Marshal(apiRequest{Model: c.cfg.Model, State: c.cfg.ContextJSON, Questions: qs})
	if err != nil {
		return Usage{}, nil, errors.New("encode")
	}
	if len(c.cfg.APIKey) >= 8 && bytes.Contains(body, []byte(c.cfg.APIKey)) {
		return Usage{}, nil, errors.New("encode")
	}
	usage, byIdx, err := c.do(ctx, body, idToIdx)
	if err != nil {
		return usage, nil, err
	}
	answered := map[int]float64{}
	deduped := 0
	for _, it := range batch {
		if primary[it.hash] != it.idx {
			deduped++
		}
		src := primary[it.hash]
		if noul, ok := byIdx[src]; ok {
			answered[it.idx] = noul
		}
	}
	usage.Deduped = deduped
	return usage, answered, nil
}

func noulCriteria(yes, no string) json.RawMessage {
	yb, _ := json.Marshal(yes)
	nb, _ := json.Marshal(no)
	return json.RawMessage(`{"true":` + string(yb) + `,"false":` + string(nb) + `}`)
}

func (c *Client) do(ctx context.Context, body []byte, idToIdx map[string]int) (Usage, map[int]float64, error) {
	env, err := c.roundTrip(ctx, body)
	if err != nil {
		return Usage{}, nil, err
	}
	usage := Usage{InputTokens: env.Usage.InputTokens, OutputTokens: env.Usage.OutputTokens, Model: env.Model}
	byIdx := map[int]float64{}
	for id, idx := range idToIdx {
		raw, ok := env.Answers[id]
		if !ok {
			continue
		}
		var a struct {
			Noul *float64 `json:"noul"`
		}
		if json.Unmarshal(raw, &a) != nil || a.Noul == nil || math.IsNaN(*a.Noul) || *a.Noul < 0 || *a.Noul > 1 {
			continue
		}
		byIdx[idx] = *a.Noul
	}
	return usage, byIdx, nil
}

// roundTrip posts one System One body. 429 and 529 are retried with
// exponential backoff, honoring Retry-After when it fits the call budget.
// Any other failure returns so the caller can keep the current knowledge.
func (c *Client) roundTrip(ctx context.Context, body []byte) (apiResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
		if err != nil {
			return apiResponse{}, errors.New("request")
		}
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			if callCtx.Err() != nil || ctx.Err() != nil {
				return apiResponse{}, errors.New("timeout")
			}
			return apiResponse{}, errors.New("unreachable")
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == 529 {
			last = fmt.Errorf("http_%d", res.StatusCode)
			if attempt == 3 {
				return apiResponse{}, last
			}
			wait := time.Duration(200*(1<<uint(attempt))) * time.Millisecond
			if ra, ok := retryAfterWait(res.Header); ok {
				wait = ra
			}
			if !waitFits(callCtx, wait) {
				log.Printf("goso jev: retry status=%d skipped wait_ms=%d reason=budget", res.StatusCode, wait.Milliseconds())
				return apiResponse{}, errors.New("timeout")
			}
			log.Printf("goso jev: retry status=%d attempt=%d wait_ms=%d", res.StatusCode, attempt+1, wait.Milliseconds())
			if !sleepBudget(callCtx, wait) {
				return apiResponse{}, errors.New("timeout")
			}
			continue
		}
		if res.StatusCode != http.StatusOK {
			return apiResponse{}, fmt.Errorf("http_%d", res.StatusCode)
		}
		var env apiResponse
		if json.Unmarshal(raw, &env) != nil {
			return apiResponse{}, errors.New("bad_json")
		}
		if len(env.Answers) == 0 {
			return apiResponse{}, errors.New("no_answers")
		}
		model := env.Model
		if model == "" {
			model = "-"
		}
		log.Printf("goso jev: response_model=%s requested_model=%s input_tokens=%d output_tokens=%d",
			model, c.cfg.Model, env.Usage.InputTokens, env.Usage.OutputTokens)
		return env, nil
	}
	if last == nil {
		last = errors.New("unreachable")
	}
	return apiResponse{}, last
}

func retryAfterWait(h http.Header) (time.Duration, bool) {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

func waitFits(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	dl, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(dl) >= d+20*time.Millisecond
}

func sleepBudget(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	if !waitFits(ctx, d) {
		return false
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func fillInstructions(tmpl map[string]string, message, name, content string) map[string]string {
	out := make(map[string]string, len(tmpl))
	r := strings.NewReplacer("{{message}}", message, "{{name}}", name, "{{content}}", content)
	for k, v := range tmpl {
		out[k] = r.Replace(v)
	}
	return out
}

func questionTokens(cfg Config, message string, p Passage) int {
	n := 48 + estimateTokens(message) + estimateTokens(p.Name) + estimateTokens(p.Content)
	for k, v := range cfg.Instructions {
		n += estimateTokens(k) + estimateTokens(v)
	}
	n += estimateTokens(cfg.CriteriaTrue) + estimateTokens(cfg.CriteriaFalse)
	return n
}

// estimateTokens overestimates (about two bytes per token) so a packed
// request stays under the published 64k per-request and 32k per-question caps.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 1) / 2
}

func pack(items []askItem, stateTok, perReq, perQ, maxCalls int) (batches [][]askItem, overflow, oversize []askItem) {
	if maxCalls < 1 {
		maxCalls = 1
	}
	var cur []askItem
	curTok := stateTok
	flush := func() {
		if len(cur) == 0 {
			return
		}
		batches = append(batches, cur)
		cur = nil
		curTok = stateTok
	}
	for _, it := range items {
		q := it.tokens
		if stateTok+q > perQ || stateTok+q > perReq {
			oversize = append(oversize, it)
			continue
		}
		if len(batches) >= maxCalls {
			overflow = append(overflow, it)
			continue
		}
		if len(cur) > 0 && curTok+q > perReq {
			flush()
			if len(batches) >= maxCalls {
				overflow = append(overflow, it)
				continue
			}
		}
		cur = append(cur, it)
		curTok += q
	}
	if len(batches) < maxCalls {
		flush()
	} else if len(cur) > 0 {
		overflow = append(overflow, cur...)
	}
	return batches, overflow, oversize
}
