// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/replycheck"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// clefStub answers every question with the same score.
func clefStub(t *testing.T, score float64, calls *atomic.Int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		a := map[string]any{"type": "noul", "noul": score}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{
			"answers": map[string]any{"mentions_offer": a, "unsupported_offer": a, "other_customer_leak": a}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func checkCfg(url string, mode replycheck.Mode) replycheck.Config {
	return replycheck.Config{Mode: mode, HoldOnError: true, Timeout: 500 * time.Millisecond, URL: url, Token: "t",
		Model: "clef", OfferThreshold: 0.7, UnsupThreshold: 0.5, LeakThreshold: 0.5, FactsMaxBytes: 3000}
}

// rawSend posts a customer-channel reply and returns the body.
func (e *custEnv) rawSend(conv, prompt string, stream bool, withChannel bool) string {
	e.t.Helper()
	md := map[string]string{"channel_type": "zalo", "channel_account_id": "acc-1", "external_conversation_id": conv}
	if !withChannel {
		md = map[string]string{"conversation_id": conv}
	}
	body, _ := json.Marshal(map[string]any{"agent_id": memAgent, "model": "m1", "prompt": prompt, "metadata": md})
	path := "/v1/chat/reply"
	if stream {
		path = "/v1/chat/reply/stream"
	}
	req, _ := http.NewRequest(http.MethodPost, e.base+path, bytes.NewReader(body))
	stamp(req.Header, e.svc.Secret, tenantOne, body)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		e.t.Fatalf("%s %d %s", path, res.StatusCode, raw)
	}
	return string(raw)
}

func (e *custEnv) assistantTurns(conv string) []string {
	e.t.Helper()
	label := customerLabel(ReplyInput{TenantID: tenantOne, Metadata: map[string]string{
		"channel_type": "zalo", "channel_account_id": "acc-1", "external_conversation_id": conv}})
	msgs, err := e.svc.Store.ListMessages(e.session(label).ID)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, m := range msgs {
		if m.Role == "assistant" {
			out = append(out, m.Content)
		}
	}
	return out
}

func newCheckEnv(t *testing.T) *custEnv {
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	return newCustEnv(t, st)
}

func TestReplyCheckHoldsInventedOffer(t *testing.T) {
	for _, stream := range []bool{false, true} {
		e := newCheckEnv(t)
		var calls atomic.Int32
		e.svc.ReplyCheck = replycheck.New(checkCfg(clefStub(t, 0.01, &calls), replycheck.ModeEnforce), nil)
		e.fake.setDeltas([]string{"Dạ nhập mã ", "MOCAN99 giảm 30%", " ạ"})
		body := e.rawSend("conv-H", "Có mã giảm giá không?", stream, true)
		if strings.Contains(body, "MOCAN99") || !strings.Contains(body, StopHeldForStaff) {
			t.Fatalf("stream=%v held draft leaked or not marked: %s", stream, body)
		}
		if strings.Contains(body, `"delta"`) && !stream {
			t.Fatalf("unexpected delta: %s", body)
		}
		if stream && strings.Contains(body, "event: delta") {
			t.Fatalf("stream=%v delta sent before the check: %s", stream, body)
		}
		if got := e.assistantTurns("conv-H"); len(got) != 0 {
			t.Fatalf("held draft stored as assistant turn: %v", got)
		}
		if calls.Load() != 0 {
			t.Fatalf("code-detected invention should not call Clef (%d calls)", calls.Load())
		}
	}
}

func TestReplyCheckPassesGroundedReply(t *testing.T) {
	for _, stream := range []bool{false, true} {
		e := newCheckEnv(t)
		var calls atomic.Int32
		e.svc.ReplyCheck = replycheck.New(checkCfg(clefStub(t, 0.02, &calls), replycheck.ModeEnforce), nil)
		e.fake.setDeltas([]string{"Dạ em chào ", "chị ạ"})
		body := e.rawSend("conv-P", "Shop ơi", stream, true)
		if !strings.Contains(body, "Dạ em chào chị ạ") || strings.Contains(body, StopHeldForStaff) {
			t.Fatalf("stream=%v clean reply not sent: %s", stream, body)
		}
		if calls.Load() != 1 {
			t.Fatalf("want one Clef call, got %d", calls.Load())
		}
		if got := e.assistantTurns("conv-P"); len(got) != 1 {
			t.Fatalf("assistant turns %v", got)
		}
	}
}

func TestReplyCheckClefDownFailsClosed(t *testing.T) {
	e := newCheckEnv(t)
	e.svc.ReplyCheck = replycheck.New(checkCfg("http://127.0.0.1:1/unreachable", replycheck.ModeEnforce), nil)
	e.fake.setDeltas([]string{"Dạ em chào chị ạ"})
	if body := e.rawSend("conv-D", "Shop ơi", false, true); !strings.Contains(body, StopHeldForStaff) || strings.Contains(body, "chào") {
		t.Fatalf("Clef down must hold: %s", body)
	}
}

func TestReplyCheckShadowAndOffAndStaffPath(t *testing.T) {
	e := newCheckEnv(t)
	var calls atomic.Int32
	url := clefStub(t, 0.99, &calls)

	e.fake.setDeltas([]string{"Dạ mã MOCAN99 ạ"})
	e.svc.ReplyCheck = replycheck.New(checkCfg(url, replycheck.ModeShadow), nil)
	if body := e.rawSend("conv-S", "Mã?", false, true); !strings.Contains(body, "MOCAN99") {
		t.Fatalf("shadow must not hold: %s", body)
	}
	e.svc.ReplyCheck = replycheck.New(checkCfg(url, replycheck.ModeOff), nil)
	if body := e.rawSend("conv-O", "Mã?", false, true); !strings.Contains(body, "MOCAN99") {
		t.Fatalf("off must not hold: %s", body)
	}
	e.svc.ReplyCheck = nil
	if body := e.rawSend("conv-N", "Mã?", false, true); !strings.Contains(body, "MOCAN99") {
		t.Fatalf("nil checker must not hold: %s", body)
	}
	// Staff/agent calls without customer channel metadata are not checked.
	e.svc.ReplyCheck = replycheck.New(checkCfg(url, replycheck.ModeEnforce), nil)
	if body := e.rawSend("staff-1", "Mã?", false, false); !strings.Contains(body, "MOCAN99") {
		t.Fatalf("staff path must not be checked: %s", body)
	}
	if calls.Load() != 0 {
		t.Fatalf("Clef called %d times", calls.Load())
	}
}
