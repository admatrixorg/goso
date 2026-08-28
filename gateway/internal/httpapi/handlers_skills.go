// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
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
				writeSkillErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, doc)
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q != "" {
			hits, err := skill.Search(q)
			if err != nil {
				if errors.Is(err, skill.ErrNotConfigured) {
					writeJSON(w, http.StatusOK, map[string]any{"skills": []skill.Hit{}})
					return
				}
				writeSkillErr(w, err)
				return
			}
			if hits == nil {
				hits = []skill.Hit{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"skills": hits})
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

func handleCreateSkill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		doc, err := skill.Create(body.Name, body.Body)
		if err != nil {
			writeSkillManageErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	}
}

func handleDeleteSkill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if err := skill.Delete(name); err != nil {
			writeSkillManageErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func writeSkillErr(w http.ResponseWriter, err error) {
	writeSkillErrCode(w, err, http.StatusOK)
}

func writeSkillManageErr(w http.ResponseWriter, err error) {
	writeSkillErrCode(w, err, http.StatusBadRequest)
}

func writeSkillErrCode(w http.ResponseWriter, err error, notConfigured int) {
	switch {
	case errors.Is(err, skill.ErrNotConfigured):
		writeJSON(w, notConfigured, map[string]any{"error": "not_configured"})
	case errors.Is(err, skill.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found")
	case errors.Is(err, skill.ErrPathEscape):
		writeErr(w, http.StatusBadRequest, "path escape")
	case errors.Is(err, skill.ErrInvalidName):
		writeErr(w, http.StatusBadRequest, "invalid name")
	case errors.Is(err, skill.ErrTooLarge):
		writeErr(w, http.StatusBadRequest, "too large")
	default:
		writeErr(w, http.StatusBadRequest, "read failed")
	}
}
