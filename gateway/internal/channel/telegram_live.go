// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
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
	token := t.resolveToken()
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
	if mode == "webhook" {
		pub := strings.TrimSpace(os.Getenv("GOSO_PUBLIC_URL"))
		if err := t.setWebhook(ctx, token, strings.TrimRight(pub, "/")+"/api/channels/telegram/webhook"); err != nil {
			if mgr != nil {
				mgr.SetFailed("telegram", err.Error())
			}
			return
		}
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
		go t.pollUpdates(cctx, token)
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

func (t *Telegram) setWebhook(ctx context.Context, token, hookURL string) error {
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	body, _ := json.Marshal(map[string]any{"url": hookURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/bot"+token+"/setWebhook", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
		return fmt.Errorf("setWebhook %d", resp.StatusCode)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(b, &out)
	if !out.OK {
		return fmt.Errorf("setWebhook not ok")
	}
	return nil
}

func (t *Telegram) pollUpdates(ctx context.Context, token string) {
	var offset int64
	for {
		if ctx.Err() != nil {
			return
		}
		updates, err := t.getUpdates(ctx, token, offset)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			t.dispatchUpdate(ctx, u)
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

func (t *Telegram) getUpdates(ctx context.Context, token string, offset int64) ([]TelegramUpdate, error) {
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	u := fmt.Sprintf("%s/bot%s/getUpdates?timeout=1&offset=%d", strings.TrimRight(base, "/"), token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("getUpdates %d", resp.StatusCode)
	}
	var out struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	if err := json.Unmarshal(b, &out); err != nil || !out.OK {
		return nil, fmt.Errorf("getUpdates decode")
	}
	return out.Result, nil
}

func (t *Telegram) dispatchUpdate(ctx context.Context, upd TelegramUpdate) {
	_ = t.ingest(ctx, upd)
}
