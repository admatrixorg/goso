package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// FriendInfo is one contact the picker can allow.
type FriendInfo struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	ZaloName    string `json:"zaloName,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// GroupListInfo is one group the picker can allow.
type GroupListInfo struct {
	GroupID     string `json:"groupId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar,omitempty"`
	TotalMember int    `json:"totalMember"`
}

// Contacts is the picker payload: who the account can hear from.
type Contacts struct {
	Friends []FriendInfo    `json:"friends"`
	Groups  []GroupListInfo `json:"groups"`
}

// FetchContacts reads the friend list and the group list.
func (s *Session) FetchContacts(ctx context.Context) (*Contacts, error) {
	friends, err := s.FetchFriends(ctx)
	if err != nil {
		return nil, err
	}
	out := &Contacts{Friends: friends, Groups: []GroupListInfo{}}
	// A group read can fail on an account that belongs to none, so a failed
	// group read must not hide the friend list that already loaded.
	if groups, gerr := s.FetchGroups(ctx); gerr == nil {
		out.Groups = groups
	}
	return out, nil
}

// FetchFriends lists the account's friends.
func (s *Session) FetchFriends(ctx context.Context) ([]FriendInfo, error) {
	key, err := s.payloadKey()
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"page":        1,
		"count":       20000,
		"incInvalid":  1,
		"avatar_size": 120,
		"actiontime":  0,
	}
	if s.IMEI != "" {
		payload["imei"] = s.IMEI
	}
	blob, merr := json.Marshal(payload)
	if merr != nil {
		return nil, merr
	}
	enc, eerr := EncodeAESCBC(key, string(blob), false)
	if eerr != nil {
		return nil, fmt.Errorf("zalo personal: encrypt friend list request: %w", eerr)
	}
	plain, derr := s.apiGetEncrypted(ctx, "profile", "/api/social/friend/getfriends", map[string]string{"params": enc})
	if derr != nil {
		return nil, derr
	}
	friends := []FriendInfo{}
	if len(plain) == 0 {
		return friends, nil
	}
	if jerr := json.Unmarshal(plain, &friends); jerr != nil {
		return nil, fmt.Errorf("zalo personal: friend list is not json: %w", jerr)
	}
	for i := range friends {
		if strings.TrimSpace(friends[i].DisplayName) == "" {
			friends[i].DisplayName = friends[i].ZaloName
		}
	}
	sort.Slice(friends, func(i, j int) bool {
		return strings.ToLower(friends[i].DisplayName) < strings.ToLower(friends[j].DisplayName)
	})
	return friends, nil
}

// FetchGroups lists the account's groups in two steps, because Zalo returns the
// ids first and the details in bounded batches.
func (s *Session) FetchGroups(ctx context.Context) ([]GroupListInfo, error) {
	plain, err := s.apiGetEncrypted(ctx, "group_poll", "/api/group/getlg/v4", nil)
	if err != nil {
		return nil, err
	}
	versions := map[string]string{}
	if len(plain) > 0 {
		var ids struct {
			GridVerMap map[string]string `json:"gridVerMap"`
		}
		if jerr := json.Unmarshal(plain, &ids); jerr == nil {
			versions = ids.GridVerMap
		}
		if len(versions) == 0 {
			var plainIDs map[string]string
			if jerr := json.Unmarshal(plain, &plainIDs); jerr == nil {
				versions = plainIDs
			}
		}
	}
	if len(versions) == 0 {
		return []GroupListInfo{}, nil
	}
	key, kerr := s.payloadKey()
	if kerr != nil {
		return nil, kerr
	}
	ids := make([]string, 0, len(versions))
	for id := range versions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	const batchSize = 50
	groups := []GroupListInfo{}
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := make(map[string]string, end-start)
		for _, id := range ids[start:end] {
			batch[id] = versions[id]
		}
		// The details call wants version zero for every group, because a real
		// version makes Zalo answer with the group listed as unchanged and no
		// info at all in gridInfoMap.
		zeroed := make(map[string]int, end-start)
		for _, id := range ids[start:end] {
			zeroed[id] = 0
		}
		gridJSON, merr := json.Marshal(zeroed)
		if merr != nil {
			return nil, merr
		}
		blob, merr2 := json.Marshal(map[string]any{"gridVerMap": string(gridJSON)})
		if merr2 != nil {
			return nil, merr2
		}
		enc, eerr := EncodeAESCBC(key, string(blob), false)
		if eerr != nil {
			return nil, fmt.Errorf("zalo personal: encrypt group request: %w", eerr)
		}
		detail, derr := s.apiPostEncrypted(ctx, "group", "/api/group/getmg-v2", enc)
		if derr != nil {
			return nil, derr
		}
		if len(detail) == 0 {
			continue
		}
		var parsed struct {
			GridInfoMap map[string]struct {
				Name        string `json:"name"`
				Avatar      string `json:"avt"`
				TotalMember int    `json:"totalMember"`
			} `json:"gridInfoMap"`
		}
		if jerr := json.Unmarshal(detail, &parsed); jerr != nil {
			return nil, fmt.Errorf("zalo personal: group details are not json: %w", jerr)
		}
		for id, info := range parsed.GridInfoMap {
			groups = append(groups, GroupListInfo{
				GroupID:     id,
				Name:        info.Name,
				Avatar:      info.Avatar,
				TotalMember: info.TotalMember,
			})
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}

// payloadKey is the AES key Zalo crypts an API payload with.
func (s *Session) payloadKey() ([]byte, error) {
	if s == nil || strings.TrimSpace(s.SecretKey) == "" {
		return nil, fmt.Errorf("zalo personal: the credential carries no secret key, so the API payload cannot be crypted")
	}
	return DecodeSecretKey(s.SecretKey)
}

// apiGetEncrypted calls a service route and returns the decrypted data field.
func (s *Session) apiGetEncrypted(ctx context.Context, serviceKey, apiPath string, params map[string]string) (json.RawMessage, error) {
	if s == nil || s.HTTP == nil {
		return nil, fmt.Errorf("zalo personal: session is not ready")
	}
	base := s.ServiceURL(serviceKey)
	if base == "" {
		return nil, fmt.Errorf("zalo personal: the login info carries no %s service url", serviceKey)
	}
	reqURL := s.BuildURL(base+apiPath, params)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	s.Decorate(req)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal: %s request: %w", apiPath, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("zalo personal: %s answered http %d", apiPath, resp.StatusCode)
	}
	var envelope struct {
		ErrorCode int             `json:"error_code"`
		ErrorMsg  string          `json:"error_message"`
		Data      json.RawMessage `json:"data"`
	}
	if jerr := json.Unmarshal(body, &envelope); jerr != nil {
		return nil, fmt.Errorf("zalo personal: %s reply is not json", apiPath)
	}
	if envelope.ErrorCode != 0 {
		msg := strings.TrimSpace(envelope.ErrorMsg)
		if msg == "" {
			msg = fmt.Sprintf("error %d", envelope.ErrorCode)
		}
		return nil, fmt.Errorf("zalo personal: %s rejected: %s", apiPath, msg)
	}
	var encrypted string
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, &encrypted) != nil || encrypted == "" {
		return nil, nil
	}
	return s.decryptDataField(encrypted)
}

// apiPostEncrypted posts an encrypted payload and returns the decrypted data.
func (s *Session) apiPostEncrypted(ctx context.Context, serviceKey, apiPath, params string) (json.RawMessage, error) {
	if s == nil || s.HTTP == nil {
		return nil, fmt.Errorf("zalo personal: session is not ready")
	}
	base := s.ServiceURL(serviceKey)
	if base == "" {
		return nil, fmt.Errorf("zalo personal: the login info carries no %s service url", serviceKey)
	}
	reqURL := s.BuildURL(base+apiPath, nil)
	form := url.Values{}
	form.Set("params", params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	s.Decorate(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal: %s request: %w", apiPath, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("zalo personal: %s answered http %d", apiPath, resp.StatusCode)
	}
	var envelope struct {
		ErrorCode int             `json:"error_code"`
		ErrorMsg  string          `json:"error_message"`
		Data      json.RawMessage `json:"data"`
	}
	if jerr := json.Unmarshal(body, &envelope); jerr != nil {
		return nil, fmt.Errorf("zalo personal: %s reply is not json", apiPath)
	}
	if envelope.ErrorCode != 0 {
		msg := strings.TrimSpace(envelope.ErrorMsg)
		if msg == "" {
			msg = fmt.Sprintf("error %d", envelope.ErrorCode)
		}
		return nil, fmt.Errorf("zalo personal: %s rejected: %s", apiPath, msg)
	}
	var encrypted string
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, &encrypted) != nil || encrypted == "" {
		return nil, nil
	}
	return s.decryptDataField(encrypted)
}

// decryptDataField opens a data field, which wraps another response envelope.
func (s *Session) decryptDataField(data string) (json.RawMessage, error) {
	key, err := s.payloadKey()
	if err != nil {
		return nil, err
	}
	raw := data
	if unescaped, uerr := url.PathUnescape(data); uerr == nil {
		raw = unescaped
	}
	plain, derr := DecodeAESCBC(key, raw)
	if derr != nil {
		return nil, fmt.Errorf("zalo personal: decrypt payload: %w", derr)
	}
	var inner struct {
		ErrorCode int             `json:"error_code"`
		ErrorMsg  string          `json:"error_message"`
		Data      json.RawMessage `json:"data"`
	}
	if jerr := json.Unmarshal(plain, &inner); jerr != nil {
		return nil, fmt.Errorf("zalo personal: unwrap payload: %w", jerr)
	}
	if inner.ErrorCode != 0 {
		msg := strings.TrimSpace(inner.ErrorMsg)
		if msg == "" {
			msg = fmt.Sprintf("error %d", inner.ErrorCode)
		}
		return nil, fmt.Errorf("zalo personal: payload rejected: %s", msg)
	}
	return inner.Data, nil
}
