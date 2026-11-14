// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package kgimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// CustomerMap is the shared map from PR #46. A row matches on user_id.
// The stored key is "<channel>:<id>". id defaults to sender_id.
// A user_id with no row falls back to "goclaw:<user_id>". An empty
// user_id stays empty. A non-empty user_id never resolves to empty.
//
//	{"customers":[{"channel":"zalo-personal","sender_id":"ext-minh-an-1","user_id":"user-minh-an-01"}]}
type CustomerMap struct {
	byUser map[string]string
}

type customerMapFile struct {
	Customers []customerMapRow `json:"customers"`
}

type customerMapRow struct {
	Channel  string `json:"channel"`
	SenderID string `json:"sender_id"`
	UserID   string `json:"user_id"`
	ID       string `json:"id"`
}

// ReadCustomerMap decodes the map. An empty reader is an empty map.
func ReadCustomerMap(r io.Reader) (CustomerMap, error) {
	if r == nil {
		return CustomerMap{}, nil
	}
	var f customerMapFile
	dec := json.NewDecoder(r)
	if err := dec.Decode(&f); err != nil {
		if err == io.EOF {
			return CustomerMap{}, nil
		}
		return CustomerMap{}, errConfig("customer map is not json")
	}
	out := CustomerMap{byUser: make(map[string]string, len(f.Customers))}
	for _, row := range f.Customers {
		user := strings.TrimSpace(row.UserID)
		channel := strings.TrimSpace(row.Channel)
		id := strings.TrimSpace(row.ID)
		if id == "" {
			id = strings.TrimSpace(row.SenderID)
		}
		if user == "" || channel == "" || id == "" {
			return CustomerMap{}, errConfig("customer map: entry is incomplete")
		}
		key := channel + ":" + id
		if _, ok := out.byUser[user]; ok {
			continue
		}
		out.byUser[user] = key
	}
	return out, nil
}

// ResolveCustomerKey maps a GoClaw user_id to a customer key.
// Empty in, empty out. Otherwise the map hit, or goclaw:<user_id>.
func ResolveCustomerKey(userID string, m CustomerMap) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if key := strings.TrimSpace(m.byUser[userID]); key != "" {
		return key
	}
	return "goclaw:" + userID
}

// DenyList is a local file of tenant ids or slugs. Nothing in this list
// is named by the program. The operator supplies the file.
type DenyList struct {
	ids map[string]struct{}
}

type denyFile struct {
	Tenants []string `json:"tenants"`
}

// ReadDenyFile reads a JSON {"tenants":[...]} document or one token per line.
// Blank lines and lines starting with # are ignored. A missing reader, an
// empty body, or a document that lists no tenant is an error. JSON that
// does not parse is malformed. Blocking is the default: an empty list
// does not mean every tenant is allowed.
func ReadDenyFile(r io.Reader) (DenyList, error) {
	if r == nil {
		return DenyList{}, errConfig("deny file is empty")
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return DenyList{}, errConfig("deny file is malformed")
	}
	if !utf8.Valid(body) {
		return DenyList{}, errConfig("deny file is malformed")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return DenyList{}, errConfig("deny file is empty")
	}
	out := DenyList{ids: map[string]struct{}{}}
	switch trimmed[0] {
	case '{', '[':
		if err := decodeDenyJSON(trimmed, &out); err != nil {
			return DenyList{}, err
		}
	default:
		sc := bufio.NewScanner(bytes.NewReader(body))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			addDeny(&out, line)
		}
		if err := sc.Err(); err != nil {
			return DenyList{}, errConfig("deny file is malformed")
		}
	}
	if len(out.ids) == 0 {
		return DenyList{}, errConfig("deny file is empty")
	}
	return out, nil
}

func decodeDenyJSON(body []byte, out *DenyList) error {
	if body[0] == '[' {
		var ids []string
		if err := json.Unmarshal(body, &ids); err != nil {
			return errConfig("deny file is malformed")
		}
		for _, id := range ids {
			addDeny(out, id)
		}
		return nil
	}
	var f denyFile
	if err := json.Unmarshal(body, &f); err != nil {
		return errConfig("deny file is malformed")
	}
	for _, id := range f.Tenants {
		addDeny(out, id)
	}
	return nil
}

func addDeny(d *DenyList, id string) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return
	}
	if d.ids == nil {
		d.ids = map[string]struct{}{}
	}
	d.ids[id] = struct{}{}
}

// Has reports whether id is on the list. Comparison is case-insensitive.
func (d DenyList) Has(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || d.ids == nil {
		return false
	}
	_, ok := d.ids[id]
	return ok
}
