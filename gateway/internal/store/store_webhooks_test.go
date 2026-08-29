// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
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

func TestStore_WebhookEndpointAndLatestJob(t *testing.T) {
	for _, s := range []StoreIface{New(), mustSQLiteWebhooks(t)} {
		w, err := s.CreateWebhook(Webhook{TokenPrefix: "wh_end", TokenHash: "hash-end", Endpoint: "http://127.0.0.1:9/hooks"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.GetWebhook(w.ID)
		if err != nil || got.Endpoint != "http://127.0.0.1:9/hooks" {
			t.Fatalf("endpoint %#v %v", got, err)
		}
		if _, err := s.LatestWebhookJob(w.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("latest empty %v", err)
		}
		first, err := s.CreateWebhookJob(WebhookJob{WebhookID: w.ID, Status: WebhookDone, CallbackURL: "http://127.0.0.1:9/hooks"})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
		second, err := s.CreateWebhookJob(WebhookJob{WebhookID: w.ID, Status: WebhookFailed, CallbackURL: "http://127.0.0.1:9/hooks"})
		if err != nil {
			t.Fatal(err)
		}
		latest, err := s.LatestWebhookJob(w.ID)
		if err != nil || latest.ID != second.ID {
			t.Fatalf("latest %#v want %s %v", latest, second.ID, err)
		}
		if latest.ID == first.ID {
			t.Fatal("stale job")
		}
		got.Endpoint = "http://127.0.0.1:9/rotated"
		upd, err := s.UpdateWebhook(*got)
		if err != nil || upd.Endpoint != "http://127.0.0.1:9/rotated" {
			t.Fatalf("update endpoint %#v %v", upd, err)
		}
	}
}

func mustSQLiteWebhooks(t *testing.T) StoreIface {
	t.Helper()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "wh-ops.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
