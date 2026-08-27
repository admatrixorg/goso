// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func cronSession(t *testing.T, s StoreIface) *Session {
	t.Helper()
	a, err := s.CreateAgent(Agent{AgentKey: "cron-" + t.Name(), DisplayName: "C"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(Session{AgentID: a.ID, Label: "cron"})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestStore_CronCRUDAndCap(t *testing.T) {
	s := New()
	if len(s.ListCronJobs()) != 0 {
		t.Fatal("empty")
	}
	sess := cronSession(t, s)
	j, err := s.CreateCronJob(CronJob{Spec: "every:1h", SessionID: sess.ID, Message: "hi", Enabled: true})
	if err != nil || j.ID == "" || j.LastRun != nil {
		t.Fatalf("create %v %+v", err, j)
	}
	list := s.ListCronJobs()
	if len(list) != 1 || list[0].Spec != "every:1h" {
		t.Fatalf("list %+v", list)
	}
	now := time.Now().UTC()
	if err := s.MarkCronRun(j.ID, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCronJob(j.ID)
	if err != nil || got.LastRun == nil {
		t.Fatalf("get %v %+v", err, got)
	}
	if err := s.DeleteCronJob(j.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.ListCronJobs()) != 0 {
		t.Fatal("after delete")
	}
	if err := s.DeleteCronJob(j.ID); err != ErrNotFound {
		t.Fatalf("delete missing %v", err)
	}
	if _, err := s.CreateCronJob(CronJob{Spec: "every:1h", SessionID: "nope", Message: "x", Enabled: true}); err == nil {
		t.Fatal("missing session")
	}
}

func TestStore_CronCap20(t *testing.T) {
	s := New()
	sess := cronSession(t, s)
	for i := 0; i < CronJobCap; i++ {
		if _, err := s.CreateCronJob(CronJob{Spec: "every:1h", SessionID: sess.ID, Message: "m", Enabled: true}); err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
	}
	if _, err := s.CreateCronJob(CronJob{Spec: "every:1h", SessionID: sess.ID, Message: "m", Enabled: true}); err != ErrCronCap {
		t.Fatalf("cap %v", err)
	}
}

func TestSQLiteStore_CronPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cron.db")
	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	sess := cronSession(t, s1)
	j, err := s1.CreateCronJob(CronJob{Spec: "every:15m", SessionID: sess.ID, Message: "tick", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.MarkCronRun(j.ID, time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC))
	_ = s1.Close()

	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	list := s2.ListCronJobs()
	if len(list) != 1 || list[0].Spec != "every:15m" || list[0].LastRun == nil {
		t.Fatalf("persist %+v", list)
	}
	if err := s2.DeleteCronJob(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(s2.ListCronJobs()) != 0 {
		t.Fatal("sqlite delete")
	}
}
