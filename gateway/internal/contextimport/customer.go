// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextimport

import (
	"encoding/json"
	"io"
	"strings"
)

// CustomerLink is one row of the shared customer-key map.
// The stored key is channel + ":" + sender_id. An empty sender id uses user_id.
type CustomerLink struct {
	Channel  string `json:"channel"`
	SenderID string `json:"sender_id"`
	UserID   string `json:"user_id"`
}

// ReadCustomerMap indexes user_id to `<channel>:<id>`.
// A user id that is not in the map falls back to goclaw:<user_id> in customerKey.
func ReadCustomerMap(r io.Reader) (map[string]string, error) {
	if r == nil {
		return nil, errConfig("customer map is missing")
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var rows []CustomerLink
	if err := dec.Decode(&rows); err != nil {
		return nil, errConfig("customer map is malformed")
	}
	out := map[string]string{}
	for _, row := range rows {
		userID := strings.TrimSpace(row.UserID)
		channel := strings.TrimSpace(row.Channel)
		sender := strings.TrimSpace(row.SenderID)
		if userID == "" || channel == "" || strings.ContainsAny(userID+channel+sender, "\r\n\x00") {
			return nil, errConfig("customer map is malformed")
		}
		id := sender
		if id == "" {
			id = userID
		}
		if _, ok := out[userID]; ok {
			continue
		}
		out[userID] = channel + ":" + id
	}
	return out, nil
}

func customerKey(links map[string]string, userID string) string {
	if links != nil {
		if key := strings.TrimSpace(links[userID]); key != "" {
			return key
		}
	}
	return "goclaw:" + userID
}
