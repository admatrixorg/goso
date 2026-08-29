// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerProviderRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/providers", handleListProviders(opt))
	aliasAPI(mux, "POST /api/providers", handleCreateProvider(opt))
	aliasAPI(mux, "PATCH /api/providers/{name}", handlePatchProvider(opt))
	aliasAPI(mux, "DELETE /api/providers/{name}/key", handleDeleteProviderKey(opt))
	aliasAPI(mux, "POST /api/providers/{name}/test", handleTestProvider(opt))
}

func handleListProviders(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"providers": mergeProviderInfosTenant(opt, requestTenant(r))})
	}
}

func mergeProviderInfosTenant(opt Options, tid string) []llm.ProviderInfo {
	all := mergeProviderInfos(opt)
	out := make([]llm.ProviderInfo, 0, len(all))
	for _, inf := range all {
		if inf.Source == llm.SourceSQLite && opt.Store != nil {
			row, err := opt.Store.GetLLMProvider(inf.Name)
			if err != nil || row == nil || !store.SameTenant(row.TenantID, tid) {
				continue
			}
		}
		out = append(out, inf)
	}
	return out
}

func mergeProviderInfos(opt Options) []llm.ProviderInfo {
	reg := opt.LLM
	if reg == nil {
		reg = llm.NewRegistry()
	}
	env := map[string]llm.ProviderInfo{}
	for _, inf := range reg.Infos() {
		inf.Source = llm.SourceEnv
		env[inf.Name] = inf
	}
	out := make([]llm.ProviderInfo, 0, len(env)+4)
	for _, inf := range env {
		out = append(out, inf)
	}
	if opt.Store != nil {
		for _, row := range opt.Store.ListLLMProviders() {
			if row == nil {
				continue
			}
			if _, clash := env[row.Name]; clash {
				continue
			}
			keySet := false
			if sec, err := opt.Store.GetSecret(llm.APIKeySecretName(row.Name)); err == nil && sec != nil {
				keySet = true
			}
			out = append(out, llm.ProviderInfo{
				Name:    row.Name,
				Type:    row.Type,
				BaseURL: row.BaseURL,
				Model:   row.Model,
				KeySet:  keySet,
				Source:  llm.SourceSQLite,
				Enabled: row.Enabled,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func publicProvider(opt Options, name string) llm.ProviderInfo {
	for _, inf := range mergeProviderInfos(opt) {
		if inf.Name == name {
			return inf
		}
	}
	return llm.ProviderInfo{Name: name, Source: llm.SourceSQLite}
}

type providerWrite struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	Enabled *bool  `json:"enabled"`
}

func handleCreateProvider(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opt.Store == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		var body providerWrite
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		body.Type = strings.TrimSpace(body.Type)
		body.BaseURL = strings.TrimSpace(body.BaseURL)
		body.Model = strings.TrimSpace(body.Model)
		key := strings.TrimSpace(body.APIKey)
		if body.Name == "" {
			writeErr(w, http.StatusBadRequest, "name is required")
			return
		}
		if !llm.ValidType(body.Type) {
			writeErr(w, http.StatusBadRequest, "unknown type")
			return
		}
		reg := opt.LLM
		if reg == nil {
			reg = llm.NewRegistry()
		}
		if reg.Has(body.Name) {
			writeErr(w, http.StatusConflict, "already exists")
			return
		}
		if key != "" {
			if _, _, err := secrets.Encrypt([]byte(key)); err != nil {
				if errors.Is(err, secrets.ErrNoMasterKey) {
					writeErr(w, http.StatusBadRequest, "master key required")
					return
				}
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		row, err := opt.Store.CreateLLMProvider(store.LLMProvider{
			Name: body.Name, TenantID: requestTenant(r), Type: body.Type, BaseURL: body.BaseURL, Model: body.Model, Enabled: enabled,
		})
		if err != nil {
			if errors.Is(err, store.ErrExists) {
				writeErr(w, http.StatusConflict, "already exists")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if key != "" {
			if err := secrets.Put(opt.Store, llm.APIKeySecretName(row.Name), []byte(key)); err != nil {
				if errors.Is(err, secrets.ErrNoMasterKey) {
					writeErr(w, http.StatusBadRequest, "master key required")
					return
				}
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		recordAudit(opt.Audit, r, auditlog.Record{
			Action: "create", Entity: "provider", EntityID: row.Name,
			After: auditMeta(true, map[string]any{"type": row.Type, "key_set": key != ""}),
		})
		writeJSON(w, http.StatusCreated, publicProvider(opt, row.Name))
	}
}

func handlePatchProvider(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opt.Store == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		name := strings.TrimSpace(r.PathValue("name"))
		reg := opt.LLM
		if reg == nil {
			reg = llm.NewRegistry()
		}
		cur, err := opt.Store.GetLLMProvider(name)
		if err != nil {
			if reg.Has(name) {
				writeErr(w, http.StatusBadRequest, "env overlay")
				return
			}
			writeErr(w, http.StatusNotFound, "provider not found")
			return
		}
		if hideWrongTenant(w, cur.TenantID, requestTenant(r)) {
			return
		}
		if reg.Has(name) {
			writeErr(w, http.StatusBadRequest, "env overlay")
			return
		}
		var body struct {
			Type    *string `json:"type"`
			BaseURL *string `json:"base_url"`
			Model   *string `json:"model"`
			APIKey  *string `json:"api_key"`
			Enabled *bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		upd := *cur
		if body.Type != nil {
			t := strings.TrimSpace(*body.Type)
			if t != "" && !llm.ValidType(t) {
				writeErr(w, http.StatusBadRequest, "unknown type")
				return
			}
			if t != "" {
				upd.Type = t
			}
		}
		if body.BaseURL != nil {
			upd.BaseURL = strings.TrimSpace(*body.BaseURL)
		}
		if body.Model != nil {
			upd.Model = strings.TrimSpace(*body.Model)
		}
		if body.Enabled != nil {
			upd.Enabled = *body.Enabled
		}
		key := ""
		if body.APIKey != nil {
			key = strings.TrimSpace(*body.APIKey)
		}
		if key != "" {
			if _, _, err := secrets.Encrypt([]byte(key)); err != nil {
				if errors.Is(err, secrets.ErrNoMasterKey) {
					writeErr(w, http.StatusBadRequest, "master key required")
					return
				}
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		saved, err := opt.Store.UpdateLLMProvider(upd)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "provider not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if key != "" {
			if err := secrets.Put(opt.Store, llm.APIKeySecretName(saved.Name), []byte(key)); err != nil {
				if errors.Is(err, secrets.ErrNoMasterKey) {
					writeErr(w, http.StatusBadRequest, "master key required")
					return
				}
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		recordAudit(opt.Audit, r, auditlog.Record{
			Action: "update", Entity: "provider", EntityID: saved.Name,
			Before: auditMeta(true, map[string]any{"enabled": cur.Enabled}),
			After:  auditMeta(true, map[string]any{"enabled": saved.Enabled, "key_rotated": key != ""}),
		})
		writeJSON(w, http.StatusOK, publicProvider(opt, saved.Name))
	}
}

func handleDeleteProviderKey(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opt.Store == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		name := strings.TrimSpace(r.PathValue("name"))
		reg := opt.LLM
		if reg == nil {
			reg = llm.NewRegistry()
		}
		if reg.Has(name) {
			writeErr(w, http.StatusBadRequest, "env overlay")
			return
		}
		cur, err := opt.Store.GetLLMProvider(name)
		if err != nil {
			writeErr(w, http.StatusNotFound, "provider not found")
			return
		}
		if hideWrongTenant(w, cur.TenantID, requestTenant(r)) {
			return
		}
		if err := secrets.Delete(opt.Store, llm.APIKeySecretName(name)); err != nil {
			writeErr(w, http.StatusInternalServerError, "clear failed")
			return
		}
		info := publicProvider(opt, name)
		recordAudit(opt.Audit, r, auditlog.Record{
			Action: "clear", Entity: "provider", EntityID: name,
			After: auditMeta(true, map[string]any{"key_set": false}),
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"name":    info.Name,
			"key_set": info.KeySet,
			"source":  info.Source,
			"enabled": info.Enabled,
		})
	}
}

func handleTestProvider(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		kind := "models"
		if r.Body != nil {
			raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			trim := strings.TrimSpace(string(raw))
			if trim != "" {
				var body struct {
					Kind string `json:"kind"`
				}
				if err := json.Unmarshal(raw, &body); err != nil {
					writeErr(w, http.StatusBadRequest, "invalid json")
					return
				}
				if k := strings.TrimSpace(body.Kind); k != "" {
					kind = k
				}
			}
		}
		if kind != "models" && kind != "chat" {
			writeErr(w, http.StatusBadRequest, `kind must be "models" or "chat"`)
			return
		}
		p, ok := resolveProvider(opt, name, requestTenant(r))
		if !ok {
			writeErr(w, http.StatusNotFound, "provider not found")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, llm.Probe(ctx, p, kind))
	}
}

func resolveProvider(opt Options, name, tid string) (llm.Provider, bool) {
	reg := opt.LLM
	if reg == nil {
		reg = llm.NewRegistry()
	}
	if reg.Has(name) {
		return reg.Get(name), true
	}
	if opt.Store == nil {
		return nil, false
	}
	row, err := opt.Store.GetLLMProvider(name)
	if err != nil || row == nil || !store.SameTenant(row.TenantID, tid) {
		return nil, false
	}
	key := ""
	if b, err := secrets.Get(opt.Store, llm.APIKeySecretName(name)); err == nil {
		key = string(b)
	}
	p, err := llm.Build(row.Name, row.Type, row.BaseURL, row.Model, key)
	if err != nil {
		return nil, false
	}
	return p, true
}
