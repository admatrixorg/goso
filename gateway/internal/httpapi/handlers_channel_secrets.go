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

func handlePutChannelSecrets(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if !channel.Known(name) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if store.LiteEnabled() {
			writeErr(w, http.StatusForbidden, "lite: channels off")
			return
		}
		if channel.WritableFields(name) == nil {
			if name == "zalo-personal" {
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
			kind, ok := channel.FieldKind(name, lk)
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

func handleDeleteChannelSecrets(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if !channel.Known(name) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if store.LiteEnabled() {
			writeErr(w, http.StatusForbidden, "lite: channels off")
			return
		}
		fields := channel.WritableFields(name)
		if fields == nil {
			if name == "zalo-personal" {
				writeErr(w, http.StatusBadRequest, "zalo-personal uses QR, not a token form")
				return
			}
			writeErr(w, http.StatusConflict, "parked")
			return
		}
		cleared := make([]string, 0, len(fields))
		for _, field := range fields {
			kind, ok := channel.FieldKind(name, field)
			if !ok {
				continue
			}
			if err := secrets.Delete(st, channel.SecretName(name, kind)); err != nil {
				writeErr(w, http.StatusInternalServerError, "clear failed")
				return
			}
			cleared = append(cleared, field)
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
	for _, c := range overlayChannelRows(st, channel.CatalogWith(st, nil)) {
		if c.Name == name {
			return c
		}
	}
	return channel.Info{Name: name}
}

func handleTestChannel(st store.StoreIface, mgr *channel.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if !channel.Known(name) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if store.LiteEnabled() {
			writeErr(w, http.StatusForbidden, "lite: channels off")
			return
		}
		if name == "discord" || name == "slack" || name == "feishu" || name == "whatsapp" {
			writeErr(w, http.StatusConflict, "parked")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		switch name {
		case "telegram":
			tg := &channel.Telegram{Store: st}
			if mgr != nil && mgr.Telegram != nil {
				tg = mgr.Telegram
			}
			if err := tg.ProbeToken(ctx); err != nil {
				if mgr != nil {
					mgr.SetFailed("telegram", err.Error())
				}
				writeJSON(w, http.StatusBadGateway, map[string]any{
					"ok":     false,
					"name":   name,
					"health": "failed",
					"error":  channelRedactPublic(err.Error()),
				})
				return
			}
			if mgr != nil {
				mode := "poll"
				if t := mgr.Transport("telegram"); t != "" {
					mode = t
				}
				mgr.SetRunning("telegram", mode)
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
