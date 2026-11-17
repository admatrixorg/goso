// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCron_RunNow(t *testing.T) {
	f := newOpsFixture(t)
	job, err := f.st.CreateCronJob(store.CronJob{Spec: "every:1h", SessionID: f.notesA.ID, Message: "Nhắc Minh An gọi lại khách", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	var ran []string
	withRun := func(env OpsEnv) OpsEnv {
		env.RunCron = func(j *store.CronJob) error {
			if len(ran) > 0 {
				return cron.ErrRunning
			}
			ran = append(ran, j.ID)
			return nil
		}
		return env
	}

	// Not wired: not_configured.
	if _, err := ops(t, f.env(f.agentA, f.webA), ToolCron, map[string]any{"action": "run", "job_id": job.ID}); err == nil || !strings.Contains(err.Error(), "not_configured") {
		t.Fatalf("unwired: %v", err)
	}
	out, err := ops(t, withRun(f.env(f.agentA, f.webA)), ToolCron, map[string]any{"action": "run", "job_id": job.ID})
	if err != nil || out["ran"] != true || out["job_id"] != job.ID || len(ran) != 1 {
		t.Fatalf("run: %v %+v %v", err, out, ran)
	}
	if _, err := ops(t, withRun(f.env(f.agentA, f.webA)), ToolCron, map[string]any{"action": "run", "job_id": job.ID}); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second run: %v", err)
	}
	ran = nil

	// Another agent of the tenant, another tenant, a customer thread, and a
	// turn started by cron: all refused, nothing runs.
	for name, env := range map[string]OpsEnv{
		"other agent":     withRun(f.env(f.agentA2, f.otherA)),
		"other tenant":    withRun(f.env(f.agentB, f.webB)),
		"customer thread": withRun(f.env(f.agentA, f.custA)),
	} {
		if _, err := ops(t, env, ToolCron, map[string]any{"action": "run", "job_id": job.ID}); err == nil {
			t.Fatalf("%s ran the job", name)
		}
	}
	fromCron := withRun(f.env(f.agentA, f.webA))
	fromCron.FromCron = true
	if _, err := ops(t, fromCron, ToolCron, map[string]any{"action": "run", "job_id": job.ID}); err == nil || !strings.Contains(err.Error(), "cron") {
		t.Fatalf("cron turn: %v", err)
	}
	if _, err := ops(t, withRun(f.env(f.agentA, f.webA)), ToolCron, map[string]any{"action": "run"}); err == nil {
		t.Fatal("missing job_id")
	}
	if len(ran) != 0 {
		t.Fatalf("ran %v", ran)
	}
}
