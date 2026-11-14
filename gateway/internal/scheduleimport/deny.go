// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"bufio"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// DenyError means the source tenant or the destination tenant is on the
// denylist. Count is how many of those two ids matched. The error text does
// not include a tenant id.
type DenyError struct{ Count int }

func (e *DenyError) Error() string { return "tenant is denied" }

// LoadDenyFile reads one tenant id per line from a local file. A missing
// file, a file with no ids, or a line that is not a tenant id is an error.
// The error text does not include file contents. Blank lines are ignored.
// This package does not embed a list.
func LoadDenyFile(path string) (map[string]struct{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errConfig("deny file is missing")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errConfig("deny file is missing")
	}
	defer f.Close()
	out := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !store.TenantOK(line) {
			return nil, errConfig("deny file is malformed")
		}
		out[strings.ToLower(line)] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("deny file is malformed")
	}
	if len(out) == 0 {
		return nil, errConfig("deny file is empty")
	}
	return out, nil
}

func tenantDenied(deny map[string]struct{}, tenant string) bool {
	if len(deny) == 0 {
		return false
	}
	_, ok := deny[strings.ToLower(strings.TrimSpace(tenant))]
	return ok
}

// DenialCount is the number of distinct ids in the list that are denied.
func DenialCount(deny map[string]struct{}, ids ...string) int {
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || !tenantDenied(deny, id) {
			continue
		}
		seen[id] = struct{}{}
	}
	return len(seen)
}
