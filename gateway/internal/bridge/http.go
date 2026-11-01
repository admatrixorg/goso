// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

func (s *Service) verify(h http.Header, payload []byte) error {
	secret := ""
	if s != nil {
		secret = s.Secret
	}
	return verifySignature(secret, h.Get(headerTimestamp), h.Get(headerSignature), h.Get(headerNonce), payload, s.now(), s.skew())
}

func (s *Service) readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, s.maxBody()+1))
	if err != nil {
		return nil, badRequest("cannot read body")
	}
	if int64(len(body)) > s.maxBody() {
		return nil, errStatus(http.StatusRequestEntityTooLarge, "payload too large")
	}
	return body, nil
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"healthy": true,
		"version": s.version(),
	})
}

func (s *Service) handleReply(w http.ResponseWriter, r *http.Request) {
	in, err := s.readReply(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	out, err := s.Reply(r.Context(), in, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, replyJSON(out))
}

// handleReplyStream serves POST /v1/chat/reply/stream (the path omni-core
// calls) and POST /v1/chat/stream. Auth and JSON body match POST /v1/chat/reply.
// The response is text/event-stream. Core ignores event: lines and reads each
// data: JSON object (delta, is_finished, stop_reason; reply_text only if no
// delta has been seen yet). POST /v1/chat/reply stays JSON.
func (s *Service) handleReplyStream(w http.ResponseWriter, r *http.Request) {
	in, err := s.readReply(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	started := false
	start := func() {
		if started {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		started = true
	}
	out, err := s.Reply(r.Context(), in, func(delta string) {
		if delta == "" {
			return
		}
		start()
		_ = writeSSE(w, "delta", map[string]any{
			"delta":       delta,
			"chunk_text":  delta,
			"is_finished": false,
		})
	})
	if err != nil {
		if !started {
			writeErr(w, err)
			return
		}
		_ = writeSSE(w, "error", streamErrPayload(err))
		return
	}
	if !started && out.ReplyText != "" {
		start()
		_ = writeSSE(w, "delta", map[string]any{
			"delta":       out.ReplyText,
			"chunk_text":  out.ReplyText,
			"is_finished": false,
		})
	}
	start()
	_ = writeSSE(w, "done", map[string]any{
		"reply_text":        out.ReplyText,
		"chunk_text":        "",
		"is_finished":       true,
		"stop_reason":       out.StopReason,
		"prompt_tokens":     out.PromptTokens,
		"completion_tokens": out.CompletionTokens,
		"latency_ms":        out.LatencyMS,
		"delay_ms":          out.DelayMS,
	})
}

func (s *Service) readReply(r *http.Request) (ReplyInput, error) {
	body, err := s.readBody(r)
	if err != nil {
		return ReplyInput{}, err
	}
	if err := s.verify(r.Header, body); err != nil {
		return ReplyInput{}, err
	}
	tenantID := strings.TrimSpace(r.Header.Get(headerTenant))
	if tenantID == "" {
		return ReplyInput{}, unauthorized("missing x-goclaw-tenant")
	}
	in, err := parseReplyBody(body)
	if err != nil {
		return ReplyInput{}, err
	}
	if in.TenantID != "" && in.TenantID != tenantID {
		return ReplyInput{}, unauthorized("tenant mismatch")
	}
	in.TenantID = tenantID
	if strings.TrimSpace(in.AgentID) == "" {
		in.AgentID = strings.TrimSpace(r.Header.Get(headerAgent))
	}
	return in, nil
}

func replyJSON(out ReplyOutput) map[string]any {
	return map[string]any{
		"reply_text":        out.ReplyText,
		"attachment_url":    "",
		"attachment_kind":   "",
		"delay_ms":          out.DelayMS,
		"prompt_tokens":     out.PromptTokens,
		"completion_tokens": out.CompletionTokens,
		"latency_ms":        out.LatencyMS,
		"stop_reason":       out.StopReason,
	}
}

func writeSSE(w http.ResponseWriter, event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func streamErrPayload(err error) map[string]any {
	msg := "internal error"
	body := map[string]any{"error": msg, "is_finished": true}
	var be *Error
	if errors.As(err, &be) && be != nil {
		if be.Msg != "" {
			body["error"] = be.Msg
		}
		if be.Code != "" {
			body["code"] = be.Code
		}
		if be.Upstream != 0 {
			body["upstream_status"] = be.Upstream
		}
	}
	return body
}

func parseReplyBody(body []byte) (ReplyInput, error) {
	var raw struct {
		TenantID     string            `json:"tenant_id"`
		AgentID      string            `json:"agent_id"`
		Model        string            `json:"model"`
		SystemPrompt string            `json:"system_prompt"`
		Temperature  *float64          `json:"temperature"`
		Prompt       string            `json:"prompt"`
		History      json.RawMessage   `json:"history"`
		SessionID    string            `json:"session_id"`
		Metadata     map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ReplyInput{}, badRequest("invalid json")
	}
	hist, err := parseHistory(raw.History)
	if err != nil {
		return ReplyInput{}, badRequest("invalid history")
	}
	in := ReplyInput{
		TenantID:     raw.TenantID,
		AgentID:      raw.AgentID,
		Model:        raw.Model,
		SystemPrompt: raw.SystemPrompt,
		Prompt:       raw.Prompt,
		History:      hist,
		SessionID:    raw.SessionID,
		Metadata:     raw.Metadata,
	}
	if raw.Temperature != nil {
		in.Temperature = *raw.Temperature
		in.TemperatureSet = true
	}
	return in, nil
}

func parseHistory(raw json.RawMessage) ([]llm.Message, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]llm.Message, 0, len(items))
	for _, m := range items {
		out = append(out, llm.Message{
			Role:    strField(m, "role", "Role"),
			Content: strField(m, "content", "Content"),
		})
	}
	return out, nil
}

func (s *Service) handlePutProvider(w http.ResponseWriter, r *http.Request) {
	body, err := s.authed(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	tenantID := r.PathValue("tenantId")
	providerID := r.PathValue("providerId")
	if err := sameTenant(r, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	p, keyProvided, activeSet, err := parseProvider(body, tenantID, providerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if prev, gerr := s.catalog().GetProvider(tenantID, providerID); gerr == nil && !activeSet {
		p.Active = prev.Active
	}
	saved, err := s.SaveProvider(p, keyProvided)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": publicProvider(saved)})
}

func (s *Service) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authed(r); err != nil {
		writeErr(w, err)
		return
	}
	tenantID := r.PathValue("tenantId")
	if err := sameTenant(r, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.RemoveProvider(tenantID, r.PathValue("providerId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	body, err := s.authed(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	tenantID := r.PathValue("tenantId")
	agentID := r.PathValue("agentId")
	if err := sameTenant(r, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	a, err := parseAgent(body, tenantID, agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if prev, gerr := s.catalog().GetAgent(tenantID, agentID); gerr == nil {
		if a.Temperature == nil {
			a.Temperature = prev.Temperature
		}
		if a.ProviderID == "" {
			a.ProviderID = prev.ProviderID
		}
		if a.ModelName == "" {
			a.ModelName = prev.ModelName
		}
		if a.SystemPrompt == "" {
			a.SystemPrompt = prev.SystemPrompt
		}
	}
	saved, err := s.SaveAgent(a)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": publicAgent(saved)})
}

func (s *Service) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authed(r); err != nil {
		writeErr(w, err)
		return
	}
	tenantID := r.PathValue("tenantId")
	if err := sameTenant(r, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.RemoveAgent(tenantID, r.PathValue("agentId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handlePutKnowledge(w http.ResponseWriter, r *http.Request) {
	body, err := s.authed(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	tenantID := r.PathValue("tenantId")
	agentID := r.PathValue("agentId")
	if err := sameTenant(r, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	if hdr := strings.TrimSpace(r.Header.Get(headerAgent)); hdr != "" && hdr != agentID {
		writeErr(w, unauthorized("agent mismatch"))
		return
	}
	docs, err := parseKnowledge(body, tenantID, agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.catalog().EnsureTenant(tenantID); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.catalog().ReplaceKnowledge(tenantID, agentID, docs); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "SYNCED",
		"tenant_id": tenantID,
		"agent_id":  agentID,
		"documents": len(docs),
	})
}

func parseKnowledge(body []byte, tenantID, agentID string) ([]KnowledgeDoc, error) {
	var raw struct {
		TenantID  string `json:"tenant_id"`
		AgentID   string `json:"agent_id"`
		Documents []struct {
			ID       string `json:"id"`
			FileName string `json:"file_name"`
			MimeType string `json:"mime_type"`
			FileSize int64  `json:"file_size"`
			Content  string `json:"content"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, badRequest("invalid json")
	}
	if raw.TenantID != "" && raw.TenantID != tenantID {
		return nil, badRequest("tenant_id does not match path")
	}
	if raw.AgentID != "" && raw.AgentID != agentID {
		return nil, badRequest("agent_id does not match path")
	}
	docs := make([]KnowledgeDoc, 0, len(raw.Documents))
	for i, d := range raw.Documents {
		id := strings.TrimSpace(d.ID)
		if id == "" {
			id = strings.TrimSpace(d.FileName)
		}
		if id == "" {
			id = "doc-" + strconv.Itoa(i+1)
		}
		docs = append(docs, KnowledgeDoc{
			ID:       id,
			FileName: d.FileName,
			MimeType: d.MimeType,
			FileSize: d.FileSize,
			Content:  d.Content,
		})
	}
	return docs, nil
}

func (s *Service) authed(r *http.Request) ([]byte, error) {
	body, err := s.readBody(r)
	if err != nil {
		return nil, err
	}
	if err := s.verify(r.Header, body); err != nil {
		return nil, err
	}
	return body, nil
}

func sameTenant(r *http.Request, pathTenant string) error {
	h := strings.TrimSpace(r.Header.Get(headerTenant))
	if h == "" {
		return unauthorized("missing x-goclaw-tenant")
	}
	if h != pathTenant {
		return unauthorized("tenant mismatch")
	}
	return nil
}

func parseProvider(body []byte, tenantID, providerID string) (Provider, bool, bool, error) {
	var raw struct {
		ID           string   `json:"id"`
		TenantID     string   `json:"tenant_id"`
		Name         string   `json:"name"`
		ProviderType string   `json:"provider_type"`
		Type         string   `json:"type"`
		BaseURL      string   `json:"base_url"`
		APIKey       *string  `json:"api_key"`
		Models       []string `json:"models"`
		IsActive     *bool    `json:"is_active"`
		DefaultModel string   `json:"default_model"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Provider{}, false, false, badRequest("invalid json")
	}
	if raw.TenantID != "" && raw.TenantID != tenantID {
		return Provider{}, false, false, badRequest("tenant_id does not match path")
	}
	if raw.ID != "" && raw.ID != providerID {
		return Provider{}, false, false, badRequest("id does not match path")
	}
	typ := firstNonEmpty(raw.ProviderType, raw.Type)
	models := raw.Models
	if models == nil && raw.DefaultModel != "" {
		models = []string{raw.DefaultModel}
	}
	active := true
	if raw.IsActive != nil {
		active = *raw.IsActive
	}
	p := Provider{
		TenantID: tenantID,
		ID:       providerID,
		Name:     raw.Name,
		Type:     typ,
		BaseURL:  raw.BaseURL,
		Models:   models,
		Active:   active,
	}
	if raw.APIKey != nil {
		p.APIKey = *raw.APIKey
		return p, true, raw.IsActive != nil, nil
	}
	return p, false, raw.IsActive != nil, nil
}

func parseAgent(body []byte, tenantID, agentID string) (Agent, error) {
	var raw struct {
		ID                string          `json:"id"`
		TenantID          string          `json:"tenant_id"`
		Name              string          `json:"name"`
		Description       string          `json:"description"`
		HarnessType       string          `json:"harness_type"`
		ModelName         string          `json:"model_name"`
		Model             string          `json:"model"`
		SystemPrompt      string          `json:"system_prompt"`
		Temperature       *float64        `json:"temperature"`
		DailyReplyCap     int             `json:"daily_reply_cap"`
		Status            string          `json:"status"`
		HarnessConfigJSON string          `json:"harness_config_json"`
		ProviderID        string          `json:"provider_id"`
		Provider          string          `json:"provider"`
		Params            json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Agent{}, badRequest("invalid json")
	}
	if raw.TenantID != "" && raw.TenantID != tenantID {
		return Agent{}, badRequest("tenant_id does not match path")
	}
	if raw.ID != "" && raw.ID != agentID {
		return Agent{}, badRequest("id does not match path")
	}
	params := "{}"
	if len(raw.Params) > 0 && string(raw.Params) != "null" {
		if !json.Valid(raw.Params) {
			return Agent{}, badRequest("invalid params")
		}
		params = string(raw.Params)
	}
	return Agent{
		TenantID:          tenantID,
		ID:                agentID,
		Name:              raw.Name,
		Description:       raw.Description,
		HarnessType:       raw.HarnessType,
		ProviderID:        firstNonEmpty(raw.ProviderID, raw.Provider),
		ModelName:         firstNonEmpty(raw.ModelName, raw.Model),
		SystemPrompt:      raw.SystemPrompt,
		Temperature:       raw.Temperature,
		DailyReplyCap:     raw.DailyReplyCap,
		Status:            raw.Status,
		HarnessConfigJSON: raw.HarnessConfigJSON,
		ParamsJSON:        params,
	}, nil
}

func publicProvider(p Provider) map[string]any {
	return map[string]any{
		"id":            p.ID,
		"tenant_id":     p.TenantID,
		"name":          p.Name,
		"provider_type": p.Type,
		"base_url":      p.BaseURL,
		"models":        cloneModels(p.Models),
		"is_active":     p.Active,
		"api_key_set":   p.KeySet,
	}
}

func publicAgent(a Agent) map[string]any {
	var params any
	if json.Unmarshal([]byte(a.ParamsJSON), &params) != nil {
		params = map[string]any{}
	}
	out := map[string]any{
		"id":                  a.ID,
		"tenant_id":           a.TenantID,
		"name":                a.Name,
		"description":         a.Description,
		"harness_type":        a.HarnessType,
		"provider_id":         a.ProviderID,
		"model_name":          a.ModelName,
		"system_prompt":       a.SystemPrompt,
		"daily_reply_cap":     a.DailyReplyCap,
		"status":              a.Status,
		"harness_config_json": a.HarnessConfigJSON,
		"params":              params,
	}
	if a.Temperature != nil {
		out["temperature"] = *a.Temperature
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal error"
	var be *Error
	if errors.As(err, &be) && be != nil {
		if be.Status != 0 {
			status = be.Status
		}
		if be.Msg != "" {
			msg = be.Msg
		}
	}
	body := map[string]any{"error": msg}
	if be != nil && be.Code != "" {
		body["code"] = be.Code
	}
	if be != nil && be.Upstream != 0 {
		body["upstream_status"] = be.Upstream
	}
	writeJSON(w, status, body)
}
