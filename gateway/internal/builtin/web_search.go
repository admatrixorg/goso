// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
)

// web_search providers. Brave, Exa and Tavily need an API key from env;
// DuckDuckGo Instant Answer needs none and is the last resort.
const (
	SearchBrave  = "brave"
	SearchExa    = "exa"
	SearchTavily = "tavily"
	SearchDDG    = "ddg"
)

// DefaultSearchOrder is used when GOSO_WEB_SEARCH is 1/on/true.
var DefaultSearchOrder = []string{SearchBrave, SearchExa, SearchTavily, SearchDDG}

// Provider endpoints. Tests replace them with httptest URLs; an empty base
// takes that provider out of the chain (never a live fallback).
var (
	BraveSearchBase  = "https://api.search.brave.com/res/v1/web/search"
	ExaSearchBase    = "https://api.exa.ai/search"
	TavilySearchBase = "https://api.tavily.com/search"
)

// WebSearchClient is the HTTP client for the keyed providers. Nil → 15s default.
var WebSearchClient *http.Client

const (
	maxSnippetRunes = 500
	maxSearchBody   = 1 << 20
)

var searchKeyEnv = map[string]string{
	SearchBrave:  "GOSO_BRAVE_API_KEY",
	SearchExa:    "GOSO_EXA_API_KEY",
	SearchTavily: "GOSO_TAVILY_API_KEY",
}

// WebSearchOrder is the provider chain from GOSO_WEB_SEARCH: empty = off,
// "1"/"on"/"true" = brave,exa,tavily,ddg, "ddg" = DuckDuckGo only, or an
// explicit comma list such as "tavily,brave,ddg". Unknown names are dropped.
func WebSearchOrder() []string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("GOSO_WEB_SEARCH")))
	switch v {
	case "", "0", "off", "false":
		return nil
	case "1", "on", "true":
		return append([]string(nil), DefaultSearchOrder...)
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if seen[p] {
			continue
		}
		switch p {
		case SearchBrave, SearchExa, SearchTavily, SearchDDG:
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// WebSearchNetworkAllowed is the env gate (independent of the UI flag).
func WebSearchNetworkAllowed() bool {
	return len(WebSearchOrder()) > 0
}

func searchKey(provider string) string {
	if k, ok := searchKeyEnv[provider]; ok {
		return strings.TrimSpace(os.Getenv(k))
	}
	return ""
}

func searchBase(provider string) string {
	switch provider {
	case SearchBrave:
		return strings.TrimSpace(BraveSearchBase)
	case SearchExa:
		return strings.TrimSpace(ExaSearchBase)
	case SearchTavily:
		return strings.TrimSpace(TavilySearchBase)
	case SearchDDG:
		return strings.TrimSpace(InstantAnswerBase)
	}
	return ""
}

// searchReady reports a provider that can run: a base URL, and a key unless DDG.
func searchReady(provider string) bool {
	if searchBase(provider) == "" {
		return false
	}
	return provider == SearchDDG || searchKey(provider) != ""
}

// webSearchConfigured reports whether at least one provider in the chain can run.
func webSearchConfigured() bool {
	for _, p := range WebSearchOrder() {
		if searchReady(p) {
			return true
		}
	}
	return false
}

func queryArg(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, k := range []string{"q", "query"} {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func countArg(args map[string]any) int {
	n := maxSearchHits
	for _, k := range []string{"count", "max_results"} {
		switch v := args[k].(type) {
		case float64:
			n = int(v)
		case int:
			n = v
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				n = i
			}
		default:
			continue
		}
		break
	}
	if n < 1 {
		n = 1
	}
	if n > maxSearchHits {
		n = maxSearchHits
	}
	return n
}

// webSearch tries each ready provider in order. An error or an empty answer
// moves on to the next one. Errors name the provider and the HTTP status only:
// keys travel in headers and never appear in a result, error or log.
func webSearch(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	q := queryArg(args)
	if q == "" {
		// Empty query is fail-closed (not_configured), no network.
		return notConfigured(ToolWebSearch), nil
	}
	count := countArg(args)
	var tried []map[string]any
	var failures []string
	var empty *connector.InvokeResult
	ran := false
	for _, p := range WebSearchOrder() {
		if !searchReady(p) {
			continue
		}
		ran = true
		hits, raw, err := runSearch(ctx, p, q, count)
		if err != nil {
			tried = append(tried, map[string]any{"provider": p, "error": err.Error()})
			failures = append(failures, p+": "+err.Error())
			continue
		}
		res := &connector.InvokeResult{
			Tool:      ToolWebSearch,
			Connector: ConnectorName,
			Status:    "ok",
			Content:   map[string]any{"provider": p, "results": hits},
		}
		if p == SearchDDG {
			res.Raw = raw
		}
		if len(hits) == 0 {
			tried = append(tried, map[string]any{"provider": p, "error": "no results"})
			if empty == nil {
				empty = res
			}
			continue
		}
		if len(tried) > 0 {
			res.Content.(map[string]any)["fallback_from"] = tried
		}
		return res, nil
	}
	if !ran {
		return notConfigured(ToolWebSearch), nil
	}
	if empty != nil {
		return empty, nil
	}
	return nil, fmt.Errorf("web_search: all providers failed (%s)", strings.Join(failures, "; "))
}

func runSearch(ctx context.Context, provider, q string, count int) ([]map[string]any, []byte, error) {
	switch provider {
	case SearchBrave:
		return searchBrave(ctx, q, count)
	case SearchExa:
		return searchExa(ctx, q, count)
	case SearchTavily:
		return searchTavily(ctx, q, count)
	case SearchDDG:
		return searchDDG(ctx, q, count)
	}
	return nil, nil, errors.New("unknown provider")
}

func keyedClient() *http.Client {
	if WebSearchClient != nil {
		return WebSearchClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// doSearch sends req and returns the body of a 200 answer.
func doSearch(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(req.Context().Err(), context.Canceled) || errors.Is(req.Context().Err(), context.DeadlineExceeded) {
			return nil, errors.New("timeout")
		}
		return nil, errors.New("request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSearchBody))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return body, nil
}

func searchBrave(ctx context.Context, q string, count int) ([]map[string]any, []byte, error) {
	u, err := url.Parse(searchBase(SearchBrave))
	if err != nil {
		return nil, nil, errors.New("bad base url")
	}
	qs := u.Query()
	qs.Set("q", q)
	qs.Set("count", strconv.Itoa(count))
	u.RawQuery = qs.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, errors.New("bad request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", searchKey(SearchBrave))
	body, err := doSearch(keyedClient(), req)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil, nil, errors.New("decode")
	}
	hits := make([]map[string]any, 0, count)
	for _, r := range out.Web.Results {
		if len(hits) >= count {
			break
		}
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		hits = append(hits, searchHit(stripTags(r.Title), strings.TrimSpace(r.URL), clip(stripTags(r.Description))))
	}
	return hits, body, nil
}

func postSearch(ctx context.Context, provider string, payload any, auth func(*http.Request)) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("encode")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchBase(provider), bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("bad request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	auth(req)
	return doSearch(keyedClient(), req)
}

func searchExa(ctx context.Context, q string, count int) ([]map[string]any, []byte, error) {
	body, err := postSearch(ctx, SearchExa, map[string]any{
		"query":      q,
		"numResults": count,
		"contents":   map[string]any{"text": map[string]any{"maxCharacters": maxSnippetRunes}},
	}, func(r *http.Request) { r.Header.Set("x-api-key", searchKey(SearchExa)) })
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Results []struct {
			Title      string   `json:"title"`
			URL        string   `json:"url"`
			Text       string   `json:"text"`
			Summary    string   `json:"summary"`
			Highlights []string `json:"highlights"`
		} `json:"results"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil, nil, errors.New("decode")
	}
	hits := make([]map[string]any, 0, count)
	for _, r := range out.Results {
		if len(hits) >= count {
			break
		}
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		snippet := r.Summary
		if len(r.Highlights) > 0 && strings.TrimSpace(r.Highlights[0]) != "" {
			snippet = r.Highlights[0]
		}
		if strings.TrimSpace(snippet) == "" {
			snippet = r.Text
		}
		hits = append(hits, searchHit(strings.TrimSpace(r.Title), strings.TrimSpace(r.URL), clip(snippet)))
	}
	return hits, body, nil
}

func searchTavily(ctx context.Context, q string, count int) ([]map[string]any, []byte, error) {
	body, err := postSearch(ctx, SearchTavily, map[string]any{
		"query":        q,
		"max_results":  count,
		"search_depth": "basic",
	}, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+searchKey(SearchTavily)) })
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil, nil, errors.New("decode")
	}
	hits := make([]map[string]any, 0, count)
	for _, r := range out.Results {
		if len(hits) >= count {
			break
		}
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		hits = append(hits, searchHit(strings.TrimSpace(r.Title), strings.TrimSpace(r.URL), clip(r.Content)))
	}
	return hits, body, nil
}

func searchDDG(ctx context.Context, q string, count int) ([]map[string]any, []byte, error) {
	u, err := url.Parse(searchBase(SearchDDG))
	if err != nil {
		return nil, nil, errors.New("bad base url")
	}
	qs := u.Query()
	qs.Set("q", q)
	qs.Set("format", "json")
	qs.Set("no_html", "1")
	qs.Set("skip_disambig", "1")
	u.RawQuery = qs.Encode()
	client := InstantAnswerClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, errors.New("bad request")
	}
	body, err := doSearch(client, req)
	if err != nil {
		return nil, nil, err
	}
	hits, err := mapSearchHits(body)
	if err != nil {
		return nil, nil, errors.New("decode")
	}
	if len(hits) > count {
		hits = hits[:count]
	}
	return hits, body, nil
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string {
	return strings.TrimSpace(tagRe.ReplaceAllString(s, ""))
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > maxSnippetRunes {
		return string(r[:maxSnippetRunes]) + "…"
	}
	return s
}

func mapSearchHits(body []byte) ([]map[string]any, error) {
	var ddg struct {
		AbstractText  string            `json:"AbstractText"`
		Abstract      string            `json:"Abstract"`
		AbstractURL   string            `json:"AbstractURL"`
		Answer        string            `json:"Answer"`
		Heading       string            `json:"Heading"`
		RelatedTopics []json.RawMessage `json:"RelatedTopics"`
		Results       []json.RawMessage `json:"Results"`
	}
	if err := json.Unmarshal(body, &ddg); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, maxSearchHits)
	snippet := strings.TrimSpace(ddg.AbstractText)
	if snippet == "" {
		snippet = strings.TrimSpace(ddg.Abstract)
	}
	if snippet == "" {
		snippet = strings.TrimSpace(ddg.Answer)
	}
	title := strings.TrimSpace(ddg.Heading)
	u := strings.TrimSpace(ddg.AbstractURL)
	if title != "" || u != "" || snippet != "" {
		if title == "" {
			title = snippet
		}
		out = append(out, searchHit(title, u, snippet))
	}
	appendTopicHits(&out, ddg.Results)
	appendTopicHits(&out, ddg.RelatedTopics)
	return out, nil
}

func appendTopicHits(out *[]map[string]any, raw []json.RawMessage) {
	for _, r := range raw {
		if len(*out) >= maxSearchHits {
			return
		}
		var item struct {
			Text     string            `json:"Text"`
			FirstURL string            `json:"FirstURL"`
			Topics   []json.RawMessage `json:"Topics"`
		}
		if json.Unmarshal(r, &item) != nil {
			continue
		}
		text := strings.TrimSpace(item.Text)
		u := strings.TrimSpace(item.FirstURL)
		if text != "" || u != "" {
			title, snippet := splitTopicText(text)
			*out = append(*out, searchHit(title, u, snippet))
		}
		if len(item.Topics) > 0 {
			appendTopicHits(out, item.Topics)
		}
	}
}

func splitTopicText(text string) (title, snippet string) {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, " - "); i > 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+3:])
	}
	return text, text
}

func searchHit(title, u, snippet string) map[string]any {
	return map[string]any{
		"title":   title,
		"url":     u,
		"snippet": snippet,
	}
}
