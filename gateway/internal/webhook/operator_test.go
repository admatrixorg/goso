// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestNormalizeEndpoint(t *testing.T) {
	got, err := NormalizeEndpoint("https://user:secret@example.invalid/hook")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "user:") {
		t.Fatalf("userinfo leaked: %s", got)
	}
	if _, err := NormalizeEndpoint("javascript:alert(1)"); err != ErrInvalidEndpoint {
		t.Fatalf("js scheme %v", err)
	}
	empty, err := NormalizeEndpoint("  ")
	if err != nil || empty != "" {
		t.Fatalf("empty %q %v", empty, err)
	}
}

func TestRegistry_ListOperatorIncludesRevokedAndNoSecrets(t *testing.T) {
	r := New()
	c, err := r.CreateOpts(CreateOpts{Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(c.ID); err != nil {
		t.Fatal(err)
	}
	ops := r.ListOperator()
	if len(ops) != 1 || ops[0].Status != "revoked" || ops[0].Endpoint != InboundPath {
		t.Fatalf("operator %+v", ops)
	}
	if ops[0].SecretSet != true {
		t.Fatal("secret_set")
	}
	raw, _ := json.Marshal(ops)
	if strings.Contains(string(raw), c.Token) || strings.Contains(string(raw), c.HMACKey) {
		t.Fatalf("secrets in operator list %s", raw)
	}
	if strings.Contains(string(raw), `"token":`) || strings.Contains(string(raw), `"hmac_key"`) {
		t.Fatalf("secret fields %s", raw)
	}
	if len(r.List()) != 0 {
		t.Fatal("auth list should skip revoked")
	}
}

func TestRegistry_TestAndReplayRedactPayload(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_SSRF", "")
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if r.Header.Get("X-Goso-Signature") == "" {
			t.Error("missing signature")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := New()
	c, err := r.CreateOpts(CreateOpts{Endpoint: srv.URL, Name: "out"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Test(c.ID)
	if err != nil || d == nil || d.Status != store.WebhookDone {
		t.Fatalf("test %#v %v", d, err)
	}
	if len(bodies) != 1 || !strings.Contains(bodies[0], `"event":"webhook.test"`) {
		t.Fatalf("test body %v", bodies)
	}
	if strings.Contains(bodies[0], c.Token) || strings.Contains(bodies[0], c.HMACKey) {
		t.Fatalf("secret in test payload %s", bodies[0])
	}
	if strings.Contains(bodies[0], `"input"`) || strings.Contains(bodies[0], `"reply"`) || strings.Contains(bodies[0], `"hmac_key"`) {
		t.Fatalf("payload fields %s", bodies[0])
	}

	_, err = r.st.CreateWebhookJob(store.WebhookJob{
		WebhookID:   c.ID,
		Status:      store.WebhookDone,
		Input:       "super-secret-prompt " + c.Token,
		Reply:       "secret-reply " + c.HMACKey,
		CallbackURL: srv.URL,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rd, err := r.Replay(c.ID, "")
	if err != nil || rd == nil {
		t.Fatalf("replay %#v %v", rd, err)
	}
	if len(bodies) < 2 {
		t.Fatalf("replay not posted %v", bodies)
	}
	last := bodies[len(bodies)-1]
	if !strings.Contains(last, `"event":"webhook.replay"`) {
		t.Fatalf("replay body %s", last)
	}
	if strings.Contains(last, "super-secret-prompt") || strings.Contains(last, "secret-reply") || strings.Contains(last, c.Token) || strings.Contains(last, c.HMACKey) {
		t.Fatalf("secret-bearing replay %s", last)
	}

	pub, err := r.GetOperator(c.ID)
	if err != nil || pub.LastDelivery == nil || pub.LastDelivery.ID == "" {
		t.Fatalf("last delivery %#v %v", pub, err)
	}
	raw, _ := json.Marshal(pub)
	if strings.Contains(string(raw), c.Token) || strings.Contains(string(raw), "super-secret-prompt") {
		t.Fatalf("get leaked %s", raw)
	}
}

func TestRegistry_TestRequiresEndpoint(t *testing.T) {
	r := New()
	c, err := r.Create()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Test(c.ID); err != ErrNoEndpoint {
		t.Fatalf("want no endpoint got %v", err)
	}
}
