// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenLocalSQLitePersistsAgent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goso.db")
	h, closeFn, status, err := OpenLocal(path, "test")
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider == "" {
		t.Fatal("expected provider")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", strings.NewReader(`{"agent_key":"desk","display_name":"Desktop"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create agent %d %s", rr.Code, rr.Body.String())
	}
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}

	h2, close2, _, err := OpenLocal(path, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer close2()
	rr = httptest.NewRecorder()
	h2.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list %d", rr.Code)
	}
	var body struct {
		Agents []struct {
			AgentKey string `json:"agent_key"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Agents) != 1 || body.Agents[0].AgentKey != "desk" {
		t.Fatalf("sqlite reuse failed: %+v", body.Agents)
	}
}
