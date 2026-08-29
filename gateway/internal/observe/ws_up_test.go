// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestStats_WsUp(t *testing.T) {
	o := NewWithWriter(nil)
	if o.Snapshot().WsUp {
		t.Fatal("default")
	}
	o.SetWsUp(true)
	if !o.Snapshot().WsUp {
		t.Fatal("set")
	}
	w := httptest.NewRecorder()
	o.HandleStats(w, httptest.NewRequest("GET", "/api/stats", nil))
	var s Stats
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil || !s.WsUp {
		t.Fatalf("%v %+v %s", err, s, w.Body.String())
	}
}
