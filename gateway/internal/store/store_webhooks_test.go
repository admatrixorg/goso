// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_WebhookMemoryAndSQLite(t *testing.T) {
	s := New()
	w, err := s.CreateWebhook(Webhook{TokenPrefix: "wh_abc", TokenHash: "hash1", Kind: "llm"})
	if err != nil || w.ID == "" {
		t.Fatalf("create %#v %v", w, err)
	}
	got, err := s.GetWebhook(w.ID)
	if err != nil || got.TokenHash != "hash1" {
		t.Fatalf("get %#v %v", got, err)
	}
	job, err := s.CreateWebhookJob(WebhookJob{WebhookID: w.ID, Status: WebhookQueued, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimWebhookJob(time.Now().UTC(), "lease-1")
	if err != nil || claimed.ID != job.ID || claimed.Status != WebhookRunning {
		t.Fatalf("claim %#v %v", claimed, err)
	}
}

func TestSQLiteStore_WebhookPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wh.db")
	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s1.CreateWebhook(Webhook{Name: "n", TokenPrefix: "wh_xyz", TokenHash: "abc", HMACEnc: "enc"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s1.CreateWebhookJob(WebhookJob{WebhookID: w.ID, Status: WebhookQueued, Input: "x", IdempotencyKey: "k", BodyHash: "h"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()

	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetWebhook(w.ID)
	if err != nil || got.TokenHash != "abc" || got.HMACEnc != "enc" || got.Name != "n" {
		t.Fatalf("persist webhook %#v %v", got, err)
	}
	j, err := s2.GetWebhookJobByIdempotency(w.ID, "k")
	if err != nil || j.BodyHash != "h" {
		t.Fatalf("persist job %#v %v", j, err)
	}
}
