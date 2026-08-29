// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCron_EmptyList(t *testing.T) {
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cron", nil))
	if w.Code != 200 {
		t.Fatalf("empty list %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Jobs []any `json:"jobs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Jobs == nil || len(body.Jobs) != 0 {
		t.Fatalf("want jobs:[], got %s", w.Body.String())
	}
}

func TestCron_CreateListDelete(t *testing.T) {
	st, h := newTestServer()
	a, err := st.CreateAgent(store.Agent{AgentKey: "cj", DisplayName: "CJ"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cron", bytes.NewBufferString(
		`{"spec":"every:1h","session_id":"`+sess.ID+`","message":"heartbeat"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created store.CronJob
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Spec != "every:1h" || !created.Enabled {
		t.Fatalf("created %+v", created)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cron", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Jobs []store.CronJob `json:"jobs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Jobs) != 1 || listed.Jobs[0].ID != created.ID {
		t.Fatalf("listed %+v", listed)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/cron/"+created.ID, nil))
	if w.Code != 200 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cron", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Jobs) != 0 {
		t.Fatalf("after delete %+v", listed)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/cron/"+created.ID, nil))
	if w.Code != 404 {
		t.Fatalf("delete missing %d %s", w.Code, w.Body.String())
	}
}

func TestCron_InvalidSpec400(t *testing.T) {
	st, h := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "bad", DisplayName: "B"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cron", bytes.NewBufferString(
		`{"spec":"hourly","session_id":"`+sess.ID+`","message":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("invalid spec %d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("invalid spec")) {
		t.Fatalf("body %s", w.Body.String())
	}
}

func TestCron_FiveFieldAccepted(t *testing.T) {
	st, h := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "five", DisplayName: "F"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cron", bytes.NewBufferString(
		`{"spec":"0 * * * *","session_id":"`+sess.ID+`","message":"hourly"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("5-field %d %s", w.Code, w.Body.String())
	}
}

func TestCron_V1Alias(t *testing.T) {
	_, h := newTestServer()
	assertSameGET(t, h, "/api/cron", "/v1/cron")
}

func TestCron_TickFiresChat(t *testing.T) {
	st, _ := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "fire", DisplayName: "F"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	_, err := st.CreateCronJob(store.CronJob{
		Spec: "every:1m", SessionID: sess.ID, Message: "hello cron", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt := agent.New(st, nil, nil, nil, llm.Echo{})
	cron.Tick(context.Background(), st, time.Now().UTC(), FireSessionChat(rt, st, llm.Echo{}, nil))
	msgs, err := st.ListMessages(sess.ID)
	if err != nil || len(msgs) < 2 {
		t.Fatalf("messages %v %+v", err, msgs)
	}
	var user, asst bool
	for _, m := range msgs {
		if m.Role == "user" && m.Content == "hello cron" {
			user = true
		}
		if m.Role == "assistant" && m.Content == "echo: hello cron" {
			asst = true
		}
	}
	if !user || !asst {
		t.Fatalf("chat not persisted %+v", msgs)
	}
}

func TestCron_Cap20(t *testing.T) {
	st, h := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "cap", DisplayName: "C"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	for i := 0; i < store.CronJobCap; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/cron", bytes.NewBufferString(
			`{"spec":"every:1h","session_id":"`+sess.ID+`","message":"m"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("job %d %d %s", i, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cron", bytes.NewBufferString(
		`{"spec":"every:1h","session_id":"`+sess.ID+`","message":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("cap %d %s", w.Code, w.Body.String())
	}
}

func TestCron_PatchEnableAndLastError(t *testing.T) {
	st, h := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "en", DisplayName: "E"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cron", bytes.NewBufferString(
		`{"spec":"every:1h","session_id":"`+sess.ID+`","message":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created store.CronJob
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/cron/"+created.ID, bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("disable %d %s", w.Code, w.Body.String())
	}
	got, _ := st.GetCronJob(created.ID)
	if got.Enabled {
		t.Fatal("still enabled")
	}
	_ = st.MarkCronError(created.ID, `401 Bearer secret-token {"token":"secret-token"}`)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cron", nil))
	body := w.Body.String()
	if !strings.Contains(body, `"last_error"`) {
		t.Fatalf("missing last_error %s", body)
	}
	if strings.Contains(body, "secret-token") {
		t.Fatalf("leaked last_error %s", body)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/cron/missing", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}
}
