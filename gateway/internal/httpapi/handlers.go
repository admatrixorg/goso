// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
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
	// LLM is the env registry used for GET/test overlay (sqlite rows merge; env wins).
	LLM *llm.Registry
	// Pairing issues one-time view-token codes. Nil → NewPairing.
	Pairing *auth.Pairing
	// Channels is optional live health (SPEC 084).
	Channels *channel.Manager
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
		o.Webhooks = webhook.NewWithStore(o.Store)
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
	registerProviderRoutes(mux, opt)
	var chat http.HandlerFunc
	if opt.Runtime != nil {
		chat = handleChatRuntime(opt.Runtime, opt.Store, opt.Meter, opt.Provider)
	} else if opt.Provider != nil {
		chat = handleChatWithLLM(opt.Store, opt.Provider, opt.Meter)
	} else {
		chat = handleChat(opt.Store, opt.Meter)
	}
	aliasAPI(mux, "POST /api/chat", chat)
	mux.HandleFunc("GET /api/usage", handleUsage(opt.Meter))
	mux.HandleFunc("GET /api/quota", handleQuota(opt.Meter))
	registerConfigRoutes(mux, opt.Store)
	registerConnectorRoutes(mux, opt)
	registerCronRoutes(mux, opt)
	registerBackupRoutes(mux)
	registerPairingRoutes(mux, opt.Pairing)
	registerChannelPairingRoutes(mux, opt.Store)
	registerZaloPersonalQR(mux, opt.Store)
	return mux
}

func routerBase(st store.StoreIface, version string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version, "ws_up": WSMounted()})
	})
	aliasAPI(mux, "GET /api/tenant", handleTenant)

	// Agents
	mux.HandleFunc("POST /api/agents", handleCreateAgent(st))
	aliasAPI(mux, "GET /api/agents", handleListAgents(st))
	mux.HandleFunc("GET /api/agents/{id}", handleGetAgent(st))
	mux.HandleFunc("PATCH /api/agents/{id}", handlePatchAgent(st))
	mux.HandleFunc("DELETE /api/agents/{id}", handleDeleteAgent(st))

	// Sessions
	mux.HandleFunc("POST /api/sessions", handleCreateSession(st))
	aliasAPI(mux, "GET /api/sessions", handleListSessions(st))
	mux.HandleFunc("PATCH /api/sessions/{id}", handlePatchSession(st))
	mux.HandleFunc("DELETE /api/sessions/{id}", handleDeleteSession(st))
	mux.HandleFunc("POST /api/sessions/{id}/messages", handleAddMessage(st))
	mux.HandleFunc("GET /api/sessions/{id}/messages", handleListMessages(st))

	mux.HandleFunc("GET /api/memory/search", handleSearchMemory(st))
	aliasAPI(mux, "GET /api/memory/index", handleMemoryIndex(st))
	aliasAPI(mux, "GET /api/memory/{id}", handleGetMemory(st))
	aliasAPI(mux, "PATCH /api/memory/{id}", handlePatchMemory(st))
	aliasAPI(mux, "DELETE /api/memory/{id}", handleDeleteMemory(st))
	aliasAPI(mux, "GET /api/memory", handleListMemory(st))
	mux.HandleFunc("POST /api/memory", handleCreateMemory(st))

	aliasAPI(mux, "GET /api/kg/search", handleSearchKG(st))
	aliasAPI(mux, "GET /api/kg/entities/{id}", handleExpandKG(st))
	aliasAPI(mux, "POST /api/kg/entities", handleCreateKGEntity(st))
	aliasAPI(mux, "POST /api/kg/relations", handleCreateKGRelation(st))

	mux.HandleFunc("GET /api/vault/search", handleSearchVault(st))
	mux.HandleFunc("POST /api/vault/sync", handleSyncVault(st))
	mux.HandleFunc("GET /api/vault/health", handleVaultHealth(st))
	mux.HandleFunc("GET /api/vault/graph", handleVaultGraph(st))
	mux.HandleFunc("GET /api/vault/docs/{id}/links", handleVaultDocLinks(st))
	mux.HandleFunc("GET /api/vault/docs/{id}", handleGetVaultDoc(st))
	mux.HandleFunc("GET /api/vault/docs", handleListVaultDocs(st))
	mux.HandleFunc("PUT /api/vault/docs", handlePutVaultDoc(st))

	aliasAPI(mux, "GET /api/skills", handleListSkills())
	aliasAPI(mux, "POST /api/skills", handleCreateSkill())
	aliasAPI(mux, "DELETE /api/skills/{name}", handleDeleteSkill())

	registerTeamRoutes(mux, st)

	// WebSocket is registered separately via RegisterWS to keep gorilla dep isolated.
	return mux
}

func rejectUnknownProvider(w http.ResponseWriter, st store.StoreIface, name, model string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if _, err := llm.Resolve(st, name, model, nil); err != nil {
		writeErr(w, http.StatusBadRequest, "provider not found")
		return true
	}
	return false
}

func handleCreateAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentKey          string `json:"agent_key"`
			DisplayName       string `json:"display_name"`
			Model             string `json:"model"`
			LLMProvider       string `json:"llm_provider"`
			Instructions      string `json:"instructions"`
			OrchestrationMode string `json:"orchestration_mode"`
			Enabled           *bool  `json:"enabled"`
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
		body.LLMProvider = strings.TrimSpace(body.LLMProvider)
		if rejectUnknownProvider(w, st, body.LLMProvider, body.Model) {
			return
		}
		a, err := st.CreateAgent(store.Agent{
			TenantID:          requestTenant(r),
			AgentKey:          body.AgentKey,
			DisplayName:       body.DisplayName,
			Model:             body.Model,
			LLMProvider:       body.LLMProvider,
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
		if body.Enabled != nil && !*body.Enabled {
			upd := *a
			upd.Enabled = false
			upd.UpdatedAt = a.Stamp()
			disabled, err := st.UpdateAgent(upd)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			a = disabled
		}
		writeJSON(w, http.StatusCreated, a)
	}
}

func handleListAgents(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := agentsInTenant(st.ListAgents(), requestTenant(r))
		if list == nil {
			list = []*store.Agent{}
		}
		out := make([]*store.Agent, 0, len(list))
		for _, a := range list {
			out = append(out, agentListView(a))
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": out})
	}
}

func agentListView(a *store.Agent) *store.Agent {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Instructions = ""
	return &cp
}

func handleGetAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := agentVisible(st, id, requestTenant(r))
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
		cur, err := agentVisible(st, id, requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var body struct {
			OrchestrationMode *string `json:"orchestration_mode"`
			Model             *string `json:"model"`
			LLMProvider       *string `json:"llm_provider"`
			Instructions      *string `json:"instructions"`
			Enabled           *bool   `json:"enabled"`
			IfUpdatedAt       *string `json:"if_updated_at"`
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
			LLMProvider:       cur.LLMProvider,
			Enabled:           cur.Enabled,
		}
		if body.IfUpdatedAt != nil {
			want, err := parseIfUpdatedAt(*body.IfUpdatedAt)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			upd.UpdatedAt = want
		}
		if body.OrchestrationMode != nil {
			mode, err := parseOrchMode(*body.OrchestrationMode)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			if mode == "" {
				writeErr(w, http.StatusBadRequest, `unknown orchestration_mode ""`)
				return
			}
			upd.OrchestrationMode = mode
		}
		if body.Model != nil {
			upd.Model = *body.Model
		}
		if body.LLMProvider != nil {
			upd.LLMProvider = strings.TrimSpace(*body.LLMProvider)
			if rejectUnknownProvider(w, st, upd.LLMProvider, upd.Model) {
				return
			}
		}
		if body.Instructions != nil {
			upd.Instructions = *body.Instructions
		}
		if body.Enabled != nil {
			upd.Enabled = *body.Enabled
		}
		a, err := st.UpdateAgent(upd)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			if errors.Is(err, store.ErrConflict) {
				writeErr(w, http.StatusConflict, "agent was modified")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handleDeleteAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := agentVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		if err := st.DeleteAgent(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "agent not found")
				return
			}
			if errors.Is(err, store.ErrConflict) {
				writeErr(w, http.StatusConflict, "agent is team lead")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func parseIfUpdatedAt(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("invalid if_updated_at")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("invalid if_updated_at")
}

func rejectInactiveAgent(w http.ResponseWriter, st store.StoreIface, agentID string) bool {
	a, err := st.GetAgent(agentID)
	if err != nil || a == nil {
		return false
	}
	if a.Enabled {
		return false
	}
	writeErr(w, http.StatusConflict, "agent is inactive")
	return true
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
		tid := requestTenant(r)
		if _, err := agentVisible(st, body.AgentID, tid); err != nil {
			writeErr(w, http.StatusBadRequest, "agent not found")
			return
		}
		sess, err := st.CreateSession(store.Session{TenantID: tid, AgentID: body.AgentID, Label: body.Label})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	}
}

func handleListSessions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := sessionsInTenant(st.ListSessions(), requestTenant(r))
		if list == nil {
			list = []*store.Session{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
	}
}

func handlePatchSession(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		cur, err := sessionVisible(st, id, requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		var body struct {
			PromptMode *string `json:"prompt_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.PromptMode == nil {
			writeErr(w, http.StatusBadRequest, "prompt_mode is required")
			return
		}
		mode, err := pipeline.ParseMode(*body.PromptMode)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		upd := *cur
		upd.PromptMode = string(mode)
		sess, err := st.UpdateSession(upd)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sess)
	}
}

func handleDeleteSession(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := sessionVisible(st, id, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		if err := st.DeleteSession(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleAddMessage(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("id")
		if _, err := sessionVisible(st, sid, requestTenant(r)); err != nil {
			writeErr(w, http.StatusBadRequest, "session not found")
			return
		}
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
		if _, err := sessionVisible(st, sid, requestTenant(r)); err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
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
	Stream     bool    `json:"stream"`
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
		sess, err := sessionVisible(st, body.SessionID, requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		if rejectInactiveAgent(w, st, sess.AgentID) {
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
		respondChat(w, r, body, reply, map[string]any{"reply": reply, "session_id": body.SessionID}, nil, 0)
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
		sess, err := sessionVisible(st, body.SessionID, requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		if rejectInactiveAgent(w, st, sess.AgentID) {
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
		if chatWantsStream(r, body) {
			sw := newSSEWriter(w)
			reply, usage, err := llm.ChatStream(r.Context(), provider, msgs, sw.delta)
			if err != nil {
				sw.errEvent(err.Error())
				return
			}
			_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "assistant", Content: reply})
			recordUsage(meter, sess.AgentID, provider.Name(), usage)
			sw.data("[DONE]")
			return
		}
		reply, usage, err := llm.ChatUsage(r.Context(), provider, msgs)
		if err != nil {
			respondChat(w, r, body, "", nil, err, http.StatusBadGateway)
			return
		}
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "assistant", Content: reply})
		recordUsage(meter, sess.AgentID, provider.Name(), usage)
		respondChat(w, r, body, reply, map[string]any{"reply": reply, "session_id": body.SessionID}, nil, 0)
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
	aliasAPI(mux, "GET /api/channels", handleListChannels(opt.Store, opt.Channels))
	aliasAPI(mux, "GET /api/channels/{name}/health", handleChannelHealth(opt.Store, opt.Channels))
	aliasAPI(mux, "PATCH /api/channels/{name}", handlePatchChannel(opt.Store))
	aliasAPI(mux, "PUT /api/channels/{name}/secrets", handlePutChannelSecrets(opt.Store))
	aliasAPI(mux, "DELETE /api/channels/{name}/secrets", handleDeleteChannelSecrets(opt.Store))
	aliasAPI(mux, "POST /api/channels/{name}/test", handleTestChannel(opt.Store, opt.Channels))
}

func handleListChannels(st store.StoreIface, mgr *channel.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"channels": overlayChannelRows(st, channel.CatalogWith(st, mgr)),
			"lite":     store.LiteEnabled(),
		})
	}
}

func handleChannelHealth(st store.StoreIface, mgr *channel.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if !channel.Known(name) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		for _, row := range overlayChannelRows(st, channel.CatalogWith(st, mgr)) {
			if row.Name == name {
				writeJSON(w, http.StatusOK, row)
				return
			}
		}
		writeErr(w, http.StatusNotFound, "not found")
	}
}

func overlayChannelRows(st store.StoreIface, rows []channel.Info) []channel.Info {
	out := make([]channel.Info, len(rows))
	copy(out, rows)
	for i := range out {
		var cfg *store.ChannelConfig
		if st != nil {
			if got, err := st.GetChannelConfig(out[i].Name); err == nil && got != nil {
				cfg = got
				out[i].BoundAgentID = cfg.AgentID
				out[i].AllowFrom = append([]string(nil), cfg.AllowFrom...)
				out[i].AllowFromCount = len(cfg.AllowFrom)
				out[i].Enabled = cfg.Enabled
			}
		}
		pol := channel.MergePolicy(out[i].Name, cfg)
		out[i].DMPolicy = pol.DMPolicy
		out[i].GroupPolicy = pol.GroupPolicy
		out[i].RequireMention = pol.RequireMention
	}
	return out
}

func handlePatchChannel(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if !channel.Known(name) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		var body map[string]any
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil && err != io.EOF {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		for k := range body {
			lk := strings.ToLower(strings.TrimSpace(k))
			if channelSecretKey(lk) {
				writeErr(w, http.StatusBadRequest, "channel tokens are env-only")
				return
			}
		}
		cfg := store.ChannelConfig{Name: name, Enabled: true}
		if st != nil {
			if prev, err := st.GetChannelConfig(name); err == nil && prev != nil {
				cfg = *prev
				cfg.Name = name
			}
		}
		if v, ok := body["enabled"]; ok {
			b, ok := v.(bool)
			if !ok {
				writeErr(w, http.StatusBadRequest, "enabled must be bool")
				return
			}
			cfg.Enabled = b
		}
		if v, ok := body["dm_policy"]; ok {
			s, _ := v.(string)
			cfg.DMPolicy = strings.TrimSpace(s)
		}
		if v, ok := body["group_policy"]; ok {
			s, _ := v.(string)
			cfg.GroupPolicy = strings.TrimSpace(s)
		}
		if v, ok := body["require_mention"]; ok {
			b, ok := v.(bool)
			if !ok {
				writeErr(w, http.StatusBadRequest, "require_mention must be bool")
				return
			}
			cfg.RequireMention = b
		}
		if v, ok := body["allow_from"]; ok {
			arr, ok := v.([]any)
			if !ok {
				writeErr(w, http.StatusBadRequest, "allow_from must be array")
				return
			}
			ids := make([]string, 0, len(arr))
			for _, item := range arr {
				ids = append(ids, strings.TrimSpace(fmt.Sprint(item)))
			}
			cfg.AllowFrom = ids
		}
		if v, ok := body["agent_id"]; ok {
			s, _ := v.(string)
			s = strings.TrimSpace(s)
			if s != "" && st != nil {
				if _, err := st.GetAgent(s); err != nil {
					writeErr(w, http.StatusNotFound, "agent not found")
					return
				}
			}
			cfg.AgentID = s
		}
		if st != nil {
			if err := st.PutChannelConfig(cfg); err != nil {
				writeErr(w, http.StatusInternalServerError, "save failed")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
	}
}

func channelSecretKey(k string) bool {
	if k == "token" || k == "bot_token" || k == "app_token" || k == "user_token" || k == "access_token" ||
		k == "api_key" || k == "secret" || k == "app_secret" || k == "hmac" || k == "hmac_key" ||
		k == "password" || k == "credential" || k == "credential_ref" {
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "password")
}

// aliasAPI registers h at pattern and, when pattern is METHOD /api/..., the same h at METHOD /v1/...
func aliasAPI(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.HandleFunc(pattern, h)
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		return
	}
	rest, ok := strings.CutPrefix(path, "/api/")
	if !ok || rest == "" {
		return
	}
	mux.HandleFunc(method+" /v1/"+rest, h)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
