// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"net/http"
	"os"
	"strconv"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/httpapi"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/ratelimit"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Status describes the assembled gateway handler (no secrets).
type Status struct {
	Provider  string
	HasReal   bool
	Auth      bool
	RateLimit int
}

// DefaultProvider picks Anthropic, else OpenAI, else echo — same rules as the CLI.
func DefaultProvider() llm.Provider {
	reg := llm.NewRegistry()
	provider := reg.Get("anthropic")
	if !reg.HasReal() {
		return llm.Echo{}
	}
	if provider.Name() == "echo" {
		return reg.Get("openai")
	}
	return provider
}

// Mux builds the gateway ServeMux (API + channels + WS) on the given store.
func Mux(st store.StoreIface, version string, provider llm.Provider) *http.ServeMux {
	if provider == nil {
		provider = llm.Echo{}
	}
	tg := &channel.Telegram{Store: st, LLM: provider}
	zp := &channel.ZaloPersonal{Store: st, LLM: provider}
	zo := &channel.ZaloOA{Store: st, LLM: provider}
	mux := httpapi.RouterWithAllChannels(st, version, provider, tg.HandleUpdate, zp.HandleUpdate, zo.HandleUpdate).(*http.ServeMux)
	httpapi.RegisterWS(mux)
	return mux
}

// New wires store + LLM + channels + auth/ratelimit into a single handler.
// Domain types stay in gateway/internal; this package only assembles them.
func New(st store.StoreIface, version string) (http.Handler, Status) {
	provider := DefaultProvider()
	mux := Mux(st, version, provider)

	adminToken := os.Getenv("GOSO_ADMIN_TOKEN")
	rateLimit := 60
	if v := os.Getenv("GOSO_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rateLimit = n
		}
	}

	var handler http.Handler = mux
	if rateLimit > 0 {
		handler = ratelimit.New(rateLimit).Middleware(handler)
	}
	handler = auth.RequireToken(adminToken, []string{"/healthz"})(handler)

	return handler, Status{
		Provider:  provider.Name(),
		HasReal:   provider.Name() != "echo",
		Auth:      adminToken != "",
		RateLimit: rateLimit,
	}
}
