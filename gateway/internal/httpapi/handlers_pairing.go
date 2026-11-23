// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerPairingRoutes(mux *http.ServeMux, p *auth.Pairing, st store.StoreIface) {
	if p == nil {
		p = auth.NewPairing()
	}
	aliasAPI(mux, "POST /api/pairing", handleCreatePairing(p))
	aliasAPI(mux, "POST /api/pairing/exchange", handleExchangePairing(p, st))
}

func handleCreatePairing(p *auth.Pairing) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issued, err := p.Issue(strings.TrimSpace(os.Getenv("GOSO_VIEW_TOKEN")))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "pairing unavailable")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"code":        issued.Code,
			"expires_at":  issued.ExpiresAt.Format("2006-01-02T15:04:05Z"),
			"ttl_seconds": issued.TTLSeconds,
			"role":        issued.Role,
		})
	}
}

func handleExchangePairing(p *auth.Pairing, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code string `json:"code"`
		}
		rawBody, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err := json.Unmarshal(rawBody, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(body.Code) == "" {
			writeErr(w, http.StatusBadRequest, "code is required")
			return
		}
		ex, err := p.Exchange(body.Code)
		if err != nil {
			if errors.Is(err, auth.ErrPairingExpired) {
				writeErr(w, http.StatusUnauthorized, "pairing code expired")
				return
			}
			writeErr(w, http.StatusUnauthorized, "invalid pairing code")
			return
		}
		out := map[string]any{
			"token":  ex.Token,
			"role":   ex.Role,
			"minted": ex.Minted,
		}
		if ex.Minted {
			out["expires_at"] = ex.ExpiresAt.Format("2006-01-02T15:04:05Z")
		}
		if dev, derr := pairDeviceFromBody(st, requestTenant(r), rawBody, ex.Role); derr == nil && dev != nil {
			out["device_id"] = dev.ID
		}
		writeJSON(w, http.StatusOK, out)
	}
}
