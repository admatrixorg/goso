// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func (t *Telegram) startLive(ctx context.Context, mgr *Manager) {
	if store.LiteEnabled() {
		return
	}
	token := strings.TrimSpace(t.BotToken)
	if token == "" {
		token = os.Getenv("GOSO_TELEGRAM_BOT_TOKEN")
	}
	if token == "" {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_TELEGRAM_MODE")))
	if mode == "" {
		mode = "poll"
	}
	if mode == "webhook" {
		pub := strings.TrimSpace(os.Getenv("GOSO_PUBLIC_URL"))
		if pub == "" {
			if mgr != nil {
				mgr.SetFailed("telegram", "public url required for webhook mode")
			}
			return
		}
	}
	if err := t.getMe(ctx, token); err != nil {
		if mgr != nil {
			mgr.SetFailed("telegram", err.Error())
		}
		return
	}
	if mgr != nil {
		mgr.SetRunning("telegram", mode)
	}
	if mode == "poll" {
		t.pollCancelMu.Lock()
		if t.pollStop != nil {
			t.pollCancelMu.Unlock()
			return
		}
		cctx, cancel := context.WithCancel(ctx)
		t.pollStop = cancel
		t.pollCancelMu.Unlock()
		go t.pollLoop(cctx, token)
	}
}

func (t *Telegram) stopLive() {
	t.pollCancelMu.Lock()
	defer t.pollCancelMu.Unlock()
	if t.pollStop != nil {
		t.pollStop()
		t.pollStop = nil
	}
}

func (t *Telegram) getMe(ctx context.Context, token string) error {
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/bot"+token+"/getMe", nil)
	if err != nil {
		return err
	}
	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("getMe %d", resp.StatusCode)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(b, &out)
	if !out.OK {
		return fmt.Errorf("getMe not ok")
	}
	return nil
}

func (t *Telegram) pollLoop(ctx context.Context, token string) {
	ticker := time.NewTicker(t.probeEvery)
	if t.probeEvery <= 0 {
		ticker.Stop()
		ticker = time.NewTicker(5 * time.Minute)
	}
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.getMe(ctx, token); err != nil && t.onProbeFail != nil {
				t.onProbeFail(err)
			}
		}
	}
}
