// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// CustomerIndex maps a GoClaw user_id to a goso customer_key.
// The file is the TSV written by the contacts importer:
//
//	channel \t sender \t user \t key
//
// Channel names are normalized to the live session catalog
// (zalo_personal → zalo-personal, lark → feishu) before the key is used.
// A memory user_id matches the user column first, then the sender column.
// No match becomes "goclaw:<user_id>". The result is never empty for a
// non-empty user. The file is not a live database.
type CustomerIndex struct {
	byUser   map[string]string
	bySender map[string]string
}

// ReadCustomerMap parses that TSV. A nil or empty reader is an empty index.
// A line that is not four fields is an error. The error text has no ids.
func ReadCustomerMap(r io.Reader) (*CustomerIndex, error) {
	idx := &CustomerIndex{byUser: map[string]string{}, bySender: map[string]string{}}
	if r == nil {
		return idx, nil
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, errConfig("customer map could not be read")
	}
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(body)) == 0 {
		return idx, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			return nil, errConfig("customer map is malformed")
		}
		channel, ok := gosoChannel(parts[0])
		if !ok {
			continue
		}
		sender := strings.TrimSpace(parts[1])
		user := strings.TrimSpace(parts[2])
		key := normalizeCustomerKey(parts[3])
		if key == "" {
			key = customerKeyFrom(channel, sender, user)
		}
		if key == "" || strings.ContainsAny(key, "\r\n") {
			continue
		}
		if user != "" {
			keepSmaller(idx.byUser, user, key)
		}
		if sender != "" {
			keepSmaller(idx.bySender, sender, key)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("customer map could not be read")
	}
	return idx, nil
}

// gosoChannel maps a source channel name onto the live session catalog.
func gosoChannel(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "telegram":
		return "telegram", true
	case "zalo_personal", "zalo-personal":
		return "zalo-personal", true
	case "zalo_oa", "zalo-oa":
		return "zalo-oa", true
	case "discord":
		return "discord", true
	case "slack":
		return "slack", true
	case "feishu", "lark":
		return "feishu", true
	case "whatsapp":
		return "whatsapp", true
	default:
		return "", false
	}
}

func normalizeCustomerKey(key string) string {
	key = strings.TrimSpace(key)
	i := strings.IndexByte(key, ':')
	if i <= 0 {
		return key
	}
	channel, ok := gosoChannel(key[:i])
	if !ok {
		return key
	}
	return channel + key[i:]
}

func customerKeyFrom(channel, sender, user string) string {
	if channel != "" && sender != "" {
		return channel + ":" + sender
	}
	if user != "" {
		return "goclaw:" + user
	}
	return ""
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
