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

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ZaloOA handles Zalo Official Account webhook.
type ZaloOA struct {
	AccessToken string
	Store       store.StoreIface
	LLM         llm.Provider
	Meter       *billing.Store
	Sender      func(ctx context.Context, userID string, text string) error
	// HTTPClient downloads received attachments (nil: a default client).
	HTTPClient *http.Client
}

func (z *ZaloOA) Name() string { return "zalo-oa" }

// ZaloOAUpdate is a flexible OA webhook payload.
type ZaloOAUpdate struct {
	EventName string `json:"event_name"`
	Sender    *struct {
		ID string `json:"id"`
	} `json:"sender"`
	UserID  string `json:"user_id"`
	Message *struct {
		Text        string             `json:"text"`
		Attachments []ZaloOAAttachment `json:"attachments"`
	} `json:"message"`
}

// ZaloOAAttachment is one attachment of an OA message (user_send_image,
// user_send_file, user_send_audio, user_send_video, user_send_gif,
// user_send_sticker). Its payload URL is a public CDN link.
type ZaloOAAttachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL       string `json:"url"`
		Thumbnail string `json:"thumbnail"`
		Name      string `json:"name"`
		Size      any    `json:"size"`
		Type      string `json:"type"`
		ID        string `json:"id"`
	} `json:"payload"`
}

// zaloOAAttachments maps OA attachments. A sticker becomes a note: it is
// a picture with no words, so it is not downloaded.
func zaloOAAttachments(list []ZaloOAAttachment) []Attachment {
	var out []Attachment
	for _, a := range list {
		ref := strings.TrimSpace(a.Payload.URL)
		var kind string
		switch strings.ToLower(a.Type) {
		case "image", "gif":
			kind = MediaImage
			if ref == "" {
				ref = strings.TrimSpace(a.Payload.Thumbnail)
			}
		case "file":
			kind = MediaDocument
		case "audio", "voice":
			kind = MediaVoice
		case "video":
			kind = MediaVideo
		case "sticker":
			out = append(out, Attachment{Kind: MediaSticker, Note: "[sticker]"})
			continue
		default:
			continue
		}
		if ref == "" {
			continue
		}
		out = append(out, Attachment{Kind: kind, Name: a.Payload.Name, Size: anySize(a.Payload.Size), Ref: ref})
	}
	return out
}

// anySize reads a size that platforms send as a number or a string.
func anySize(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		var x int64
		for _, c := range strings.TrimSpace(n) {
			if c < '0' || c > '9' {
				return 0
			}
			x = x*10 + int64(c-'0')
			if x > 1<<40 {
				return 0
			}
		}
		return x
	}
	return 0
}

func (z *ZaloOA) fetchAttachment(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
	return fetchURL(ctx, z.HTTPClient, a.Ref, limit, "", "")
}

func oaWebhookAuthorized(r *http.Request, sec string) bool {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_ENV")))
	if sec == "" {
		if env == "production" {
			return false
		}
		return true
	}
	got := r.Header.Get("X-Goso-OA-Secret")
	if got == "" {
		got = r.URL.Query().Get("secret")
	}
	return got == sec
}

func (z *ZaloOA) resolveAccess() string {
	if z == nil {
		return ""
	}
	if v := strings.TrimSpace(z.AccessToken); v != "" {
		return v
	}
	v, _, set := Credential(z.Store, "zalo-oa", KindAccess, []string{"GOSO_ZALO_OA_ACCESS_TOKEN"})
	if set {
		return v
	}
	return ""
}

func (z *ZaloOA) resolveSecret() string {
	if z == nil {
		return ""
	}
	v, _, set := Credential(z.Store, "zalo-oa", KindAppSecret, []string{"GOSO_ZALO_OA_SECRET"})
	if set {
		return v
	}
	return ""
}

func (z *ZaloOA) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if !oaWebhookAuthorized(r, z.resolveSecret()) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var upd ZaloOAUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	userID := upd.UserID
	if upd.Sender != nil && upd.Sender.ID != "" {
		userID = upd.Sender.ID
	}
	var text string
	var atts []Attachment
	if upd.Message != nil {
		text = upd.Message.Text
		atts = zaloOAAttachments(upd.Message.Attachments)
	}
	if userID == "" || (text == "" && len(atts) == 0) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}

	var cfg *store.ChannelConfig
	if z.Store != nil {
		cfg, _ = z.Store.GetChannelConfig("zalo-oa")
	}
	pol := MergePolicy("zalo-oa", cfg)
	paired := false
	if z.Store != nil {
		paired = SenderPaired(z.Store, "zalo-oa", userID, time.Time{})
	}
	in := Inbound{Channel: "zalo-oa", SenderID: userID, ChatID: userID, PeerKind: "direct", Text: text}
	switch CheckPolicy("zalo-oa", pol, in, paired) {
	case PolicyReject, PolicyNeedMention:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	case PolicyNeedPairing:
		if msg := OfferPairingCode(z.Store, "zalo-oa", userID, time.Time{}); msg != "" {
			sender := z.Sender
			if sender == nil {
				sender = z.sendMessage
			}
			_ = sender(r.Context(), userID, msg)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}

	agent := z.ensureAgent()
	sess := z.ensureSession(agent.ID, userID)

	ctx := withContextUser(r.Context(), "zalo-oa", "direct", userID, userID)
	ctx, text = withInboundMedia(ctx, agent, "zalo-oa", text, atts, z.fetchAttachment)
	reply := answerTurn(ctx, z.Store, agent, sess, z.LLM, z.Meter, text)

	sender := z.Sender
	if sender == nil {
		sender = z.sendMessage
	}
	if err := sender(r.Context(), userID, reply); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (z *ZaloOA) ensureAgent() *store.Agent {
	if a := store.FindAgentByKey(z.Store, store.DefaultTenant, "zalo-oa"); a != nil {
		return a
	}
	a, _ := z.Store.CreateAgent(store.Agent{TenantID: store.DefaultTenant, AgentKey: "zalo-oa", DisplayName: "Zalo OA"})
	return a
}

func (z *ZaloOA) ensureSession(agentID, userID string) *store.Session {
	label := "zalo-oa:" + userID
	for _, s := range z.Store.ListSessions() {
		if s.AgentID == agentID && s.Label == label {
			return s
		}
	}
	sess, _ := z.Store.CreateSession(store.Session{AgentID: agentID, Label: label})
	return sess
}

func (z *ZaloOA) sendMessage(ctx context.Context, userID, text string) error {
	token := z.resolveAccess()
	if token == "" {
		return fmt.Errorf("zalo-oa: missing access token")
	}
	body, _ := json.Marshal(map[string]any{
		"recipient": map[string]string{"user_id": userID},
		"message":   map[string]string{"text": text},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://openapi.zalo.me/v3.0/oa/message/cs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("zalo oa send %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
