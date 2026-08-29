// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

var unofficialOnce sync.Once

func registerZaloPersonalQR(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "GET /api/channels/zalo-personal/qr", handlePersonalQR(st))
	aliasAPI(mux, "POST /api/channels/zalo-personal/logout", handlePersonalLogout(st))
}

func handlePersonalQR(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		unofficialOnce.Do(func() {
			log.Printf("zalo-personal unofficial")
		})
		status := "unconfigured"
		if channel.SecretSet(st, "zalo-personal", "session", []string{"GOSO_ZALO_PERSONAL_TOKEN"}) {
			status = "confirmed"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     status,
			"expires_at": "",
		})
	}
}

func handlePersonalLogout(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = secrets.Delete(st, channel.SecretName("zalo-personal", "session"))
		// do not unset process env
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func personalEnvSet() bool {
	return strings.TrimSpace(os.Getenv("GOSO_ZALO_PERSONAL_TOKEN")) != ""
}
