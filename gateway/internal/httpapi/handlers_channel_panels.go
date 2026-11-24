package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// The channel detail page always loads these panels. They are not ported as
// working extractors or context grants, so the honest answer is an empty
// disabled surface instead of a 404 that makes the page look broken.
func registerChannelDetailPanelRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/channels/instances/{tenant}/{kind}/{slug}/memory-extraction", handleChannelMemoryStatus(st))
	mux.HandleFunc("GET /v1/channels/instances/{tenant}/{kind}/{slug}/memory-extraction/items", handleChannelMemoryItems(st))
	mux.HandleFunc("PUT /v1/channels/instances/{tenant}/{kind}/{slug}/memory-extraction/settings", handleChannelMemorySettings(st))
	mux.HandleFunc("POST /v1/channels/instances/{tenant}/{kind}/{slug}/memory-extraction/run", handleChannelMemoryRun(st))
	mux.HandleFunc("GET /v1/channels/instances/{tenant}/{kind}/{slug}/contexts", handleChannelContexts(st))
}

func emptyMemoryConfig() map[string]any {
	return map[string]any{
		"enabled":          false,
		"review_mode":      true,
		"interval_minutes": 60,
		"message_cap":      0,
		"retention_hours":  0,
		"allowed_types":    []string{},
		"min_messages":     0,
		"group_only":       true,
	}
}

func handleChannelMemoryStatus(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := instanceIDFromPath(r)
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if cfg, err := st.GetChannelConfig(id); err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"config":        emptyMemoryConfig(),
			"pending_count": 0,
			"recent_items":  []any{},
		})
	}
}

func handleChannelMemoryItems(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := instanceIDFromPath(r)
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if cfg, err := st.GetChannelConfig(id); err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0})
	}
}

func handleChannelMemorySettings(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := instanceIDFromPath(r)
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		cfg, err := st.GetChannelConfig(id)
		if err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		var body map[string]any
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if cfg.Settings == nil {
			cfg.Settings = map[string]string{}
		}
		raw, _ := json.Marshal(body)
		cfg.Settings["memory_extraction"] = string(raw)
		cfg.UpdatedAt = time.Now().UTC()
		if perr := st.PutChannelConfig(*cfg); perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": body})
	}
}

func handleChannelMemoryRun(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := instanceIDFromPath(r)
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if cfg, err := st.GetChannelConfig(id); err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		writeErr(w, http.StatusNotImplemented, "passive memory extraction is not wired on GOSO yet")
	}
}

func handleChannelContexts(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := instanceIDFromPath(r)
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		if cfg, err := st.GetChannelConfig(id); err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "instance not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"contexts": []any{}, "total": 0})
	}
}
