// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	gosov1 "github.com/mqglobal/goso/gateway/internal/bridge/gosov1"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestVerifySignature(t *testing.T) {
	secret := "secret123"
	body := []byte(`{"prompt":"hi"}`)
	now := time.Unix(1_700_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	good := sign(secret, ts, body)

	if err := verifySignature(secret, ts, good, "", body, now, defaultSkew); err != nil {
		t.Fatal(err)
	}
	if err := verifySignature(secret, "", good, "", body, now, defaultSkew); !errors.Is(err, errMissingSig) {
		t.Fatalf("missing: %v", err)
	}
	if err := verifySignature(secret, ts, "", "", body, now, defaultSkew); !errors.Is(err, errMissingSig) {
		t.Fatalf("blank sig: %v", err)
	}
	if err := verifySignature("", ts, good, "", body, now, defaultSkew); !errors.Is(err, errMissingSig) {
		t.Fatalf("no server secret: %v", err)
	}
	if err := verifySignature(secret, ts, sign("other", ts, body), "", body, now, defaultSkew); !errors.Is(err, errBadSig) {
		t.Fatalf("bad sig: %v", err)
	}
	if err := verifySignature(secret, ts, good, "", []byte(`{"prompt":"no"}`), now, defaultSkew); !errors.Is(err, errBadSig) {
		t.Fatalf("tampered: %v", err)
	}
	old := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	if err := verifySignature(secret, old, sign(secret, old, body), "", body, now, defaultSkew); !errors.Is(err, errExpired) {
		t.Fatalf("expired: %v", err)
	}
	nonce := "abc"
	dotted := ts + "." + nonce + "." + string(body)
	if err := verifySignature(secret, ts, sign(secret, dotted, nil), nonce, body, now, defaultSkew); err != nil {
		t.Fatalf("nonce form: %v", err)
	}
}

func TestMapTenant(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	if MapTenantID(id) != id {
		t.Fatalf("uuid changed: %s", MapTenantID(id))
	}
	a := MapTenantID("@@@")
	b := MapTenantID("###")
	if a == b || a == store.DefaultTenant || b == store.DefaultTenant {
		t.Fatalf("collision or default: %s %s", a, b)
	}
	if !store.TenantOK(a) || MapTenantID("@@@") != a {
		t.Fatalf("unstable %s", a)
	}
	cat := newMem()
	first, err := cat.EnsureTenant(id)
	if err != nil || !first.Created || first.GosoID != id {
		t.Fatalf("first %+v %v", first, err)
	}
	second, err := cat.EnsureTenant(id)
	if err != nil || second.Created || second.GosoID != id {
		t.Fatalf("second %+v %v", second, err)
	}
	weird, err := cat.EnsureTenant("@@@")
	if err != nil || weird.GosoID != a {
		t.Fatalf("weird %+v %v", weird, err)
	}
}

func TestSQLCatalogSealsKey(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "goso.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat, err := OpenCatalog(st)
	if err != nil {
		t.Fatal(err)
	}
	const apiKey = "sk-bridge-test-key-9f3a"
	tenant := "00000000-0000-0000-0000-000000000001"
	ten, err := cat.EnsureTenant(tenant)
	if err != nil || !ten.Created || ten.GosoID != tenant {
		t.Fatalf("tenant %+v %v", ten, err)
	}
	again, err := cat.EnsureTenant(tenant)
	if err != nil || again.Created {
		t.Fatalf("second %+v %v", again, err)
	}
	if err := cat.PutProvider(Provider{
		TenantID: tenant,
		ID:       "prov-1",
		Name:     "core",
		Type:     "openai",
		BaseURL:  "https://zztest.invalid/v1",
		Models:   []string{"gpt-4o-mini"},
		Active:   true,
		APIKey:   apiKey,
	}); err != nil {
		t.Fatal(err)
	}
	db, _ := st.SQL()
	var ct string
	if err := db.QueryRow(`SELECT key_ct FROM omni_providers WHERE provider_id=?`, "prov-1").Scan(&ct); err != nil {
		t.Fatal(err)
	}
	if ct == "" || strings.Contains(ct, apiKey) {
		t.Fatal("api key stored in the clear")
	}
	got, err := cat.GetProvider(tenant, "prov-1")
	if err != nil || got.APIKey != apiKey || !got.KeySet {
		t.Fatalf("round trip key=%q set=%v err=%v", got.APIKey, got.KeySet, err)
	}
	if err := cat.PutProvider(Provider{
		TenantID: tenant,
		ID:       "prov-1",
		Name:     "core",
		Type:     "openai",
		BaseURL:  "https://zztest.invalid/v1",
		Models:   []string{"gpt-4o-mini"},
		Active:   true,
		APIKey:   apiKey,
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.ReplaceKnowledge(tenant, "agent-1", []KnowledgeDoc{{
		ID: "d1", FileName: "SOUL.md", MimeType: "text/markdown", FileSize: 5, Content: "hello",
	}}); err != nil {
		t.Fatal(err)
	}
	docs, err := cat.ListKnowledge(tenant, "agent-1")
	if err != nil || len(docs) != 1 || docs[0].FileName != "SOUL.md" || docs[0].Content != "hello" {
		t.Fatalf("knowledge %+v %v", docs, err)
	}
}

func TestPostgresPlaceholders(t *testing.T) {
	c := &sqlCatalog{pg: true}
	got := c.q(`SELECT * FROM t WHERE a=? AND b=?`)
	if got != `SELECT * FROM t WHERE a=$1 AND b=$2` {
		t.Fatal(got)
	}
}

func TestDefaultCompleteEcho(t *testing.T) {
	res, err := DefaultComplete(context.Background(), Completion{
		Type:     "echo",
		Messages: []llm.Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "echo: ping" || res.StopReason != "stop" {
		t.Fatalf("%+v", res)
	}
}

func TestValidateRuntime(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_SHARED_SECRET", "")
	t.Setenv("GOSO_DATABASE_URL", "")
	if err := ValidateRuntime(); err == nil || !strings.Contains(err.Error(), "GOSO_SHARED_SECRET") {
		t.Fatal(err)
	}
	t.Setenv("GOSO_SHARED_SECRET", "s3cret")
	t.Setenv("GOSO_MASTER_KEY", "nope")
	if err := ValidateRuntime(); err == nil || !strings.Contains(err.Error(), "GOSO_MASTER_KEY") {
		t.Fatal(err)
	}
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	if err := ValidateRuntime(); err == nil || !strings.Contains(err.Error(), "GOSO_DATABASE_URL") {
		t.Fatal(err)
	}
}

func TestBridgeHTTPAndGRPC(t *testing.T) {
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	cat := newMem()
	fake := &fakeLLM{text: "xin chao tu goso"}
	svc := &Service{
		Secret:   "test-secret",
		Version:  "test",
		Catalog:  cat,
		Store:    store.New(),
		Complete: fake.Complete,
	}
	addr := startBridge(t, svc)
	base := "http://" + addr
	const (
		tenant  = "00000000-0000-0000-0000-000000000001"
		provID  = "b9d32e4b-28a5-4dcc-9134-39a9a84017bf"
		agentID = "a2ffb11d-9a4c-40f9-a2e4-c57dbe834cb0"
		apiKey  = "sk-bridge-test-key-9f3a"
	)

	provBody := []byte(`{"name":"ZZTEST-provider-core","provider_type":"openai","base_url":"https://zztest.invalid/v1","api_key":"` + apiKey + `","models":["gpt-4o-mini"],"is_active":true}`)
	res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/"+provID, svc.Secret, tenant, provBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put provider %d %s", res.StatusCode, res.Body)
	}
	if strings.Contains(res.Body, apiKey) {
		t.Fatal("response leaked api key")
	}
	for _, row := range cat.providers {
		if row.APIKey != "" || strings.Contains(string(row.ct), apiKey) {
			t.Fatal("catalog stored plaintext api key")
		}
	}

	// Omitted api_key keeps the sealed key.
	keep := []byte(`{"name":"ZZTEST-provider-core","provider_type":"openai","base_url":"https://zztest.invalid/v1","models":["gpt-4o-mini"]}`)
	res = doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/"+provID, svc.Secret, tenant, keep)
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Body, `"api_key_set":true`) {
		t.Fatalf("keep key %d %s", res.StatusCode, res.Body)
	}
	got, err := cat.GetProvider(tenant, provID)
	if err != nil || got.APIKey != apiKey {
		t.Fatalf("kept key %q %v", got.APIKey, err)
	}

	agentBody := []byte(`{"name":"ZZTEST-agent","description":"probe","harness_type":"GOSO","provider_id":"` + provID + `","model_name":"gpt-4o-mini","system_prompt":"Ban la tro ly","temperature":0.2,"daily_reply_cap":5,"status":"ACTIVE"}`)
	res = doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/agents/"+agentID, svc.Secret, tenant, agentBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put agent %d %s", res.StatusCode, res.Body)
	}

	// Unsigned and bad signatures never reach the model.
	before := fake.calls()
	plain, err := http.Post(base+"/v1/chat/reply", "application/json", strings.NewReader(`{"agent_id":"`+agentID+`","prompt":"no"}`))
	if err != nil {
		t.Fatal(err)
	}
	plainBody, _ := io.ReadAll(plain.Body)
	_ = plain.Body.Close()
	if plain.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned %d %s", plain.StatusCode, plainBody)
	}
	bad := doSigned(t, http.MethodPost, base+"/v1/chat/reply", "wrong", tenant, []byte(`{"agent_id":"`+agentID+`","prompt":"no"}`))
	if bad.StatusCode != http.StatusUnauthorized || fake.calls() != before {
		t.Fatalf("bad sig status=%d calls=%d", bad.StatusCode, fake.calls())
	}
	mismatch := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/"+provID, svc.Secret, "other-tenant", provBody)
	if mismatch.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tenant mismatch %d %s", mismatch.StatusCode, mismatch.Body)
	}

	replyBody := []byte(`{"agent_id":"` + agentID + `","model":"gpt-4o-mini","system_prompt":"Ban la tro ly","temperature":0.2,"prompt":"xin chao","history":[{"Role":"user","Content":"truoc","Timestamp":1}]}`)
	res = doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reply %d %s", res.StatusCode, res.Body)
	}
	var reply struct {
		ReplyText string `json:"reply_text"`
		DelayMS   int64  `json:"delay_ms"`
	}
	if err := json.Unmarshal([]byte(res.Body), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.ReplyText != "xin chao tu goso" {
		t.Fatalf("reply %q", reply.ReplyText)
	}
	last := fake.snapshot()
	if last.APIKey != apiKey || last.Model != "gpt-4o-mini" || last.BaseURL != "https://zztest.invalid/v1" || last.Type != "openai" {
		t.Fatalf("provider routing type=%s model=%s base=%s keyset=%v", last.Type, last.Model, last.BaseURL, last.APIKey != "")
	}
	if last.Temperature == nil || *last.Temperature != 0.2 {
		t.Fatalf("temperature %+v", last.Temperature)
	}
	if len(last.Messages) != 3 || last.Messages[0].Role != "system" || last.Messages[1].Content != "truoc" || last.Messages[2].Content != "xin chao" {
		t.Fatalf("messages %+v", last.Messages)
	}
	sessions := svc.Store.ListSessions()
	if len(sessions) != 1 || sessions[0].TenantID != tenant {
		t.Fatalf("sessions %+v", sessions)
	}
	msgs, err := svc.Store.ListMessages(sessions[0].ID)
	if err != nil || len(msgs) != 2 || msgs[0].Role != "user" || msgs[0].Content != "xin chao" || msgs[1].Role != "assistant" {
		t.Fatalf("turns %+v %v", msgs, err)
	}
	agents := svc.Store.ListAgents()
	if len(agents) != 1 || agents[0].TenantID != tenant || agents[0].AgentKey != "omni:"+tenant+":"+agentID {
		t.Fatalf("agent %+v", agents)
	}

	// gRPC uses session_id as the agent id, matching omni-core's client.
	h2 := h2cHTTPClient()
	gclient := connect.NewClient[gosov1.GenerateReplyRequest, gosov1.GenerateReplyResponse](
		h2,
		base+procGenerate,
		connect.WithGRPC(),
	)
	greq := connect.NewRequest(&gosov1.GenerateReplyRequest{
		SessionId:    agentID,
		TenantId:     tenant,
		Model:        "gpt-4o-mini",
		SystemPrompt: "Ban la tro ly",
		Prompt:       "grpc xin chao",
		Temperature:  0.2,
	})
	stamp(greq.Header(), svc.Secret, tenant, []byte("grpc xin chao"))
	gres, err := gclient.CallUnary(context.Background(), greq)
	if err != nil {
		t.Fatal(err)
	}
	if gres.Msg.GetReplyText() != "xin chao tu goso" || gres.Msg.GetStopReason() != "stop" {
		t.Fatalf("grpc %+v", gres.Msg)
	}

	unauth := connect.NewRequest(&gosov1.GenerateReplyRequest{SessionId: agentID, TenantId: tenant, Prompt: "x"})
	_, err = gclient.CallUnary(context.Background(), unauth)
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeUnauthenticated {
		t.Fatalf("grpc unsigned %v", err)
	}

	fake.setDeltas([]string{"xin ", "chao"})
	sclient := connect.NewClient[gosov1.GenerateReplyRequest, gosov1.StreamReplyChunk](
		h2,
		base+procStream,
		connect.WithGRPC(),
	)
	sreq := connect.NewRequest(&gosov1.GenerateReplyRequest{
		SessionId: agentID,
		TenantId:  tenant,
		Model:     "gpt-4o-mini",
		Prompt:    "stream",
	})
	stamp(sreq.Header(), svc.Secret, tenant, []byte("stream"))
	stream, err := sclient.CallServerStream(context.Background(), sreq)
	if err != nil {
		t.Fatal(err)
	}
	var streamed string
	finished := false
	for stream.Receive() {
		streamed += stream.Msg().GetDelta()
		if stream.Msg().GetIsFinished() {
			finished = true
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if streamed != "xin chao" || !finished {
		t.Fatalf("stream %q finished=%v", streamed, finished)
	}

	hclient := connect.NewClient[gosov1.HealthRequest, gosov1.HealthResponse](
		h2,
		base+procHealth,
		connect.WithGRPC(),
	)
	hres, err := hclient.CallUnary(context.Background(), connect.NewRequest(&gosov1.HealthRequest{}))
	if err != nil || !hres.Msg.GetHealthy() || hres.Msg.GetVersion() != "test" {
		t.Fatalf("health %v %+v", err, hres)
	}
	health, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := io.ReadAll(health.Body)
	_ = health.Body.Close()
	if health.StatusCode != 200 || !strings.Contains(string(hb), `"healthy":true`) {
		t.Fatalf("healthz %d %s", health.StatusCode, hb)
	}

	del := doSigned(t, http.MethodDelete, base+"/v1/tenants/"+tenant+"/agents/"+agentID, svc.Secret, tenant, nil)
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete %d %s", del.StatusCode, del.Body)
	}
	missing := doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete %d %s", missing.StatusCode, missing.Body)
	}
}

func TestResolveByModelAndZenDefault(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_DEFAULT_API_KEY", "")
	t.Setenv("GOSO_DEFAULT_BASE_URL", "")
	t.Setenv("GOSO_DEFAULT_MODEL", "")
	t.Setenv("GOSO_LLM_API_KEY", "")
	t.Setenv("GOSO_LLM_BASE_URL", "")
	t.Setenv("GOSO_LLM_MODEL", "")
	t.Setenv("GOSO_LLM_PROVIDER", "")
	if got := normalizeBase("opencode", ""); got != OpenCodeZenBase {
		t.Fatalf("opencode base %s", got)
	}
	if got := normalizeBase("zen", ""); got != OpenCodeZenBase {
		t.Fatalf("zen base %s", got)
	}
	if got := normalizeBase("openai", OpenCodeZenBase); got != OpenCodeZenBase {
		t.Fatalf("custom base %s", got)
	}
	cat := newMem()
	svc := &Service{Catalog: cat}
	const tenant = "00000000-0000-0000-0000-000000000009"
	if _, err := cat.EnsureTenant(tenant); err != nil {
		t.Fatal(err)
	}
	if err := cat.PutProvider(Provider{
		TenantID: tenant, ID: "oa", Name: "oa", Type: "openai",
		BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4o-mini"}, Active: true, APIKey: "k1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cat.PutProvider(Provider{
		TenantID: tenant, ID: "zen", Name: "zen", Type: "openai",
		BaseURL: OpenCodeZenBase, Models: []string{OpenCodeZenPro, OpenCodeZenFlash}, Active: true, APIKey: "k2",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.resolveProvider(tenant, "", OpenCodeZenFlash)
	if err != nil || p.ID != "zen" || p.APIKey != "k2" {
		t.Fatalf("flash provider id=%s err=%v", p.ID, err)
	}
	p, err = svc.resolveProvider(tenant, "", "gpt-4o-mini")
	if err != nil || p.ID != "oa" {
		t.Fatalf("mini provider id=%s err=%v", p.ID, err)
	}
	t.Setenv("GOSO_DEFAULT_API_KEY", "")
	t.Setenv("GOSO_LLM_API_KEY", "sk-llm")
	t.Setenv("GOSO_LLM_PROVIDER", "opencode")
	t.Setenv("GOSO_LLM_BASE_URL", OpenCodeZenBase)
	t.Setenv("GOSO_LLM_MODEL", OpenCodeZenFlash)
	p, err = svc.resolveProvider("tenant-without-providers", "", OpenCodeZenFlash)
	if err != nil || p.Type != "opencode" || p.BaseURL != OpenCodeZenBase || p.APIKey != "sk-llm" || len(p.Models) == 0 || p.Models[0] != OpenCodeZenFlash {
		t.Fatalf("llm env base=%s type=%s model0=%v keyset=%v err=%v", p.BaseURL, p.Type, p.Models, p.APIKey != "", err)
	}
}

func TestUpstream402And429(t *testing.T) {
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	t.Setenv("GOSO_DEFAULT_API_KEY", "")
	status := http.StatusPaymentRequired
	var sawAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":""}}]}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"message":"billing sk-should-not-leak"}}`))
	}))
	defer upstream.Close()

	cat := newMem()
	svc := &Service{Secret: "test-secret", Version: "test", Catalog: cat, Store: store.New()}
	addr := startBridge(t, svc)
	base := "http://" + addr
	const (
		tenant  = "00000000-0000-0000-0000-000000000001"
		agentID = "agent-zen"
	)
	provBody := []byte(`{"name":"other","provider_type":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-other","models":["gpt-4o-mini"],"is_active":true}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/other", svc.Secret, tenant, provBody); res.StatusCode != 200 {
		t.Fatalf("other provider %d %s", res.StatusCode, res.Body)
	}
	zenBody := []byte(`{"name":"zen","provider_type":"openai","base_url":"` + upstream.URL + `/v1","api_key":"sk-zen","models":["` + OpenCodeZenPro + `","` + OpenCodeZenFlash + `"],"is_active":true}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/zen", svc.Secret, tenant, zenBody); res.StatusCode != 200 {
		t.Fatalf("zen provider %d %s", res.StatusCode, res.Body)
	}
	// Core agents have no provider_id; the model name selects the provider.
	agentBody := []byte(`{"name":"ZZTEST-agent","description":"probe","harness_type":"GOSO","model_name":"` + OpenCodeZenFlash + `","system_prompt":"Ban la tro ly","temperature":0.2,"daily_reply_cap":5}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/agents/"+agentID, svc.Secret, tenant, agentBody); res.StatusCode != 200 {
		t.Fatalf("agent %d %s", res.StatusCode, res.Body)
	}
	replyBody := []byte(`{"agent_id":"` + agentID + `","model":"` + OpenCodeZenFlash + `","prompt":"xin chao"}`)

	status = http.StatusPaymentRequired
	res := doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if res.StatusCode != http.StatusPaymentRequired || !strings.Contains(res.Body, `"code":"insufficient_funds"`) || !strings.Contains(res.Body, `"upstream_status":402`) {
		t.Fatalf("402 %d %s", res.StatusCode, res.Body)
	}
	if strings.Contains(res.Body, "sk-zen") || strings.Contains(res.Body, "sk-should-not-leak") || strings.Contains(res.Body, `"reply_text"`) {
		t.Fatalf("402 leaked or looked like success: %s", res.Body)
	}
	if sawAuth != "Bearer sk-zen" {
		t.Fatalf("auth %q", sawAuth)
	}

	status = http.StatusTooManyRequests
	res = doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if res.StatusCode != http.StatusTooManyRequests || !strings.Contains(res.Body, `"code":"rate_limited"`) || !strings.Contains(res.Body, `"upstream_status":429`) {
		t.Fatalf("429 %d %s", res.StatusCode, res.Body)
	}

	status = http.StatusUnauthorized
	res = doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(res.Body, `"code":"upstream_unauthorized"`) {
		t.Fatalf("401 %d %s", res.StatusCode, res.Body)
	}

	status = http.StatusForbidden
	res = doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if res.StatusCode != http.StatusForbidden || !strings.Contains(res.Body, `"code":"upstream_forbidden"`) {
		t.Fatalf("403 %d %s", res.StatusCode, res.Body)
	}

	status = http.StatusOK
	res = doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, replyBody)
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(res.Body, `"code":"empty_reply"`) {
		t.Fatalf("empty %d %s", res.StatusCode, res.Body)
	}

	status = http.StatusPaymentRequired
	h2 := h2cHTTPClient()
	gclient := connect.NewClient[gosov1.GenerateReplyRequest, gosov1.GenerateReplyResponse](h2, base+procGenerate, connect.WithGRPC())
	greq := connect.NewRequest(&gosov1.GenerateReplyRequest{
		SessionId: agentID, TenantId: tenant, Model: OpenCodeZenFlash, Prompt: "grpc",
	})
	stamp(greq.Header(), svc.Secret, tenant, []byte("grpc"))
	_, err = gclient.CallUnary(context.Background(), greq)
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeFailedPrecondition || !strings.Contains(ce.Message(), "insufficient_funds") {
		t.Fatalf("grpc 402 %v", err)
	}

	status = http.StatusTooManyRequests
	sclient := connect.NewClient[gosov1.GenerateReplyRequest, gosov1.StreamReplyChunk](h2, base+procStream, connect.WithGRPC())
	sreq := connect.NewRequest(&gosov1.GenerateReplyRequest{
		SessionId: agentID, TenantId: tenant, Model: OpenCodeZenPro, Prompt: "stream",
	})
	stamp(sreq.Header(), svc.Secret, tenant, []byte("stream"))
	stream, err := sclient.CallServerStream(context.Background(), sreq)
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if !errors.As(err, &ce) || ce.Code() != connect.CodeResourceExhausted || !strings.Contains(ce.Message(), "rate_limited") {
		t.Fatalf("grpc stream 429 %v", err)
	}
}

func TestKnowledgeSync(t *testing.T) {
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	t.Setenv("GOSO_LLM_API_KEY", "")
	t.Setenv("GOSO_DEFAULT_API_KEY", "")
	cat := newMem()
	fake := &fakeLLM{text: "da ro"}
	svc := &Service{Secret: "test-secret", Version: "test", Catalog: cat, Store: store.New(), Complete: fake.Complete}
	addr := startBridge(t, svc)
	base := "http://" + addr
	const (
		tenant  = "00000000-0000-0000-0000-000000000001"
		agentID = "agent-know"
	)
	prov := []byte(`{"name":"zen","provider_type":"opencode","base_url":"https://opencode.ai/zen/v1","api_key":"sk-zen","models":["deepseek-v4-pro","deepseek-v4.1-flash"],"is_active":true}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/zen", svc.Secret, tenant, prov); res.StatusCode != 200 {
		t.Fatalf("provider %d %s", res.StatusCode, res.Body)
	}
	agent := []byte(`{"id":"` + agentID + `","tenant_id":"` + tenant + `","name":"ZZTEST-agent","description":"probe","harness_type":"GOSO","model_name":"deepseek-v4-pro","system_prompt":"Ban la tro ly","temperature":0.2,"daily_reply_cap":5,"status":"ACTIVE","harness_config_json":""}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/agents/"+agentID, svc.Secret, tenant, agent); res.StatusCode != 200 {
		t.Fatalf("agent %d %s", res.StatusCode, res.Body)
	}
	know := []byte(`{"tenant_id":"` + tenant + `","agent_id":"` + agentID + `","documents":[{"id":"f1","file_name":"SOUL.md","mime_type":"text/markdown","file_size":18,"content":"Gia cua hang la 10"}]}`)
	req, err := http.NewRequest(http.MethodPut, base+"/v1/tenants/"+tenant+"/agents/"+agentID+"/knowledge", bytes.NewReader(know))
	if err != nil {
		t.Fatal(err)
	}
	stamp(req.Header, svc.Secret, tenant, know)
	req.Header.Set(headerAgent, agentID)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"status":"SYNCED"`) || strings.Contains(string(body), "Gia cua hang") {
		t.Fatalf("knowledge %d %s", res.StatusCode, body)
	}
	reply := []byte(`{"tenant_id":"` + tenant + `","agent_id":"` + agentID + `","model":"deepseek-v4-pro","system_prompt":"Ban la tro ly","temperature":0.2,"prompt":"gia?"}`)
	got := doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, reply)
	if got.StatusCode != 200 {
		t.Fatalf("reply %d %s", got.StatusCode, got.Body)
	}
	last := fake.snapshot()
	if last.Type != "opencode" || last.BaseURL != OpenCodeZenBase || last.Model != OpenCodeZenPro {
		t.Fatalf("routed type=%s base=%s model=%s", last.Type, last.BaseURL, last.Model)
	}
	if len(last.Messages) == 0 || !strings.Contains(last.Messages[0].Content, "Gia cua hang la 10") || !strings.Contains(last.Messages[0].Content, "Ban la tro ly") {
		t.Fatalf("system prompt missing knowledge: %+v", last.Messages)
	}
}

func TestListenAddr(t *testing.T) {
	t.Setenv("GOSO_PORT", "")
	t.Setenv("GOSO_HOST", "")
	if ListenAddr("", -1) != "127.0.0.1:8088" {
		t.Fatal(ListenAddr("", -1))
	}
	t.Setenv("GOSO_PORT", "9090")
	t.Setenv("GOSO_HOST", "0.0.0.0")
	if ListenAddr("", -1) != "0.0.0.0:9090" {
		t.Fatal(ListenAddr("", -1))
	}
}

type signedResult struct {
	StatusCode int
	Body       string
}

func doSigned(t *testing.T, method, url, secret, tenant string, body []byte) signedResult {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	stamp(req.Header, secret, tenant, body)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return signedResult{StatusCode: res.StatusCode, Body: string(b)}
}

func stamp(h http.Header, secret, tenant string, payload []byte) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h.Set(headerTenant, tenant)
	h.Set(headerTimestamp, ts)
	h.Set(headerSignature, sign(secret, ts, payload))
}

func startBridge(t *testing.T, svc *Service) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler:           h2c.NewHandler(NewMux(svc), &http2.Server{}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return ln.Addr().String()
}

func h2cHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}

type fakeLLM struct {
	mu     sync.Mutex
	n      int
	last   Completion
	text   string
	deltas []string
}

func (f *fakeLLM) Complete(_ context.Context, in Completion) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.last = in
	if in.OnDelta != nil {
		for _, d := range f.deltas {
			in.OnDelta(d)
		}
	}
	text := f.text
	if len(f.deltas) > 0 {
		text = strings.Join(f.deltas, "")
	}
	return Result{Text: text, PromptTokens: 4, CompletionTokens: 2, StopReason: "stop"}, nil
}

func (f *fakeLLM) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func (f *fakeLLM) snapshot() Completion {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

func (f *fakeLLM) setDeltas(d []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deltas = d
}
