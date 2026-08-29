// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerConfigRoutes(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "GET /api/config", handleGetConfig(st))
	aliasAPI(mux, "PUT /api/config", handlePutConfig(st))
}

func loadOverlay(st store.StoreIface) (*store.GatewaySettings, error) {
	if st == nil {
		return &store.GatewaySettings{Values: map[string]string{}}, nil
	}
	row, err := st.GetGatewaySettings()
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &store.GatewaySettings{Values: map[string]string{}}
	}
	config.SetOverlay(row.Values)
	return row, nil
}

func writeConfigSnapshot(w http.ResponseWriter, updatedAt time.Time) {
	snap := config.BuildSnapshot(updatedAt)
	b, err := config.MarshalPublic(snap)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "config redaction failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(b, '\n'))
}

func handleGetConfig(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, err := loadOverlay(st)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "config unavailable")
			return
		}
		writeConfigSnapshot(w, row.UpdatedAt)
	}
}

func handlePutConfig(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UpdatedAt string            `json:"updated_at"`
			Values    map[string]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Values == nil {
			writeErr(w, http.StatusBadRequest, "values is required")
			return
		}
		row, err := loadOverlay(st)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "config unavailable")
			return
		}
		merged, err := config.ApplyPatch(row.Values, body.Values)
		if err != nil {
			var pe *config.PatchError
			if errors.As(err, &pe) {
				writeErr(w, pe.Status, pe.Message)
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		stamp := time.Time{}
		if strings.TrimSpace(body.UpdatedAt) != "" {
			t, perr := time.Parse(time.RFC3339Nano, body.UpdatedAt)
			if perr != nil {
				t, perr = time.Parse(time.RFC3339, body.UpdatedAt)
			}
			if perr != nil {
				writeErr(w, http.StatusBadRequest, "invalid updated_at")
				return
			}
			stamp = t
		}
		saved, err := st.PutGatewaySettings(store.GatewaySettings{Values: merged, UpdatedAt: stamp})
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeErr(w, http.StatusConflict, "config was modified")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		config.SetOverlay(saved.Values)
		writeConfigSnapshot(w, saved.UpdatedAt)
	}
}
