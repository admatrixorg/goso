// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/heartbeat"
	"github.com/mqglobal/goso/gateway/internal/httpapi"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/logstore"
	"github.com/mqglobal/goso/gateway/internal/observe"
	"github.com/mqglobal/goso/gateway/internal/ratelimit"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/team"
)

// fatalf is log.Fatalf; tests replace it so production refuse does not os.Exit.
var fatalf = log.Fatalf

// Status describes the assembled gateway handler (no secrets).
type Status struct {
	Provider  string
	HasReal   bool
	Auth      bool
	DevMode   bool
	RateLimit int
}

// DefaultProvider picks GOSO_LLM_PROVIDER if constructed, else router9 if
// constructed, else Anthropic, OpenAI, first named OpenAI-compat, else echo.
// GOSO_E2E_SCRIPTED=1 selects a test-only ToolChat provider (not for production).
func DefaultProvider() llm.Provider {
	if envTruthy(os.Getenv("GOSO_E2E_SCRIPTED")) && strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_ENV")), "test") {
		return llm.NewE2EScripted()
	}
	return llm.NewRegistry().Preferred()
}

func loadGatewayOverlay(st store.StoreIface) {
	if st == nil {
		return
	}
	row, err := st.GetGatewaySettings()
	if err != nil || row == nil {
		return
	}
	config.SetOverlay(row.Values)
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
		if tok, err := secrets.Get(st, connector.TokenSecretName(rec.Name)); err == nil {
			cfg.BearerToken = string(tok)
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
	mux, _ := muxWithPairing(st, version, provider, obs, meter, nil)
	return mux
}

func muxWithPairing(st store.StoreIface, version string, provider llm.Provider, obs *observe.Observer, meter *billing.Store, pairing *auth.Pairing) (*http.ServeMux, *auth.Pairing) {
	if provider == nil {
		provider = llm.Echo{}
	}
	if obs == nil {
		obs = observe.New()
	}
	if meter == nil {
		meter = billing.New()
	}
	if pairing == nil {
		pairing = auth.NewPairing()
	}
	tg := &channel.Telegram{Store: st, LLM: provider, Meter: meter}
	zp := &channel.ZaloPersonal{Store: st, LLM: provider, Meter: meter}
	zo := &channel.ZaloOA{Store: st, LLM: provider, Meter: meter}
	chMgr := channel.NewManager()
	chMgr.Telegram = tg
	dc := &channel.Discord{Store: st, LLM: provider, Meter: meter}
	sl := &channel.Slack{Store: st, LLM: provider, Meter: meter}
	fs := &channel.Feishu{Store: st, LLM: provider, Meter: meter}
	wa := &channel.WhatsApp{Store: st, LLM: provider, Meter: meter}

	connReg := connector.NewRegistry()
	loadConnectors(st, connReg)
	loadGatewayOverlay(st)
	gate := approval.New(0)
	ev := eventstore.New(1024)
	logs := logstore.New(1024)
	obs.SetLogs(logs)
	rt := agent.New(st, connReg, gate, ev, provider)
	rt.Observer = obs
	mux := httpapi.NewRouter(httpapi.Options{
		Store: st, Version: version, Provider: provider,
		Registry: connReg, Gate: gate, Events: ev, Logs: logs, Runtime: rt, Meter: meter,
		TG: tg.HandleUpdate, ZP: zp.HandleUpdate, ZO: zo.HandleUpdate,
		Discord: dc.HandleUpdate, Slack: sl.HandleUpdate, Feishu: fs.HandleUpdate, WhatsApp: wa.HandleUpdate,
		LLM: llm.NewRegistry(), Pairing: pairing, Channels: chMgr, APIKeys: apikey.Default(),
	}).(*http.ServeMux)
	httpapi.RegisterWS(mux, st, provider)
	obs.SetWsUp(true)
	obs.Register(mux)
	if !testing.Testing() {
		go chMgr.StartAll(context.Background())
		go cron.Loop(context.Background(), st, httpapi.FireSessionChat(rt, st, provider, meter))
		if heartbeat.Enabled() {
			go heartbeat.Loop(context.Background(), obs)
		}
		if team.AutoEnabled() {
			go team.Loop(context.Background(), st)
		}
	}
	return mux, pairing
}

// New wires store + LLM + channels + observe + connectors + billing + auth/ratelimit.
func New(st store.StoreIface, version string) (http.Handler, Status) {
	if err := security.CheckProduction(); err != nil {
		fatalf("%v", err)
	}
	provider := DefaultProvider()
	obs := observe.New()
	provider = obs.Wrap(provider)
	meter, _, err := billing.Open(os.Getenv("GOSO_DB_PATH"))
	if err != nil {
		log.Printf("billing: %v — using memory meter", err)
		meter = billing.New()
	}
	pairing := auth.NewPairing()
	mux, pairing := muxWithPairing(st, version, provider, obs, meter, pairing)

	adminToken := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN"))
	viewToken := strings.TrimSpace(os.Getenv("GOSO_VIEW_TOKEN"))
	rateLimit := 60
	if v := os.Getenv("GOSO_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rateLimit = n
		}
	}

	devMode := envTruthy(os.Getenv("GOSO_DEV_MODE"))
	var handler http.Handler = security.LimitAPI(mux)
	handler = httpapi.GuardDeactivatedTenant(nil, handler)
	if rateLimit > 0 {
		handler = ratelimit.New(rateLimit).Middleware(handler)
	}
	if adminToken != "" || viewToken != "" || !devMode {
		// Empty token + no explicit GOSO_DEV_MODE → 401 (SPEC 016).
		// Pairing exchange is an exact POST path inside Require (code is the secret).
		handler = auth.Require(auth.Config{
			Admin:   adminToken,
			View:    viewToken,
			Pairing: pairing,
			Keys:    apikey.Default(),
			Bypass:  []string{"/healthz", "/api/webhooks/llm"},
		})(handler)
	}
	handler = obs.Middleware(handler)

	return handler, Status{
		Provider:  provider.Name(),
		HasReal:   provider.Name() != "echo",
		Auth:      adminToken != "" || viewToken != "",
		DevMode:   devMode && adminToken == "" && viewToken == "",
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
