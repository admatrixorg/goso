// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/billing"
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
	DevMode   bool
	RateLimit int
}

// DefaultProvider picks Anthropic, else OpenAI, else echo — same rules as the CLI.
// GOSO_E2E_SCRIPTED=1 selects a test-only ToolChat provider (not for production).
func DefaultProvider() llm.Provider {
	if envTruthy(os.Getenv("GOSO_E2E_SCRIPTED")) && strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_ENV")), "test") {
		return llm.NewE2EScripted()
	}
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

// Mux builds the gateway ServeMux (API + channels + WS + observe + connectors + billing).
func Mux(st store.StoreIface, version string, provider llm.Provider, obs *observe.Observer, meter *billing.Store) *http.ServeMux {
	if provider == nil {
		provider = llm.Echo{}
	}
	if obs == nil {
		obs = observe.New()
	}
	if meter == nil {
		meter = billing.New()
	}
	tg := &channel.Telegram{Store: st, LLM: provider, Meter: meter}
	zp := &channel.ZaloPersonal{Store: st, LLM: provider, Meter: meter}
	zo := &channel.ZaloOA{Store: st, LLM: provider, Meter: meter}

	connReg := connector.NewRegistry()
	loadConnectors(st, connReg)
	gate := approval.New(0)
	ev := eventstore.New(1024)
	rt := agent.New(st, connReg, gate, ev, provider)
	mux := httpapi.NewRouter(httpapi.Options{
		Store: st, Version: version, Provider: provider,
		Registry: connReg, Gate: gate, Events: ev, Runtime: rt, Meter: meter,
		TG: tg.HandleUpdate, ZP: zp.HandleUpdate, ZO: zo.HandleUpdate,
	}).(*http.ServeMux)
	httpapi.RegisterWS(mux)
	obs.Register(mux)
	return mux
}

// New wires store + LLM + channels + observe + connectors + billing + auth/ratelimit.
func New(st store.StoreIface, version string) (http.Handler, Status) {
	provider := DefaultProvider()
	obs := observe.New()
	provider = obs.Wrap(provider)
	meter, _, err := billing.Open(os.Getenv("GOSO_DB_PATH"))
	if err != nil {
		log.Printf("billing: %v — using memory meter", err)
		meter = billing.New()
	}
	mux := Mux(st, version, provider, obs, meter)

	adminToken := os.Getenv("GOSO_ADMIN_TOKEN")
	rateLimit := 60
	if v := os.Getenv("GOSO_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rateLimit = n
		}
	}

	devMode := envTruthy(os.Getenv("GOSO_DEV_MODE"))
	var handler http.Handler = mux
	if rateLimit > 0 {
		handler = ratelimit.New(rateLimit).Middleware(handler)
	}
	if adminToken != "" || !devMode {
		// Empty token + no explicit GOSO_DEV_MODE → 401 (SPEC 016).
		handler = auth.RequireToken(adminToken, []string{"/healthz"})(handler)
	}
	handler = obs.Middleware(handler)

	return handler, Status{
		Provider:  provider.Name(),
		HasReal:   provider.Name() != "echo",
		Auth:      adminToken != "",
		DevMode:   devMode && adminToken == "",
		RateLimit: rateLimit,
	}
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
