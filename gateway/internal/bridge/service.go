// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Service is the Omni-Core bridge: signed config sync and signed replies.
type Service struct {
	Secret   string
	Version  string
	Catalog  Catalog
	Store    store.StoreIface
	Complete Completer
	Now      func() time.Time
	Skew     time.Duration
	MaxBody  int64
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) skew() time.Duration {
	if s != nil && s.Skew > 0 {
		return s.Skew
	}
	return defaultSkew
}

func (s *Service) maxBody() int64 {
	if s != nil && s.MaxBody > 0 {
		return s.MaxBody
	}
	return 10 << 20
}

func (s *Service) catalog() Catalog {
	if s != nil && s.Catalog != nil {
		return s.Catalog
	}
	return newMem()
}

func (s *Service) version() string {
	if s != nil && s.Version != "" {
		return s.Version
	}
	return "0.1.0"
}

// OpenCatalog attaches the bridge tables to a SQL store, or returns an
// in-memory catalog when the store has no database.
func OpenCatalog(st store.StoreIface) (Catalog, error) {
	type sqler interface {
		SQL() (*sql.DB, bool)
	}
	if s, ok := st.(sqler); ok {
		db, pg := s.SQL()
		if db != nil {
			return newSQLCatalog(db, pg)
		}
	}
	return newMem(), nil
}

// ReplyInput is the union of the HTTP JSON body and the gRPC request.
type ReplyInput struct {
	TenantID         string
	AgentID          string
	SessionID        string
	ChannelAccountID string
	ContactID        string
	Prompt           string
	Model            string
	SystemPrompt     string
	Temperature      float64
	TemperatureSet   bool
	History          []llm.Message
	Metadata         map[string]string
}

// ReplyOutput is the unary reply. Attachment fields stay empty.
type ReplyOutput struct {
	ReplyText        string
	PromptTokens     int64
	CompletionTokens int64
	LatencyMS        int64
	StopReason       string
	DelayMS          int64
}

// Reply resolves the tenant agent and calls that tenant's provider.
// onDelta, when set, receives streaming text chunks.
func (s *Service) Reply(ctx context.Context, in ReplyInput, onDelta func(string)) (ReplyOutput, error) {
	start := s.now()
	if strings.TrimSpace(in.TenantID) == "" {
		return ReplyOutput{}, unauthorized("missing x-goclaw-tenant")
	}
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = meta(in.Metadata, "agent_id", "agentId")
	}
	if agentID == "" {
		// omni-core's gRPC client puts the agent id in session_id.
		agentID = strings.TrimSpace(in.SessionID)
	}
	if agentID == "" {
		return ReplyOutput{}, badRequest("agent id is required")
	}
	in.AgentID = agentID

	cat := s.catalog()
	ten, err := cat.EnsureTenant(in.TenantID)
	if err != nil {
		return ReplyOutput{}, err
	}
	ag, err := cat.GetAgent(in.TenantID, agentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReplyOutput{}, notFound("agent not configured")
		}
		return ReplyOutput{}, err
	}
	if !agentActive(ag.Status) {
		return ReplyOutput{}, errStatus(409, "agent is not active")
	}
	prov, err := s.resolveProvider(in.TenantID, providerID(ag, in.Metadata), firstNonEmpty(in.Model, ag.ModelName))
	if err != nil {
		return ReplyOutput{}, err
	}

	model := firstNonEmpty(in.Model, ag.ModelName)
	if model == "" && len(prov.Models) > 0 {
		model = prov.Models[0]
	}
	system := firstNonEmpty(in.SystemPrompt, ag.SystemPrompt)
	if docs, err := cat.ListKnowledge(in.TenantID, agentID); err != nil {
		return ReplyOutput{}, err
	} else if extra := knowledgePrompt(docs); extra != "" {
		if system != "" {
			system += "\n\n" + extra
		} else {
			system = extra
		}
	}
	temp, maxTok := sampling(in, ag)

	if err := s.ensureSession(ten.GosoID, &ag, model, system, in); err != nil {
		return ReplyOutput{}, err
	}

	msgs := make([]llm.Message, 0, len(in.History)+2)
	if system != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: system})
	}
	msgs = append(msgs, in.History...)
	if strings.TrimSpace(in.Prompt) != "" {
		msgs = append(msgs, llm.Message{Role: "user", Content: in.Prompt})
	}
	if len(msgs) == 0 {
		return ReplyOutput{}, badRequest("prompt is required")
	}

	fn := s.Complete
	if fn == nil {
		fn = DefaultComplete
	}
	res, err := fn(ctx, Completion{
		Type:        prov.Type,
		BaseURL:     prov.BaseURL,
		APIKey:      prov.APIKey,
		Model:       model,
		Temperature: temp,
		MaxTokens:   maxTok,
		Messages:    msgs,
		OnDelta:     onDelta,
	})
	if err != nil {
		err = classifyTransport(err, prov.APIKey)
		logReplyFailure(in, prov.Type, model, s.now().Sub(start), err)
		return ReplyOutput{}, err
	}
	if strings.TrimSpace(res.Text) == "" {
		err = &Error{Status: 502, Code: "empty_reply", Msg: "upstream returned empty text"}
		logReplyFailure(in, prov.Type, model, s.now().Sub(start), err)
		return ReplyOutput{}, err
	}
	s.appendTurn(ag.GosoAgentID, in, res.Text)
	stop := res.StopReason
	if stop == "" {
		stop = "stop"
	}
	return ReplyOutput{
		ReplyText:        res.Text,
		PromptTokens:     int64(res.PromptTokens),
		CompletionTokens: int64(res.CompletionTokens),
		LatencyMS:        s.now().Sub(start).Milliseconds(),
		StopReason:       stop,
	}, nil
}

func (s *Service) resolveProvider(tenantID, providerID, model string) (Provider, error) {
	cat := s.catalog()
	if providerID != "" {
		p, err := cat.GetProvider(tenantID, providerID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return s.providerByName(tenantID, providerID, model)
			}
			return Provider{}, err
		}
		if !p.Active {
			return Provider{}, notFound("provider not configured")
		}
		return p, nil
	}
	list, err := cat.ListProviders(tenantID)
	if err != nil {
		return Provider{}, err
	}
	var active []Provider
	for _, p := range list {
		if p.Active {
			active = append(active, p)
		}
	}
	if model != "" {
		var match []Provider
		for _, p := range active {
			if providerServesModel(p, model) {
				match = append(match, p)
			}
		}
		switch len(match) {
		case 1:
			return match[0], nil
		default:
			if len(match) > 1 {
				return Provider{}, notFound("provider not configured")
			}
		}
	}
	if len(active) == 1 {
		return active[0], nil
	}
	if p, ok := envDefaultProvider(); ok {
		return p, nil
	}
	return Provider{}, notFound("provider not configured")
}

func providerServesModel(p Provider, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" || !p.Active {
		return false
	}
	for _, m := range p.Models {
		if strings.TrimSpace(m) == model {
			return true
		}
	}
	return false
}

func (s *Service) providerByName(tenantID, name, model string) (Provider, error) {
	list, err := s.catalog().ListProviders(tenantID)
	if err != nil {
		return Provider{}, err
	}
	for _, p := range list {
		if p.Active && p.Name == name {
			return p, nil
		}
	}
	return Provider{}, notFound("provider not configured")
}

func (s *Service) ensureSession(gosoTenant string, ag *Agent, model, system string, in ReplyInput) error {
	if s == nil || s.Store == nil || ag == nil {
		return nil
	}
	st := s.Store
	ag.GosoTenant = gosoTenant
	id, err := s.ensureGosoAgent(gosoTenant, ag, model, system)
	if err != nil {
		return err
	}
	ag.GosoAgentID = id
	_ = s.catalog().BindGosoAgent(ag.TenantID, ag.ID, id)

	label := sessionLabel(in, ag.ID)
	for _, sess := range st.ListSessions() {
		if sess != nil && sess.AgentID == id && sess.Label == label {
			return nil
		}
	}
	if _, err := st.CreateSession(store.Session{
		TenantID: gosoTenant,
		AgentID:  id,
		Label:    label,
	}); err != nil {
		return unavailable("cannot open session")
	}
	return nil
}

func (s *Service) ensureGosoAgent(gosoTenant string, ag *Agent, model, system string) (string, error) {
	st := s.Store
	key := "omni:" + ag.TenantID + ":" + ag.ID
	if ag.GosoAgentID != "" {
		if cur, err := st.GetAgent(ag.GosoAgentID); err == nil && cur != nil {
			cur.Instructions = system
			cur.Model = model
			cur.LLMProvider = ag.ProviderID
			cur.Enabled = agentActive(ag.Status)
			if _, err := st.UpdateAgent(*cur); err != nil {
				return "", unavailable("cannot update agent")
			}
			return cur.ID, nil
		}
	}
	for _, cur := range st.ListAgents() {
		if cur != nil && cur.AgentKey == key {
			cur.Instructions = system
			cur.Model = model
			cur.LLMProvider = ag.ProviderID
			cur.Enabled = agentActive(ag.Status)
			if _, err := st.UpdateAgent(*cur); err != nil {
				return "", unavailable("cannot update agent")
			}
			return cur.ID, nil
		}
	}
	created, err := st.CreateAgent(store.Agent{
		TenantID:     gosoTenant,
		AgentKey:     key,
		DisplayName:  ag.Name,
		Model:        model,
		LLMProvider:  ag.ProviderID,
		Instructions: system,
	})
	if err != nil {
		if errors.Is(err, store.ErrExists) {
			for _, cur := range st.ListAgents() {
				if cur != nil && cur.AgentKey == key {
					return cur.ID, nil
				}
			}
		}
		return "", unavailable("cannot create agent")
	}
	if !agentActive(ag.Status) {
		created.Enabled = false
		if _, err := st.UpdateAgent(*created); err != nil {
			return "", unavailable("cannot update agent")
		}
	}
	return created.ID, nil
}

func (s *Service) appendTurn(gosoAgentID string, in ReplyInput, reply string) {
	if s == nil || s.Store == nil || gosoAgentID == "" {
		return
	}
	label := sessionLabel(in, in.AgentID)
	var sid string
	for _, sess := range s.Store.ListSessions() {
		if sess != nil && sess.AgentID == gosoAgentID && sess.Label == label {
			sid = sess.ID
			break
		}
	}
	if sid == "" {
		return
	}
	if strings.TrimSpace(in.Prompt) != "" {
		_, _ = s.Store.AddMessage(store.Message{SessionID: sid, Role: "user", Content: in.Prompt})
	}
	if reply != "" {
		_, _ = s.Store.AddMessage(store.Message{SessionID: sid, Role: "assistant", Content: reply})
	}
}

func knowledgePrompt(docs []KnowledgeDoc) string {
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Knowledge")
	const capBytes = 32 << 10
	for _, d := range docs {
		content := strings.TrimSpace(d.Content)
		if content == "" {
			continue
		}
		name := strings.TrimSpace(d.FileName)
		if name == "" {
			name = d.ID
		}
		chunk := "\n\n## " + name + "\n" + content
		if b.Len()+len(chunk) > capBytes {
			b.WriteString("\n\n[knowledge truncated]")
			break
		}
		b.WriteString(chunk)
	}
	if b.Len() == len("# Knowledge") {
		return ""
	}
	return b.String()
}

func logReplyFailure(in ReplyInput, provider, model string, latency time.Duration, err error) {
	code := "upstream_error"
	status := http.StatusServiceUnavailable
	upstream := 0
	msg := ""
	var be *Error
	if errors.As(err, &be) && be != nil {
		if be.Code != "" {
			code = be.Code
		}
		if be.Status != 0 {
			status = be.Status
		}
		upstream = be.Upstream
		msg = be.Msg
	}
	if msg == "" && err != nil {
		msg = err.Error()
	}
	msg = clipLogErr(redact(msg, ""))
	logJSON(map[string]any{
		"event":           "bridge_reply_failed",
		"tenant":          in.TenantID,
		"agent":           in.AgentID,
		"provider":        provider,
		"model":           model,
		"code":            code,
		"http_status":     status,
		"upstream_status": upstream,
		"latency_ms":      latency.Milliseconds(),
		"error":           msg,
	})
}

func clipLogErr(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 180 {
		msg = msg[:180]
	}
	return msg
}

func sessionLabel(in ReplyInput, agentID string) string {
	if v := meta(in.Metadata, "conversation_id", "conversationId"); v != "" {
		return "omni:" + v
	}
	if v := meta(in.Metadata, "session_id", "sessionId"); v != "" {
		return "omni:" + v
	}
	if sid := strings.TrimSpace(in.SessionID); sid != "" && sid != agentID {
		return "omni:" + sid
	}
	return "omni:" + in.TenantID + ":" + agentID + ":" + in.ChannelAccountID + ":" + in.ContactID
}

func providerID(ag Agent, metaMap map[string]string) string {
	if v := meta(metaMap, "provider_id", "providerId"); v != "" {
		return v
	}
	if ag.ProviderID != "" {
		return ag.ProviderID
	}
	return providerFromHarness(ag.HarnessConfigJSON)
}

func providerFromHarness(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == `""` {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	return strField(m, "provider_id", "providerId")
}

func sampling(in ReplyInput, ag Agent) (*float64, *int) {
	pt, pm := paramsOf(ag.ParamsJSON)
	var temp *float64
	switch {
	case in.TemperatureSet:
		v := in.Temperature
		temp = &v
	case ag.Temperature != nil:
		temp = ag.Temperature
	case pt != nil:
		temp = pt
	}
	return temp, pm
}

func paramsOf(raw string) (*float64, *int) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil, nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil, nil
	}
	var temp *float64
	if v, ok := m["temperature"].(float64); ok {
		temp = &v
	}
	var maxTok *int
	if v, ok := m["max_tokens"].(float64); ok && v > 0 {
		n := int(v)
		maxTok = &n
	}
	return temp, maxTok
}

func agentActive(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	return s == "" || s == "ACTIVE"
}

func meta(m map[string]string, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

func strField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// SaveProvider upserts a provider. An empty APIKey keeps the stored key.
func (s *Service) SaveProvider(p Provider, keyProvided bool) (Provider, error) {
	if _, err := s.catalog().EnsureTenant(p.TenantID); err != nil {
		return Provider{}, err
	}
	prev, err := s.catalog().GetProvider(p.TenantID, p.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Provider{}, err
	}
	if err == nil {
		if !keyProvided {
			p.APIKey = prev.APIKey
		}
		if p.Models == nil {
			p.Models = prev.Models
		}
	}
	if !keyProvided && p.APIKey == "" && (err != nil) {
		// new row, no key yet — allowed; replies fail until a key is synced
	}
	if err := s.catalog().PutProvider(p); err != nil {
		return Provider{}, err
	}
	got, err := s.catalog().GetProvider(p.TenantID, p.ID)
	if err != nil {
		return Provider{}, err
	}
	got.KeySet = got.APIKey != ""
	got.APIKey = ""
	return got, nil
}

// SaveAgent upserts an agent and mirrors it into the goso agent store.
func (s *Service) SaveAgent(a Agent) (Agent, error) {
	ten, err := s.catalog().EnsureTenant(a.TenantID)
	if err != nil {
		return Agent{}, err
	}
	a.GosoTenant = ten.GosoID
	if err := s.catalog().PutAgent(a); err != nil {
		return Agent{}, err
	}
	got, err := s.catalog().GetAgent(a.TenantID, a.ID)
	if err != nil {
		return Agent{}, err
	}
	id, err := s.ensureGosoAgent(ten.GosoID, &got, got.ModelName, got.SystemPrompt)
	if err != nil {
		return Agent{}, err
	}
	got.GosoAgentID = id
	_ = s.catalog().BindGosoAgent(got.TenantID, got.ID, id)
	got, err = s.catalog().GetAgent(a.TenantID, a.ID)
	return got, err
}

// RemoveProvider deletes a provider. Missing rows are success.
func (s *Service) RemoveProvider(tenantID, providerID string) error {
	return s.catalog().DeleteProvider(tenantID, providerID)
}

// RemoveAgent deletes the synced agent and its goso agent row.
func (s *Service) RemoveAgent(tenantID, agentID string) error {
	ag, err := s.catalog().GetAgent(tenantID, agentID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil && s.Store != nil && ag.GosoAgentID != "" {
		_ = s.Store.DeleteAgent(ag.GosoAgentID)
	}
	return s.catalog().DeleteAgent(tenantID, agentID)
}
