package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ThreadType picks the direction a message goes.
type ThreadType string

const (
	ThreadUser  ThreadType = "user"
	ThreadGroup ThreadType = "group"
)

// Service endpoints Zalo returns from getLoginInfo.
type ServiceMap struct {
	Chat      []string `json:"chat"`
	Group     []string `json:"group"`
	File      []string `json:"file"`
	Profile   []string `json:"profile"`
	GroupPoll []string `json:"group_poll"`
}

// SendResult is the outcome of one send.
type SendResult struct {
	OK        bool   `json:"ok"`
	MessageID string `json:"message_id,omitempty"`
	ErrorCode int    `json:"error_code"`
	Raw       string `json:"raw,omitempty"`
}

// SendMessage posts a text message and decrypts the reply envelope.
func (s *Session) SendMessage(ctx context.Context, threadID string, kind ThreadType, text string) (*SendResult, error) {
	if s == nil || s.HTTP == nil {
		return nil, fmt.Errorf("zalo personal: session is not ready")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("zalo personal: thread id is required")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("zalo personal: message text is required")
	}
	if strings.TrimSpace(s.SecretKey) == "" {
		return nil, fmt.Errorf("zalo personal: secret key is required to encrypt the payload")
	}
	serviceKey, apiPath := "chat", "/api/message/sms"
	if kind == ThreadGroup {
		serviceKey, apiPath = "group", "/api/group/sendmsg"
	}
	base := s.ServiceURL(serviceKey)
	if base == "" {
		return nil, fmt.Errorf("zalo personal: login info carries no %s service url", serviceKey)
	}
	payload := map[string]any{
		"message":  text,
		"clientId": time.Now().UnixMilli(),
		"ttl":      0,
	}
	if kind == ThreadGroup {
		payload["grid"] = threadID
		payload["visibility"] = 0
	} else {
		payload["toid"] = threadID
		payload["imei"] = s.IMEI
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	key, kerr := DecodeSecretKey(s.SecretKey)
	if kerr != nil {
		return nil, kerr
	}
	encData, eerr := EncodeAESCBC(key, string(blob), false)
	if eerr != nil {
		return nil, fmt.Errorf("zalo personal: encrypt payload: %w", eerr)
	}
	sendURL := s.BuildURL(base+apiPath, map[string]string{"nretry": "0"})
	form := url.Values{}
	form.Set("params", encData)
	req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	if rerr != nil {
		return nil, rerr
	}
	s.Decorate(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, derr := s.HTTP.Do(req)
	if derr != nil {
		return nil, fmt.Errorf("zalo personal: send message: %w", derr)
	}
	defer resp.Body.Close()
	body, _ := ReadBody(resp, 1<<20)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("zalo personal: send http %d", resp.StatusCode)
	}
	var envelope struct {
		ErrorCode    int             `json:"error_code"`
		ErrorMessage string          `json:"error_message"`
		Data         json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal: send reply is not json")
	}
	out := &SendResult{ErrorCode: envelope.ErrorCode, Raw: clip(string(body), 300)}
	if envelope.ErrorCode != 0 {
		return out, fmt.Errorf("zalo personal: send rejected: %s", firstNonEmpty(envelope.ErrorMessage, "error "+strconv.Itoa(envelope.ErrorCode)))
	}
	if len(envelope.Data) > 0 {
		var encReply string
		if json.Unmarshal(envelope.Data, &encReply) == nil && encReply != "" {
			if plain, derr := DecodeAESCBC(key, encReply); derr == nil {
				out.MessageID = strings.Trim(string(plain), "\"")
			}
		}
	}
	out.OK = true
	return out, nil
}

// ServiceURL resolves a service host from the login info.
func (s *Session) ServiceURL(key string) string {
	if s == nil || s.Services == nil {
		return ""
	}
	var list []string
	switch key {
	case "chat":
		list = s.Services.Chat
	case "group":
		list = s.Services.Group
	case "file":
		list = s.Services.File
	case "profile":
		list = s.Services.Profile
	case "group_poll":
		list = s.Services.GroupPoll
	}
	if len(list) == 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(list[0]), "/")
}

// BuildURL adds the query params Zalo expects on every API call.
func (s *Session) BuildURL(base string, params map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range params {
		if !q.Has(k) {
			q.Set(k, v)
		}
	}
	if !q.Has("zpw_ver") {
		q.Set("zpw_ver", strconv.Itoa(DefaultAPIVersion))
	}
	if !q.Has("zpw_type") {
		q.Set("zpw_type", strconv.Itoa(DefaultAPIType))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
