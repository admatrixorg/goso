// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

// Options wires SPEC 014 deps into the HTTP mux. All fields except Store are optional.
type Options struct {
	Store    store.StoreIface
	Version  string
	Provider llm.Provider
	Registry *connector.Registry
	Gate     *approval.Gate
	Events   *eventstore.Store
	Runtime  *agent.Runtime
	Meter    *billing.Store
	TG       http.HandlerFunc
	ZP       http.HandlerFunc
	ZO       http.HandlerFunc
	Discord  http.HandlerFunc
	Slack    http.HandlerFunc
	Feishu   http.HandlerFunc
	WhatsApp http.HandlerFunc
	Webhooks *webhook.Registry
	// ProviderNames is GET /api/providers (configured names only, never secrets).
	ProviderNames []string
}

func (o *Options) defaults() {
	if o.Registry == nil {
		o.Registry = connector.NewRegistry()
	}
	if o.Gate == nil {
		o.Gate = approval.New(0)
	}
	if o.Events == nil {
		o.Events = eventstore.New(256)
	}
	if o.Runtime == nil {
		o.Runtime = agent.New(o.Store, o.Registry, o.Gate, o.Events, o.Provider)
	}
	if o.Meter == nil {
		o.Meter = billing.New()
	}
	if o.Webhooks == nil {
		o.Webhooks = webhook.New()
	}
}

// Router builds the HTTP mux.
func Router(st store.StoreIface, version string) http.Handler {
	return NewRouter(Options{Store: st, Version: version})
}

// NewRouter builds the mux with optional connector/approval/event/billing deps.
func NewRouter(opt Options) http.Handler {
	opt.defaults()
	mux := routerBase(opt.Store, opt.Version)
	registerChannels(mux, opt)
	registerWebhookRoutes(mux, opt)
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		names := opt.ProviderNames
		if len(names) == 0 {
			name := "echo"
			if opt.Provider != nil {
				name = opt.Provider.Name()
			}
			names = []string{name}
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": names})
	})
	if opt.Runtime != nil {
		mux.HandleFunc("POST /api/chat", handleChatRuntime(opt.Runtime, opt.Store, opt.Meter, opt.Provider))
	} else if opt.Provider != nil {
		mux.HandleFunc("POST /api/chat", handleChatWithLLM(opt.Store, opt.Provider, opt.Meter))
	} else {
		mux.HandleFunc("POST /api/chat", handleChat(opt.Store, opt.Meter))
	}
	mux.HandleFunc("GET /api/usage", handleUsage(opt.Meter))
	mux.HandleFunc("GET /api/quota", handleQuota(opt.Meter))
	registerConnectorRoutes(mux, opt)
	return mux
}

func routerBase(st store.StoreIface, version string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
	})

	// Agents
	mux.HandleFunc("POST /api/agents", handleCreateAgent(st))
	mux.HandleFunc("GET /api/agents", handleListAgents(st))
	mux.HandleFunc("GET /api/agents/{id}", handleGetAgent(st))
	mux.HandleFunc("PATCH /api/agents/{id}", handlePatchAgent(st))

	// Sessions
	mux.HandleFunc("POST /api/sessions", handleCreateSession(st))
	mux.HandleFunc("GET /api/sessions", handleListSessions(st))
	mux.HandleFunc("POST /api/sessions/{id}/messages", handleAddMessage(st))
	mux.HandleFunc("GET /api/sessions/{id}/messages", handleListMessages(st))

	mux.HandleFunc("GET /api/memory/search", handleSearchMemory(st))
	mux.HandleFunc("GET /api/memory", handleListMemory(st))
	mux.HandleFunc("POST /api/memory", handleCreateMemory(st))

	mux.HandleFunc("GET /api/vault/search", handleSearchVault(st))
	mux.HandleFunc("POST /api/vault/sync", handleSyncVault(st))
	mux.HandleFunc("GET /api/vault/docs/{id}/links", handleVaultDocLinks(st))
	mux.HandleFunc("GET /api/vault/docs/{id}", handleGetVaultDoc(st))
	mux.HandleFunc("GET /api/vault/docs", handleListVaultDocs(st))
	mux.HandleFunc("PUT /api/vault/docs", handlePutVaultDoc(st))

	registerTeamRoutes(mux, st)

	// WebSocket is registered separately via RegisterWS to keep gorilla dep isolated.
	return mux
}

func handleCreateAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentKey          string `json:"agent_key"`
			DisplayName       string `json:"display_name"`
			Model             string `json:"model"`
			Instructions      string `json:"instructions"`
			OrchestrationMode string `json:"orchestration_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.AgentKey = strings.TrimSpace(body.AgentKey)
		if body.AgentKey == "" {
			writeErr(w, http.StatusBadRequest, "agent_key is required")
			return
		}
		mode, err := parseOrchMode(body.OrchestrationMode)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		a, err := st.CreateAgent(store.Agent{
			AgentKey:          body.AgentKey,
			DisplayName:       body.DisplayName,
			Model:             body.Model,
			Instructions:      body.Instructions,
			OrchestrationMode: mode,
		})
		if err != nil {
			if errors.Is(err, store.ErrExists) {
				writeErr(w, http.StatusConflict, "agent_key already exists")
				return
			}
			if errors.Is(err, store.ErrLiteCap) {
				writeErr(w, http.StatusBadRequest, "lite cap: max 5 agents")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, a)
	}
}

func handleListAgents(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := st.ListAgents()
		if list == nil {
			list = []*store.Agent{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": list})
	}
}

func handleGetAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := st.GetAgent(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handlePatchAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		cur, err := st.GetAgent(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var body struct {
			OrchestrationMode *string `json:"orchestration_mode"`
			Model             *string `json:"model"`
			Instructions      *string `json:"instructions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		upd := store.Agent{
			ID:                cur.ID,
			Instructions:      cur.Instructions,
			OrchestrationMode: cur.OrchestrationMode,
			Model:             cur.Model,
		}
		if body.OrchestrationMode != nil {
			mode, err := parseOrchMode(*body.OrchestrationMode)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			upd.OrchestrationMode = mode
		}
		if body.Model != nil {
			upd.Model = *body.Model
		}
		if body.Instructions != nil {
			upd.Instructions = *body.Instructions
		}
		a, err := st.UpdateAgent(upd)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handleCreateSession(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentID string `json:"agent_id"`
			Label   string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.AgentID = strings.TrimSpace(body.AgentID)
		if body.AgentID == "" {
			writeErr(w, http.StatusBadRequest, "agent_id is required")
			return
		}
		sess, err := st.CreateSession(store.Session{AgentID: body.AgentID, Label: body.Label})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	}
}

func handleListSessions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := st.ListSessions()
		if list == nil {
			list = []*store.Session{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
	}
}

func handleAddMessage(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("id")
		var body struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(body.Content) == "" {
			writeErr(w, http.StatusBadRequest, "content is required")
			return
		}
		if body.Role == "" {
			body.Role = "user"
		}
		m, err := st.AddMessage(store.Message{SessionID: sid, Role: body.Role, Content: body.Content})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, m)
	}
}

func handleListMessages(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("id")
		msgs, err := st.ListMessages(sid)
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	}
}

type boolish bool

func (b *boolish) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	switch strings.ToLower(s) {
	case "true", "1", `"true"`, `"1"`:
		*b = true
	case "false", "0", `"false"`, `"0"`, "null", "":
		*b = false
	default:
		return errors.New("invalid summarize flag")
	}
	return nil
}

type chatBody struct {
	SessionID  string  `json:"session_id"`
	Message    string  `json:"message"`
	PromptMode string  `json:"prompt_mode"`
	Summarize  boolish `json:"summarize"`
}

func decodeChatBody(r *http.Request) (chatBody, error) {
	var body chatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return body, err
	}
	if strings.TrimSpace(body.SessionID) == "" || strings.TrimSpace(body.Message) == "" {
		return body, errChatRequired
	}
	if _, err := pipeline.ParseMode(body.PromptMode); err != nil {
		return body, err
	}
	q := strings.TrimSpace(r.URL.Query().Get("summarize"))
	if q == "1" || strings.EqualFold(q, "true") {
		body.Summarize = true
	}
	return body, nil
}

var errChatRequired = errors.New("session_id and message are required")

func rejectInjectedChat(w http.ResponseWriter, message string) bool {
	matched, block := security.InspectChat(message)
	if matched == "" {
		return false
	}
	log.Printf("goso injection: matched %q mode=%s", matched, security.InjectionMode())
	if block {
		writeErr(w, http.StatusBadRequest, "injection blocked")
		return true
	}
	return false
}

func handleChat(st store.StoreIface, meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeChatBody(r)
		if err != nil {
			if errors.Is(err, errChatRequired) {
				writeErr(w, http.StatusBadRequest, errChatRequired.Error())
				return
			}
			msg := err.Error()
			if strings.Contains(msg, "unknown prompt_mode") {
				writeErr(w, http.StatusBadRequest, msg)
				return
			}
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if rejectInjectedChat(w, body.Message) {
			return
		}
		sess, err := st.GetSession(body.SessionID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		if rejectIfQuotaExceeded(w, meter) {
			return
		}
		// Persist user message and echo reply (stub for LLM in SPEC 003).
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "user", Content: body.Message})
		reply := "echo: " + body.Message
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "assistant", Content: reply})
		recordUsage(meter, sess.AgentID, "echo", llm.EstimateUsage([]llm.Message{{Role: "user", Content: body.Message}}, reply))
		writeJSON(w, http.StatusOK, map[string]any{"reply": reply, "session_id": body.SessionID})
	}
}

func handleChatWithLLM(st store.StoreIface, provider llm.Provider, meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeChatBody(r)
		if err != nil {
			if errors.Is(err, errChatRequired) {
				writeErr(w, http.StatusBadRequest, errChatRequired.Error())
				return
			}
			msg := err.Error()
			if strings.Contains(msg, "unknown prompt_mode") {
				writeErr(w, http.StatusBadRequest, msg)
				return
			}
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if rejectInjectedChat(w, body.Message) {
			return
		}
		sess, err := st.GetSession(body.SessionID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		if rejectIfQuotaExceeded(w, meter) {
			return
		}
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "user", Content: body.Message})
		history, _ := st.ListMessages(body.SessionID)
		var msgs []llm.Message
		for _, m := range history {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
		reply, usage, err := llm.ChatUsage(r.Context(), provider, msgs)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "assistant", Content: reply})
		recordUsage(meter, sess.AgentID, provider.Name(), usage)
		writeJSON(w, http.StatusOK, map[string]any{"reply": reply, "session_id": body.SessionID})
	}
}

func recordUsage(meter *billing.Store, agentID, provider string, u llm.Usage) {
	if meter == nil {
		return
	}
	meter.AddCall(agentID, provider, u.PromptTokens, u.CompletionTokens, u.Estimated)
}

func handleUsage(meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := billing.Query{
			AgentID:  strings.TrimSpace(r.URL.Query().Get("agent_id")),
			Provider: strings.TrimSpace(r.URL.Query().Get("provider")),
		}
		fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
		toStr := strings.TrimSpace(r.URL.Query().Get("to"))
		if fromStr != "" {
			t, err := time.ParseInLocation("2006-01-02", fromStr, time.UTC)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid from date")
				return
			}
			q.From = t
		}
		if toStr != "" {
			t, err := time.ParseInLocation("2006-01-02", toStr, time.UTC)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid to date")
				return
			}
			q.To = t.Add(24 * time.Hour) // exclusive end of day
		}
		writeJSON(w, http.StatusOK, meter.Query(q))
	}
}

func handleQuota(meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, meter.QuotaStatus(time.Now().UTC()))
	}
}

// rejectIfQuotaExceeded writes HTTP 429 {error:quota_exceeded} when the daily
// cap is already reached. Check runs before recording a new chat.
func rejectIfQuotaExceeded(w http.ResponseWriter, meter *billing.Store) bool {
	limit := billing.DayLimit()
	if limit <= 0 {
		return false
	}
	today := meter.TodayTotals(time.Now().UTC())
	if !billing.Exceeded(today, limit) {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(billing.SecondsUntilUTCMidnight(time.Now().UTC())))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "quota_exceeded"})
	return true
}

// RouterWithDeps builds mux with LLM and channel deps (used in main).
func RouterWithDeps(st store.StoreIface, version string, provider llm.Provider, tgHandler http.HandlerFunc) http.Handler {
	// Extended by SPEC 004: zalo handlers are registered via RouterWithAllChannels when available.
	return RouterWithAllChannels(st, version, provider, tgHandler, nil, nil)
}

func RouterWithAllChannels(st store.StoreIface, version string, provider llm.Provider, tgHandler, zpHandler, zoHandler http.HandlerFunc) http.Handler {
	return RouterWithBilling(st, version, provider, tgHandler, zpHandler, zoHandler, billing.New())
}

// RouterWithBilling is RouterWithAllChannels plus a usage meter (SPEC 010).
func RouterWithBilling(st store.StoreIface, version string, provider llm.Provider, tgHandler, zpHandler, zoHandler http.HandlerFunc, meter *billing.Store) http.Handler {
	return NewRouter(Options{
		Store: st, Version: version, Provider: provider, Meter: meter,
		TG: tgHandler, ZP: zpHandler, ZO: zoHandler,
	})
}

func registerChannels(mux *http.ServeMux, opt Options) {
	type route struct {
		path string
		h    http.HandlerFunc
	}
	for _, r := range []route{
		{"POST /api/channels/telegram/webhook", opt.TG},
		{"POST /api/channels/zalo-personal/webhook", opt.ZP},
		{"POST /api/channels/zalo-oa/webhook", opt.ZO},
		{"POST /api/channels/discord/webhook", opt.Discord},
		{"POST /api/channels/slack/webhook", opt.Slack},
		{"POST /api/channels/feishu/webhook", opt.Feishu},
		{"POST /api/channels/whatsapp/webhook", opt.WhatsApp},
	} {
		if r.h != nil {
			mux.HandleFunc(r.path, r.h)
		}
	}
	mux.HandleFunc("GET /api/channels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"channels": channel.Catalog()})
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
