// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// CustomerIndex maps a GoClaw user_id to a goso customer_key.
// The file is the channel_contacts export read by the contacts importer:
// JSON {"channel_contacts":[...]} or JSONL of those objects.
// A row maps (channel, sender_id, user_id) to "<channel>:<id>".
// id is sender_id when that is set, otherwise user_id.
// A memory user_id matches contact user_id first, then sender_id.
// No match becomes "goclaw:<user_id>". The result is never empty for a non-empty user.
// Rows with a non-empty merged_id are ignored. Display names are not kept.
type CustomerIndex struct {
	byUser   map[string]string
	bySender map[string]string
}

// ReadCustomerMap parses a channel_contacts file. A nil reader is an empty index.
func ReadCustomerMap(r io.Reader) (*CustomerIndex, error) {
	idx := &CustomerIndex{byUser: map[string]string{}, bySender: map[string]string{}}
	if r == nil {
		return idx, nil
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, errConfig("customer map could not be read")
	}
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return idx, nil
	}
	var rows []json.RawMessage
	if trim[0] == '{' {
		var wrap struct {
			Contacts []json.RawMessage `json:"channel_contacts"`
		}
		if err := json.Unmarshal(trim, &wrap); err != nil || wrap.Contacts == nil {
			return nil, errConfig("customer map is not channel_contacts JSON")
		}
		rows = wrap.Contacts
	} else {
		sc := bufio.NewScanner(bytes.NewReader(trim))
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			rows = append(rows, append(json.RawMessage(nil), line...))
		}
		if err := sc.Err(); err != nil {
			return nil, errConfig("customer map could not be read")
		}
	}
	for _, raw := range rows {
		var row contactKey
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, errConfig("customer map row is not an object")
		}
		if strings.TrimSpace(row.MergedID) != "" {
			continue
		}
		channel := strings.TrimSpace(row.ChannelType)
		if channel == "" {
			channel = strings.TrimSpace(row.Channel)
		}
		sender := strings.TrimSpace(row.SenderID)
		user := strings.TrimSpace(row.UserID)
		id := sender
		if id == "" {
			id = user
		}
		if channel == "" || id == "" || strings.Contains(channel, ":") || strings.ContainsAny(channel, "\r\n") || strings.ContainsAny(id, "\r\n") {
			continue
		}
		key := channel + ":" + id
		if user != "" {
			keepSmaller(idx.byUser, user, key)
		}
		if sender != "" {
			keepSmaller(idx.bySender, sender, key)
		}
	}
	return idx, nil
}

type contactKey struct {
	ChannelType string `json:"channel_type"`
	Channel     string `json:"channel"`
	SenderID    string `json:"sender_id"`
	UserID      string `json:"user_id"`
	MergedID    string `json:"merged_id"`
}

func keepSmaller(m map[string]string, lookup, key string) {
	if old, ok := m[lookup]; !ok || key < old {
		m[lookup] = key
	}
}

// Key returns the customer_key for a non-empty user id. It is never empty.
func (idx *CustomerIndex) Key(user string) string {
	user = strings.TrimSpace(user)
	if user == "" {
		return ""
	}
	if idx != nil {
		if key := strings.TrimSpace(idx.byUser[user]); key != "" {
			return key
		}
		if key := strings.TrimSpace(idx.bySender[user]); key != "" {
			return key
		}
	}
	return "goclaw:" + user
}
