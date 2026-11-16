// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerChannelPairingRoutes(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "GET /api/channel-pairing", handleListChannelPairing(st))
	aliasAPI(mux, "POST /api/channel-pairing/{id}/approve", handleChannelPairingStatus(st, true))
	aliasAPI(mux, "POST /api/channel-pairing/{id}/deny", handleChannelPairingStatus(st, false))
}

func handleListChannelPairing(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		items := make([]map[string]any, 0)
		tenant := requestTenant(r)
		for _, p := range st.ListChannelPairings() {
			if !pairingVisible(p, tenant) {
				continue
			}
			status := p.Status
			if status == "pending" && !p.ExpiresAt.IsZero() && !p.ExpiresAt.After(now) {
				status = "expired"
			}
			row := map[string]any{
				"id":         p.ID,
				"channel":    p.Channel,
				"sender_id":  p.SenderID,
				"status":     status,
				"expires_at": p.ExpiresAt.UTC().Format(time.RFC3339),
			}
			items = append(items, row)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func handleChannelPairingStatus(st store.StoreIface, approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		now := time.Now().UTC()
		for _, p := range st.ListChannelPairings() {
			if p != nil && p.ID == id && !pairingVisible(p, requestTenant(r)) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
		}
		var err error
		if approve {
			err = channel.ApprovePairing(st, id, now)
		} else {
			err = channel.DenyPairing(st, id, now)
		}
		if err != nil {
			if errors.Is(err, channel.ErrPairingGone) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			if errors.Is(err, channel.ErrPairingExpired) {
				writeErr(w, http.StatusConflict, "pairing expired")
				return
			}
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// pairingVisible hides a tenant bot's pairing requests from other tenants. A
// pairing for the older single bot has no tenant and stays visible.
func pairingVisible(p *store.ChannelPairing, tenant string) bool {
	if p == nil {
		return false
	}
	if owner, ok := channel.TenantOf(p.Channel); ok {
		return store.SameTenant(owner, tenant)
	}
	return true
}
