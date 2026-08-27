// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/skill"
)

func handleListSkills() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name != "" {
			doc, err := skill.Load(name)
			if err != nil {
				if errors.Is(err, skill.ErrNotConfigured) {
					writeJSON(w, http.StatusOK, map[string]any{"error": "not_configured"})
					return
				}
				if errors.Is(err, skill.ErrNotFound) {
					writeErr(w, http.StatusNotFound, "not_found")
					return
				}
				if errors.Is(err, skill.ErrPathEscape) {
					writeErr(w, http.StatusBadRequest, "path escape")
					return
				}
				writeErr(w, http.StatusBadRequest, "read failed")
				return
			}
			writeJSON(w, http.StatusOK, doc)
			return
		}
		list, err := skill.List()
		if err != nil {
			if errors.Is(err, skill.ErrNotConfigured) {
				writeJSON(w, http.StatusOK, map[string]any{"skills": []skill.Info{}})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"skills": []skill.Info{}})
			return
		}
		if list == nil {
			list = []skill.Info{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"skills": list})
	}
}
