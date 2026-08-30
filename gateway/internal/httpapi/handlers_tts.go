// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/tts"
)

func registerTTSRoutes(mux *http.ServeMux, opt Options) {
	svc := opt.TTS
	if svc == nil {
		svc = tts.Default()
	}
	al := opt.Audit
	aliasAPI(mux, "GET /api/tts", handleTTSGet(svc))
	aliasAPI(mux, "PUT /api/tts", handleTTSPut(svc, al))
	aliasAPI(mux, "POST /api/tts/test", handleTTSTest(svc, al))
	aliasAPI(mux, "POST /api/tts/clear", handleTTSClear(svc, al))
}

func handleTTSGet(svc *tts.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeTTSJSON(w, http.StatusOK, svc.Public())
	}
}

func handleTTSPut(svc *tts.Service, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body tts.Write
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		pub, err := svc.Put(body)
		if err != nil {
			writeTTSErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "configure", Entity: "tts", EntityID: pub.Provider,
			After: auditMeta(true, map[string]any{
				"provider": pub.Provider, "configured": pub.Configured, "key_set": pub.KeySet,
				"auto_apply": pub.AutoApply, "enabled": pub.Enabled,
			}),
		})
		writeTTSJSON(w, http.StatusOK, pub)
	}
}

func handleTTSTest(svc *tts.Service, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<16))
		}
		res := svc.Test()
		recordAudit(al, r, auditlog.Record{
			Action: "test", Entity: "tts", EntityID: res.Provider,
			After: auditMeta(res.OK, map[string]any{"ok": res.OK, "kind": res.Kind, "configured": res.Configured}),
		})
		if !res.OK {
			status := http.StatusBadRequest
			writeTTSJSON(w, status, res)
			return
		}
		writeTTSJSON(w, http.StatusOK, res)
	}
}

func handleTTSClear(svc *tts.Service, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		pub, err := svc.Clear(body.Confirm)
		if err != nil {
			writeTTSErr(w, err)
			return
		}
		recordAudit(al, r, auditlog.Record{
			Action: "clear", Entity: "tts",
			After: auditMeta(true, map[string]any{"configured": pub.Configured, "key_set": pub.KeySet}),
		})
		writeTTSJSON(w, http.StatusOK, pub)
	}
}

func writeTTSJSON(w http.ResponseWriter, status int, v any) {
	pub, ok := tts.AsPublicJSON(v)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "secret-shaped payload")
		return
	}
	writeJSON(w, status, pub)
}

func writeTTSErr(w http.ResponseWriter, err error) {
	msg := tts.Redact(err.Error())
	switch {
	case errors.Is(err, tts.ErrNotConfigured), errors.Is(err, tts.ErrDisabled), errors.Is(err, tts.ErrConfirm), errors.Is(err, tts.ErrProvider), errors.Is(err, tts.ErrApply):
		writeErr(w, http.StatusBadRequest, msg)
	case errors.Is(err, tts.ErrEnvOwned):
		writeErr(w, http.StatusConflict, msg)
	default:
		writeErr(w, http.StatusInternalServerError, msg)
	}
}
