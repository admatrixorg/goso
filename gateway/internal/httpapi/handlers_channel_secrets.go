// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func handlePutChannelSecrets(st store.StoreIface, mgr *channel.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := channelTarget(w, r)
		if !ok {
			return
		}
		catalog := channel.CatalogOf(name)
		if store.LiteEnabled() {
			writeErr(w, http.StatusForbidden, "lite: channels off")
			return
		}
		if channel.WritableFields(catalog) == nil {
			if catalog == "zalo-personal" {
				writeErr(w, http.StatusBadRequest, "zalo-personal uses QR, not a token form")
				return
			}
			writeErr(w, http.StatusConflict, "parked")
			return
		}
		var body map[string]any
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil && err != io.EOF {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		written := make([]string, 0, 2)
		for k, raw := range body {
			lk := strings.ToLower(strings.TrimSpace(k))
			if !channelSecretKey(lk) {
				continue
			}
			kind, ok := channel.FieldKind(catalog, lk)
			if !ok {
				writeErr(w, http.StatusBadRequest, "unknown secret field")
				return
			}
			s, _ := raw.(string)
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if err := secrets.Put(st, channel.SecretName(name, kind), []byte(s)); err != nil {
				if errors.Is(err, secrets.ErrNoMasterKey) {
					writeErr(w, http.StatusServiceUnavailable, "master key required")
					return
				}
				writeErr(w, http.StatusInternalServerError, "save failed")
				return
			}
			written = append(written, lk)
		}
		if len(written) == 0 {
			writeErr(w, http.StatusBadRequest, "no secret fields")
			return
		}
		if _, owned := channel.TenantOf(name); owned {
			mgr.Restart(name)
		}
		row := channelRowByName(st, name)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"name":       name,
			"secret_set": row.SecretSet,
			"from_env":   row.FromEnv,
			"written":    written,
		})
	}
}

func handleDeleteChannelSecrets(st store.StoreIface, mgr *channel.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := channelTarget(w, r)
		if !ok {
			return
		}
		catalog := channel.CatalogOf(name)
		if store.LiteEnabled() {
			writeErr(w, http.StatusForbidden, "lite: channels off")
			return
		}
		fields := channel.WritableFields(catalog)
		if fields == nil {
			if catalog == "zalo-personal" {
				writeErr(w, http.StatusBadRequest, "zalo-personal uses QR, not a token form")
				return
			}
			writeErr(w, http.StatusConflict, "parked")
			return
		}
		cleared := make([]string, 0, len(fields))
		for _, field := range fields {
			kind, ok := channel.FieldKind(catalog, field)
			if !ok {
				continue
			}
			if err := secrets.Delete(st, channel.SecretName(name, kind)); err != nil {
				writeErr(w, http.StatusInternalServerError, "clear failed")
				return
			}
			cleared = append(cleared, field)
		}
		if _, owned := channel.TenantOf(name); owned {
			mgr.Reconcile()
		}
		row := channelRowByName(st, name)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"name":       name,
			"secret_set": row.SecretSet,
			"from_env":   row.FromEnv,
			"cleared":    cleared,
		})
	}
}

func channelRowByName(st store.StoreIface, name string) channel.Info {
	if _, owned := channel.TenantOf(name); owned {
		return overlayChannelRows(st, []channel.Info{channel.InstanceInfo(st, nil, name)})[0]
	}
	for _, c := range overlayChannelRows(st, channel.CatalogWith(st, nil)) {
		if c.Name == name {
			return c
		}
	}
	return channel.Info{Name: name}
}

func handleTestChannel(st store.StoreIface, mgr *channel.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := channelTarget(w, r)
		if !ok {
			return
		}
		if store.LiteEnabled() {
			writeErr(w, http.StatusForbidden, "lite: channels off")
			return
		}
		catalog := channel.CatalogOf(name)
		_, owned := channel.TenantOf(name)
		if catalog == "discord" || catalog == "slack" || catalog == "feishu" || catalog == "whatsapp" || (owned && catalog != "telegram") {
			writeErr(w, http.StatusConflict, "parked")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		switch catalog {
		case "telegram":
			tg := mgr.TelegramBot(name)
			if tg == nil {
				tg = &channel.Telegram{Store: st, Instance: name}
			}
			if err := tg.ProbeToken(ctx); err != nil {
				if mgr != nil {
					mgr.SetFailed(name, err.Error())
				}
				writeJSON(w, http.StatusBadGateway, map[string]any{
					"ok":     false,
					"name":   name,
					"health": "failed",
					"error":  channelRedactPublic(err.Error()),
				})
				return
			}
			// A tenant bot shows running only when its listener really runs.
			if mgr != nil && (!owned || mgr.TelegramBot(name) != nil) {
				mode := "poll"
				if t := mgr.Transport(name); t != "" {
					mode = t
				}
				mgr.SetRunning(name, mode)
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "health": "running"})
		case "zalo-oa":
			_, _, aSet := channel.Credential(st, "zalo-oa", channel.KindAccess, []string{"GOSO_ZALO_OA_ACCESS_TOKEN"})
			_, _, sSet := channel.Credential(st, "zalo-oa", channel.KindAppSecret, []string{"GOSO_ZALO_OA_SECRET"})
			if !aSet || !sSet {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"ok":     false,
					"name":   name,
					"health": "missing",
					"error":  "access token and app secret required",
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "health": "running"})
		case "zalo-personal":
			set := channel.SecretSet(st, "zalo-personal", "session", []string{"GOSO_ZALO_PERSONAL_TOKEN"})
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":         set,
				"name":       name,
				"secret_set": set,
				"health":     map[bool]string{true: "running", false: "missing"}[set],
			})
		default:
			writeErr(w, http.StatusNotFound, "not found")
		}
	}
}

func channelRedactPublic(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}
