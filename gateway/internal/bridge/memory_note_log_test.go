// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

type summaryErrStore struct {
	store.StoreIface
	err error
}

func (s summaryErrStore) LatestSummary(string) (*store.Memory, error) { return nil, s.err }

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// The first turn of a new session has no note yet. That is the normal case
// and must not be logged as an error; a real store failure still is.
func TestMemoryNoteFirstTurnIsSilent(t *testing.T) {
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "bridge-note"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "omni:conv-new"})
	if err != nil {
		t.Fatal(err)
	}
	buf := captureLog(t)
	svc := &Service{Store: st}
	if note := svc.memoryNote(sess, "xin chào"); note != "" {
		t.Fatalf("note = %q, want empty", note)
	}
	if strings.Contains(buf.String(), "bridge note") {
		t.Fatalf("first turn logged an error: %s", buf.String())
	}

	buf.Reset()
	svc = &Service{Store: summaryErrStore{StoreIface: st, err: errors.New("disk I/O error")}}
	if note := svc.memoryNote(sess, "xin chào"); note != "" {
		t.Fatalf("note = %q, want empty on store error", note)
	}
	if !strings.Contains(buf.String(), "bridge note session="+sess.ID+" error=disk I/O error") {
		t.Fatalf("store error not logged: %q", buf.String())
	}
}
