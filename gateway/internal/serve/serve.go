// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/httpapi"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/observe"
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

func loadConnectors(st store.StoreIface, connReg *connector.Registry) {
	if st == nil {
		return
	}
	for _, rec := range st.ListConnectors() {
		if rec == nil {
			continue
		}
		cfg := connector.Config{
			Name: rec.Name, Transport: rec.Transport, Endpoint: rec.Endpoint,
			CredentialRef: rec.CredentialRef, SchemaVersion: rec.SchemaVersion,
			ManifestURL: rec.ManifestURL, ManifestJSON: rec.ManifestJSON,
			TimeoutMS: rec.TimeoutMS, Retries: rec.Retries,
		}
		c, err := connector.Build(cfg)
		if err != nil {
			log.Printf("connector %s: %v", rec.Name, err)
			continue
		}
		_ = connReg.Replace(c)
		if !rec.Enabled {
			_ = connReg.SetEnabled(rec.Name, false)
		}
	}
}

// Mux builds the gateway ServeMux (API + channels + WS + observe + connectors).
func Mux(st store.StoreIface, version string, provider llm.Provider, obs *observe.Observer) *http.ServeMux {
	if provider == nil {
		provider = llm.Echo{}
	}
	if obs == nil {
		obs = observe.New()
	}
	tg := &channel.Telegram{Store: st, LLM: provider}
	zp := &channel.ZaloPersonal{Store: st, LLM: provider}
	zo := &channel.ZaloOA{Store: st, LLM: provider}

	connReg := connector.NewRegistry()
	loadConnectors(st, connReg)
	gate := approval.New(0)
	ev := eventstore.New(1024)
	rt := agent.New(st, connReg, gate, ev, provider)
	mux := httpapi.NewRouter(httpapi.Options{
		Store: st, Version: version, Provider: provider,
		Registry: connReg, Gate: gate, Events: ev, Runtime: rt,
		TG: tg.HandleUpdate, ZP: zp.HandleUpdate, ZO: zo.HandleUpdate,
	}).(*http.ServeMux)
	httpapi.RegisterWS(mux)
	obs.Register(mux)
	return mux
}

// New wires store + LLM + channels + observe + connectors + auth/ratelimit.
func New(st store.StoreIface, version string) (http.Handler, Status) {
	provider := DefaultProvider()
	obs := observe.New()
	provider = obs.Wrap(provider)
	mux := Mux(st, version, provider, obs)

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
	handler = obs.Middleware(handler)

	return handler, Status{
		Provider:  provider.Name(),
		HasReal:   provider.Name() != "echo",
		Auth:      adminToken != "",
		RateLimit: rateLimit,
	}
}
