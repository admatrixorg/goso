// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPatchChannel_NonSecretBinding(t *testing.T) {
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "main", DisplayName: "Main"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Options{Store: st, Version: "t"})
	body := `{"dm_policy":"allowlist","group_policy":"disabled","require_mention":false,"allow_from":["u1"],"agent_id":"` + ag.ID + `","enabled":true}`
	req := httptest.NewRequest("PATCH", "/api/channels/telegram", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	var out struct {
		Channels []struct {
			Name         string   `json:"name"`
			BoundAgentID string   `json:"bound_agent_id"`
			DMPolicy     string   `json:"dm_policy"`
			AllowFrom    []string `json:"allow_from"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range out.Channels {
		if c.Name != "telegram" {
			continue
		}
		found = true
		if c.BoundAgentID != ag.ID || c.DMPolicy != "allowlist" || len(c.AllowFrom) != 1 {
			t.Fatalf("overlay %+v", c)
		}
	}
	if !found {
		t.Fatal("telegram missing")
	}

	bad := httptest.NewRequest("PATCH", "/api/channels/telegram", bytes.NewBufferString(`{"agent_id":"nope"}`))
	bad.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, bad)
	if w.Code != 404 {
		t.Fatalf("bad agent %d", w.Code)
	}
}
