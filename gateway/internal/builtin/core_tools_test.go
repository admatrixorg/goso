// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
)

// TestMain drops search keys a developer may have exported, so no test can
// reach a real search API.
func TestMain(m *testing.M) {
	for _, k := range searchKeyEnv {
		_ = os.Unsetenv(k)
	}
	os.Exit(m.Run())
}

func fixClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := Now
	Now = func() time.Time { return at }
	t.Cleanup(func() { Now = prev })
}

func TestDatetime_DefaultTimezone(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 16, 1, 30, 0, 0, time.UTC))
	res, err := Invoke(context.Background(), ToolDatetime, nil, false)
	if err != nil || res == nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	c := res.Content.(map[string]any)
	if c["timezone"] != DefaultTimezone || c["iso"] != "2026-11-16T08:30:00+07:00" {
		t.Fatalf("%+v", c)
	}
	if c["weekday"] != "Monday" || c["date"] != "2026-11-16" || c["time"] != "08:30:00" || c["utc_offset"] != "+07:00" {
		t.Fatalf("%+v", c)
	}
	if c["unix"] != int64(1794792600) {
		t.Fatalf("unix %v", c["unix"])
	}
}

func TestDatetime_NamedTimezone(t *testing.T) {
	fixClock(t, time.Date(2026, 11, 16, 1, 30, 0, 0, time.UTC))
	res, _ := Invoke(context.Background(), ToolDatetime, map[string]any{"timezone": "Europe/London"}, false)
	c := res.Content.(map[string]any)
	if res.Status != "ok" || c["iso"] != "2026-11-16T01:30:00Z" || c["timezone"] != "Europe/London" {
		t.Fatalf("%+v", res)
	}
}

func TestDatetime_InvalidTimezone(t *testing.T) {
	for _, tz := range []string{"Mars/Olympus", "Local", "../etc/passwd"} {
		res, err := Invoke(context.Background(), ToolDatetime, map[string]any{"timezone": tz}, false)
		if err != nil || res.Status != "error" {
			t.Fatalf("%s: %v %+v", tz, err, res)
		}
	}
	if !Configured(ToolDatetime) {
		t.Fatal("datetime is always configured")
	}
}

func TestMessage_NotConfiguredWithoutSender(t *testing.T) {
	prev := MessageSender
	MessageSender = nil
	t.Cleanup(func() { MessageSender = prev })
	if Configured(ToolMessage) {
		t.Fatal("no sender: not configured")
	}
	res, err := Invoke(context.Background(), ToolMessage, map[string]any{"text": "Chào Minh An"}, true)
	if err != nil || res.Status != "not_configured" {
		t.Fatalf("%v %+v", err, res)
	}
	_, bad := Prepare(context.Background(), ToolMessage, map[string]any{"text": "x"})
	if bad == nil || bad.Status != "not_configured" {
		t.Fatalf("%+v", bad)
	}
	args := map[string]any{"path": "a.txt"}
	got, bad := Prepare(context.Background(), ToolWriteFile, args)
	if bad != nil || got["path"] != "a.txt" {
		t.Fatal("other tools pass through Prepare")
	}
}

type fakeMessenger struct {
	resolved, sent int
	caller         Caller
}

func (f *fakeMessenger) Resolve(ctx context.Context, args map[string]any) (map[string]any, *connector.InvokeResult) {
	f.resolved++
	f.caller = CallerFrom(ctx)
	return map[string]any{"text": args["text"], "channel": "t1/telegram/shop", "chat_id": "42"}, nil
}

func (f *fakeMessenger) Send(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	f.sent++
	return &connector.InvokeResult{Tool: ToolMessage, Connector: ConnectorName, Status: "ok", Content: args}, nil
}

func TestMessage_PrepareAndInvokeUseSender(t *testing.T) {
	prev := MessageSender
	f := &fakeMessenger{}
	MessageSender = f
	t.Cleanup(func() { MessageSender = prev })
	ctx := WithCaller(context.Background(), "agent-1", "sess-1")
	args, bad := Prepare(ctx, ToolMessage, map[string]any{"text": "hi"})
	if bad != nil || args["channel"] != "t1/telegram/shop" || f.caller.AgentID != "agent-1" || f.caller.SessionID != "sess-1" {
		t.Fatalf("%+v %+v %+v", args, bad, f.caller)
	}
	res, err := Invoke(ctx, ToolMessage, args, false)
	if err != nil || res.Status != "ok" || f.sent != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	// Empty values keep the outer caller.
	inner := WithCaller(ctx, "", "")
	if c := CallerFrom(inner); c.AgentID != "agent-1" || c.SessionID != "sess-1" {
		t.Fatalf("%+v", c)
	}
}

func TestWebSearchOrder_Parse(t *testing.T) {
	cases := map[string][]string{
		"":                   nil,
		"off":                nil,
		"0":                  nil,
		"1":                  DefaultSearchOrder,
		"on":                 DefaultSearchOrder,
		"ddg":                {SearchDDG},
		"tavily, brave":      {SearchTavily, SearchBrave},
		"exa,nope,ddg":       {SearchExa, SearchDDG},
		"BRAVE,brave,tavily": {SearchBrave, SearchTavily},
	}
	for in, want := range cases {
		t.Setenv("GOSO_WEB_SEARCH", in)
		got := WebSearchOrder()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%q: got %v want %v", in, got, want)
		}
	}
}

// searchServers stands up one httptest server per provider and points the
// package at them. status sets what each answers; hits counts calls.
type searchServers struct {
	status map[string]int
	hits   map[string]int
	auth   map[string]string
}

func newSearchServers(t *testing.T) *searchServers {
	t.Helper()
	ss := &searchServers{status: map[string]int{}, hits: map[string]int{}, auth: map[string]string{}}
	handler := func(p string, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ss.hits[p]++
			switch p {
			case SearchBrave:
				ss.auth[p] = r.Header.Get("X-Subscription-Token")
				if r.URL.Query().Get("q") == "" {
					t.Errorf("brave: no q")
				}
			case SearchExa:
				ss.auth[p] = r.Header.Get("x-api-key")
			case SearchTavily:
				ss.auth[p] = r.Header.Get("Authorization")
			}
			if r.Method == http.MethodPost {
				b, _ := io.ReadAll(r.Body)
				var m map[string]any
				if json.Unmarshal(b, &m) != nil || m["query"] != "phở Hà Nội" {
					t.Errorf("%s: body %s", p, b)
				}
			}
			if st := ss.status[p]; st != 0 && st != http.StatusOK {
				http.Error(w, "nope", st)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}
	}
	bodies := map[string]string{
		SearchBrave:  `{"web":{"results":[{"title":"Phở <strong>ngon</strong>","url":"https://a.example/1","description":"Quán của Minh An"}]}}`,
		SearchExa:    `{"results":[{"title":"Exa hit","url":"https://b.example/1","text":"exa text"}]}`,
		SearchTavily: `{"results":[{"title":"Tavily hit","url":"https://c.example/1","content":"tavily text"}]}`,
		SearchDDG:    `{"Heading":"Pho","AbstractText":"Vietnamese soup","AbstractURL":"https://d.example/pho","RelatedTopics":[]}`,
	}
	srv := map[string]*httptest.Server{}
	for _, p := range DefaultSearchOrder {
		srv[p] = httptest.NewServer(handler(p, bodies[p]))
		t.Cleanup(srv[p].Close)
	}
	pb, pe, pt, pd := BraveSearchBase, ExaSearchBase, TavilySearchBase, InstantAnswerBase
	pc, pi := WebSearchClient, InstantAnswerClient
	BraveSearchBase, ExaSearchBase, TavilySearchBase, InstantAnswerBase = srv[SearchBrave].URL, srv[SearchExa].URL, srv[SearchTavily].URL, srv[SearchDDG].URL
	WebSearchClient, InstantAnswerClient = srv[SearchBrave].Client(), srv[SearchDDG].Client()
	t.Cleanup(func() {
		BraveSearchBase, ExaSearchBase, TavilySearchBase, InstantAnswerBase = pb, pe, pt, pd
		WebSearchClient, InstantAnswerClient = pc, pi
	})
	return ss
}

func setSearchKeys(t *testing.T) {
	t.Setenv("GOSO_BRAVE_API_KEY", "test-brave-key")
	t.Setenv("GOSO_EXA_API_KEY", "test-exa-key")
	t.Setenv("GOSO_TAVILY_API_KEY", "test-tavily-key")
}

func searchQuery() map[string]any { return map[string]any{"q": "phở Hà Nội", "count": 3} }

func TestWebSearch_BraveFirst(t *testing.T) {
	ss := newSearchServers(t)
	setSearchKeys(t)
	t.Setenv("GOSO_WEB_SEARCH", "1")
	res, err := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	c := res.Content.(map[string]any)
	hits := c["results"].([]map[string]any)
	if c["provider"] != SearchBrave || len(hits) != 1 || hits[0]["title"] != "Phở ngon" || hits[0]["url"] != "https://a.example/1" || hits[0]["snippet"] != "Quán của Minh An" {
		t.Fatalf("%+v", c)
	}
	if ss.auth[SearchBrave] != "test-brave-key" || ss.hits[SearchExa] != 0 {
		t.Fatalf("%+v %+v", ss.auth, ss.hits)
	}
	if res.Raw != nil {
		t.Fatal("keyed providers keep raw out of the result")
	}
}

func TestWebSearch_FallbackChain(t *testing.T) {
	ss := newSearchServers(t)
	setSearchKeys(t)
	t.Setenv("GOSO_WEB_SEARCH", "1")
	ss.status[SearchBrave] = http.StatusTooManyRequests
	ss.status[SearchExa] = http.StatusUnauthorized
	res, err := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	c := res.Content.(map[string]any)
	if c["provider"] != SearchTavily || ss.auth[SearchTavily] != "Bearer test-tavily-key" || ss.auth[SearchExa] != "test-exa-key" {
		t.Fatalf("%+v %+v", c, ss.auth)
	}
	from := c["fallback_from"].([]map[string]any)
	if len(from) != 2 || from[0]["provider"] != SearchBrave || from[1]["provider"] != SearchExa {
		t.Fatalf("%+v", from)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "test-") {
		t.Fatal("key leaked into result")
	}
}

func TestWebSearch_DDGLastResort(t *testing.T) {
	ss := newSearchServers(t)
	setSearchKeys(t)
	t.Setenv("GOSO_WEB_SEARCH", "1")
	ss.status[SearchBrave] = http.StatusInternalServerError
	ss.status[SearchExa] = http.StatusInternalServerError
	ss.status[SearchTavily] = http.StatusInternalServerError
	res, err := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if err != nil || res.Content.(map[string]any)["provider"] != SearchDDG {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestWebSearch_SkipsProvidersWithoutKey(t *testing.T) {
	ss := newSearchServers(t)
	t.Setenv("GOSO_EXA_API_KEY", "test-exa-key")
	t.Setenv("GOSO_WEB_SEARCH", "brave,exa")
	res, err := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if err != nil || res.Content.(map[string]any)["provider"] != SearchExa || ss.hits[SearchBrave] != 0 {
		t.Fatalf("%v %+v %+v", err, res, ss.hits)
	}
	if _, ok := res.Content.(map[string]any)["fallback_from"]; ok {
		t.Fatal("an unready provider is not a fallback")
	}
}

func TestWebSearch_AllFailNoKeyInError(t *testing.T) {
	ss := newSearchServers(t)
	setSearchKeys(t)
	t.Setenv("GOSO_WEB_SEARCH", "brave,tavily")
	ss.status[SearchBrave] = http.StatusForbidden
	ss.status[SearchTavily] = http.StatusBadGateway
	_, err := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if err == nil || !strings.Contains(err.Error(), "brave: http 403") || !strings.Contains(err.Error(), "tavily: http 502") {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "test-") {
		t.Fatal("key leaked into error")
	}
}

func TestWebSearch_KeysOnlyNotConfigured(t *testing.T) {
	ss := newSearchServers(t)
	setSearchKeys(t)
	t.Setenv("GOSO_WEB_SEARCH", "")
	if Configured(ToolWebSearch) {
		t.Fatal("keys alone must not enable web_search")
	}
	res, _ := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if res.Status != "not_configured" || len(ss.hits) != 0 {
		t.Fatalf("%+v %+v", res, ss.hits)
	}
	t.Setenv("GOSO_WEB_SEARCH", "1")
	res, _ = Invoke(context.Background(), ToolWebSearch, searchQuery(), false)
	if res.Status != "not_configured" || len(ss.hits) != 0 {
		t.Fatal("UI flag off must not search")
	}
}

func TestWebSearch_NoProviderReady(t *testing.T) {
	ss := newSearchServers(t)
	t.Setenv("GOSO_WEB_SEARCH", "brave,exa,tavily")
	if Configured(ToolWebSearch) {
		t.Fatal("no keys: not configured")
	}
	res, err := Invoke(context.Background(), ToolWebSearch, searchQuery(), true)
	if err != nil || res.Status != "not_configured" || len(ss.hits) != 0 {
		t.Fatalf("%v %+v %+v", err, res, ss.hits)
	}
}

func TestWebSearch_CountClamp(t *testing.T) {
	for in, want := range map[any]int{nil: maxSearchHits, float64(3): 3, float64(0): 1, float64(99): maxSearchHits, "2": 2} {
		args := map[string]any{}
		if in != nil {
			args["count"] = in
		}
		if got := countArg(args); got != want {
			t.Fatalf("%v: %d want %d", in, got, want)
		}
	}
}
