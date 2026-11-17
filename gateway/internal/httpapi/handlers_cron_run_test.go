// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCronRunNowTenantScoped(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-minh-an")
	t.Setenv("GOSO_VIEW_TOKEN", "")
	for _, be := range cronHTTPBackends(t) {
		t.Run(be.name, func(t *testing.T) {
			st := be.open(t)
			h := Router(st, "183")
			_, sales := cronHTTPPair(t, st, "minh-an-sales", "co-van-run")
			job, err := st.CreateCronJob(store.CronJob{Spec: "every:1h", SessionID: sales.ID, Message: "nhắc lịch tư vấn", Enabled: false})
			if err != nil {
				t.Fatal(err)
			}

			// Another tenant cannot see or run it.
			w := tenantDo(h, http.MethodPost, "/api/cron/"+job.ID+"/run?wait=1", "", "admin-minh-an", "default")
			if w.Code != http.StatusNotFound {
				t.Fatalf("cross-tenant run %d %s", w.Code, w.Body.String())
			}
			if msgs, _ := st.ListMessages(sales.ID); len(msgs) != 0 {
				t.Fatalf("cross-tenant run fired %+v", msgs)
			}
			w = tenantDo(h, http.MethodPost, "/api/cron/no-such-job/run", "", "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusNotFound {
				t.Fatalf("missing job %d", w.Code)
			}

			// Its own tenant runs it, disabled or not, and sees the outcome.
			w = tenantDo(h, http.MethodPost, "/api/cron/"+job.ID+"/run?wait=1", "", "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusOK {
				t.Fatalf("run %d %s", w.Code, w.Body.String())
			}
			var out struct {
				OK  bool           `json:"ok"`
				Ran bool           `json:"ran"`
				ID  string         `json:"id"`
				Job map[string]any `json:"job"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || !out.OK || !out.Ran || out.ID != job.ID || out.Job["last_run"] == nil {
				t.Fatalf("body %s", w.Body.String())
			}
			msgs, _ := st.ListMessages(sales.ID)
			if len(msgs) != 2 || msgs[0].Content != "nhắc lịch tư vấn" {
				t.Fatalf("messages %+v", msgs)
			}
			fresh, _ := st.GetCronJob(job.ID)
			if fresh.Enabled || fresh.LastRun == nil {
				t.Fatalf("job %+v", fresh)
			}

			// Without wait it answers 202 and runs in the background (v1 alias too).
			w = tenantDo(h, http.MethodPost, "/v1/cron/"+job.ID+"/run", "", "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusAccepted {
				t.Fatalf("async run %d %s", w.Code, w.Body.String())
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if m, _ := st.ListMessages(sales.ID); len(m) == 4 && !cron.Running(job.ID) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("async run did not finish")
		})
	}
}
