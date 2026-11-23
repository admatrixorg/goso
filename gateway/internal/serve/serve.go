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
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/builtin"
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
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/ratelimit"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/pkgmgr"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/team"
	"github.com/mqglobal/goso/gateway/internal/tenant"
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
	zcrm := &channel.ZCRMBridge{Store: st, LLM: provider, Meter: meter}
	// The message tool writes through this process's running channel bots,
	// the ZaloCRM bridge, and the send-only Discord, Slack and WhatsApp
	// channels of each tenant.
	builtin.MessageSender = &channel.MessageTool{Store: st, Manager: chMgr, Bridge: zcrm}

	connReg := connector.NewRegistry()
	loadConnectors(st, connReg)
	loadGatewayOverlay(st)
	tenants, err := tenant.Load(st)
	if err != nil {
		fatalf("tenant registry: %v", err)
	}
	if tenants != nil {
		tenant.SetDefault(tenants)
	} else {
		tenants = tenant.DefaultRegistry()
	}
	keys, err := apikey.Load(st)
	if err != nil {
		fatalf("api keys: %v", err)
	}
	if keys != nil {
		apikey.SetDefault(keys)
	}
	gate := approval.New(0)
	ev := eventstore.New(1024)
	logs := logstore.New(1024)
	obs.SetLogs(logs)
	rt := agent.New(st, connReg, gate, ev, provider)
	rt.Observer = obs
	channel.SetTurnRunner(ChannelTurnRunner(rt))
	mux := httpapi.NewRouter(httpapi.Options{
		Store: st, Version: version, Provider: provider,
		Registry: connReg, Gate: gate, Events: ev, Logs: logs, Runtime: rt, Meter: meter,
		TG: tg.HandleUpdate, ZP: zp.HandleUpdate, ZO: zo.HandleUpdate,
		Discord: dc.HandleUpdate, Slack: sl.HandleUpdate, Feishu: fs.HandleUpdate, WhatsApp: wa.HandleUpdate,
		ZCRM: zcrm.HandleUpdate,
		LLM:  llm.NewRegistry(), Pairing: pairing, Channels: chMgr, APIKeys: apikey.Default(),
		Tenants: tenants, TTS: wireMedia(st),
	}).(*http.ServeMux)
	httpapi.RegisterWS(mux, st, provider)
	rt.CronFire = httpapi.FireSessionChat(rt, st, provider, meter)
	obs.SetWsUp(true)
	observe.DBTraceSource = func(tenantID string, limit, offset int) ([]map[string]any, int) {
		x := store.Telemetry(st)
		if x == nil {
			return nil, 0
		}
		rows, total := x.ListTraces(tenantID, "", "", limit, offset)
		out := make([]map[string]any, 0, len(rows))
		for _, t := range rows {
			out = append(out, map[string]any{
				"id": t.ID, "agent_id": t.AgentID, "user_id": t.UserID, "session_key": t.SessionKey,
				"run_id": t.RunID, "start_time": t.StartTime, "end_time": t.EndTime,
				"duration_ms": t.DurationMS, "name": t.Name, "channel": t.Channel,
				"input_preview": t.InputPreview, "output_preview": t.OutputPreview,
				"total_input_tokens": t.TotalInputTokens, "total_output_tokens": t.TotalOutputTokens,
				"total_cost": t.TotalCost, "span_count": t.SpanCount, "llm_call_count": t.LLMCallCount,
				"tool_call_count": t.ToolCallCount, "status": t.Status, "error": t.Error,
				"team_id": t.TeamID, "parent_trace_id": t.ParentTraceID, "created_at": t.CreatedAt,
			})
		}
		return out, total
	}
	observe.DBTraceDetail = func(tenantID, id string) (map[string]any, []map[string]any, bool) {
		x := store.Telemetry(st)
		if x == nil {
			return nil, nil, false
		}
		tr, err := x.GetTrace(tenantID, id)
		if err != nil || tr == nil {
			return nil, nil, false
		}
		spans := make([]map[string]any, 0, 8)
		for _, sp := range x.ListSpans(tenantID, id) {
			spans = append(spans, map[string]any{
				"id": sp.ID, "trace_id": sp.TraceID, "parent_span_id": sp.ParentSpanID,
				"agent_id": sp.AgentID, "span_type": sp.SpanType, "name": sp.Name,
				"start_time": sp.StartTime, "end_time": sp.EndTime, "duration_ms": sp.DurationMS,
				"status": sp.Status, "error": sp.Error, "model": sp.Model, "provider": sp.Provider,
				"input_tokens": sp.InputTokens, "output_tokens": sp.OutputTokens,
				"total_cost": sp.TotalCost, "finish_reason": sp.FinishReason,
				"tool_name": sp.ToolName, "tool_call_id": sp.ToolCallID,
				"input_preview": sp.InputPreview, "output_preview": sp.OutputPreview,
			})
		}
		out := map[string]any{
			"id": tr.ID, "agent_id": tr.AgentID, "user_id": tr.UserID, "session_key": tr.SessionKey,
			"run_id": tr.RunID, "start_time": tr.StartTime, "end_time": tr.EndTime,
			"duration_ms": tr.DurationMS, "name": tr.Name, "channel": tr.Channel,
			"input_preview": tr.InputPreview, "output_preview": tr.OutputPreview,
			"total_input_tokens": tr.TotalInputTokens, "total_output_tokens": tr.TotalOutputTokens,
			"total_cost": tr.TotalCost, "span_count": tr.SpanCount, "llm_call_count": tr.LLMCallCount,
			"tool_call_count": tr.ToolCallCount, "status": tr.Status, "error": tr.Error,
			"created_at": tr.CreatedAt,
		}
		return out, spans, true
	}
	billing.OnRecord = func(rec billing.Record) {
		x := store.Usage(st)
		if x == nil {
			return
		}
		tenantID := "default"
		if st != nil {
			if ag, err := st.GetAgent(rec.AgentID); err == nil && ag != nil && ag.TenantID != "" {
				tenantID = ag.TenantID
			}
		}
		now := rec.CreatedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		stamp := now.Format(time.RFC3339)
		_ = x.InsertUsageEvent(store.UsageEvent{
			TenantID: tenantID, EventTime: stamp, BucketHour: stamp, EventType: "llm_call",
			ResourceType: "model", ResourceName: rec.Provider, AgentID: rec.AgentID,
			Provider: rec.Provider, Status: "ok",
			InputTokens: rec.PromptTokens, OutputTokens: rec.CompletionTokens,
			TotalTokens: rec.TotalTokens, CallCount: 1, CreatedAt: stamp,
		})
	}
	if pk := pkgmgr.Default(); pk != nil {
		pk.SetRunner(pkgmgr.DefaultRunner(3 * time.Minute))
	}
	obs.Register(mux)
	if !testing.Testing() {
		go chMgr.StartAll(context.Background())
		go cron.Loop(context.Background(), st, rt.CronFire)
		go heartbeatLoop(context.Background(), st, rt.CronFire)
		if heartbeat.Enabled() {
			go heartbeat.Loop(context.Background(), obs)
		}
		if team.AutoEnabled() {
			go team.Loop(context.Background(), st)
		}
		go pipeline.PruneLoop(context.Background(), st)
		go pipeline.IdleFlushLoop(context.Background(), st, provider)
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
	handler = httpapi.GuardDeactivatedTenant(tenant.DefaultRegistry(), handler)
	handler = httpapi.RejectInvalidTenant(handler)
	if rateLimit > 0 {
		handler = ratelimit.New(rateLimit).Middleware(handler)
	}
	handler = httpapi.RequireRole(handler)
	if adminToken != "" || viewToken != "" || !devMode {
		// Empty token + no explicit GOSO_DEV_MODE → 401 (SPEC 016).
		// Pairing exchange is an exact POST path inside Require (code is the secret).
		handler = auth.Require(auth.Config{
			Admin:    adminToken,
			View:     viewToken,
			Pairing:  pairing,
			Keys:     apikey.Default(),
			Sessions: sessionGate{st: st},
			Bypass:   []string{"/healthz", "/ws", "/api/webhooks/llm"},
			SelfAuth: channelSelfAuth,
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

// channelSelfAuth lets a channel's own signed webhook call past the admin
// token; its handler checks the signature.
func channelSelfAuth(r *http.Request) bool {
	return channel.TelegramSelfAuth(r) || channel.ZCRMSelfAuth(r) || channel.ZaloBotSelfAuth(r)
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ChannelTurnRunner runs channel turns through the agent runtime, so an agent
// answers on Telegram, Zalo or Lark with the same tools, context files, hooks
// and approvals as in the web chat.
func ChannelTurnRunner(rt *agent.Runtime) channel.TurnRunner {
	return func(ctx context.Context, sessionID, text string) (channel.TurnResult, error) {
		out, err := rt.ChatWithTurn(ctx, sessionID, text, "", false, agent.ChatTurn{Channel: true}, nil)
		if err != nil {
			return channel.TurnResult{}, err
		}
		res := channel.TurnResult{}
		if out == nil {
			return res, nil
		}
		res.Reply = out.Reply
		res.AgentID = out.AgentID
		res.Provider = out.Provider
		res.Usage = out.Usage
		for _, tr := range out.Trace {
			if tr.Status == "pending_approval" {
				res.Pending = true
				break
			}
		}
		return res, nil
	}
}

type sessionGate struct{ st store.StoreIface }

func (g sessionGate) Accept(token string) (auth.Identity, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return auth.Identity{}, false
	}
	sessions := store.AuthSessions(g.st)
	if sessions == nil {
		return auth.Identity{}, false
	}
	sess, err := sessions.LookupAuthSession(token)
	if err != nil || sess == nil {
		return auth.Identity{}, false
	}
	return auth.Identity{
		UserID: sess.UserID, Email: sess.Email, TenantID: sess.TenantID,
		Role: sess.Role, Owner: strings.EqualFold(sess.Role, "owner"),
	}, true
}
