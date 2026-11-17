// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Text limits of one message on each platform.
const (
	discordMaxText  = 2000
	slackMaxText    = 4000
	whatsappMaxText = 4096
)

const (
	discordAPIBase  = "https://discord.com/api/v10"
	slackAPIBase    = "https://slack.com/api"
	whatsappAPIBase = "https://graph.facebook.com/v21.0"
)

// discordSend posts text into a Discord channel, in pieces of at most
// discordMaxText characters.
func discordSend(ctx context.Context, client *http.Client, base, token, channelID, text string) error {
	if base == "" {
		base = discordAPIBase
	}
	u := strings.TrimRight(base, "/") + "/channels/" + url.PathEscape(channelID) + "/messages"
	for _, part := range splitRunes(text, discordMaxText) {
		if err := postJSON(ctx, client, u, "Authorization", "Bot "+token, map[string]any{"content": part}); err != nil {
			return err
		}
	}
	return nil
}

// slackSend posts text into a Slack channel, inside the thread threadTS when
// it is set. Slack answers HTTP 200 with "ok": false on a refusal.
func slackSend(ctx context.Context, client *http.Client, base, token, channel, threadTS, text string) error {
	if base == "" {
		base = slackAPIBase
	}
	payload := map[string]any{"channel": channel, "text": text}
	if threadTS != "" {
		payload["thread_ts"] = threadTS
	}
	body, err := postJSONBody(ctx, client, strings.TrimRight(base, "/")+"/chat.postMessage", "Bearer "+token, payload)
	if err != nil {
		return err
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &res) != nil {
		return fmt.Errorf("slack: unreadable answer")
	}
	if !res.OK {
		return fmt.Errorf("slack: %s", res.Error)
	}
	return nil
}

// whatsappSend sends a text message through the WhatsApp Cloud API from the
// business number phoneNumberID to the number to.
func whatsappSend(ctx context.Context, client *http.Client, base, token, phoneNumberID, to, text string) error {
	if base == "" {
		base = whatsappAPIBase
	}
	if phoneNumberID == "" {
		phoneNumberID = "me"
	}
	u := strings.TrimRight(base, "/") + "/" + url.PathEscape(phoneNumberID) + "/messages"
	return postJSON(ctx, client, u, "Authorization", "Bearer "+token, map[string]any{
		"messaging_product": "whatsapp",
		"to":                whatsappNumber(to),
		"type":              "text",
		"text":              map[string]string{"body": text},
	})
}

// postJSONBody is postJSON that also returns the answer body.
func postJSONBody(ctx context.Context, client *http.Client, u, auth string, payload any) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("send %d", resp.StatusCode)
	}
	return body, nil
}

type slackThreadKey struct{}

// withSlackThread makes a Slack send post into the thread ts; "" leaves ctx
// as it is.
func withSlackThread(ctx context.Context, ts string) context.Context {
	if ts == "" {
		return ctx
	}
	return context.WithValue(ctx, slackThreadKey{}, ts)
}

func slackThreadFrom(ctx context.Context) string {
	v, _ := ctx.Value(slackThreadKey{}).(string)
	return v
}
