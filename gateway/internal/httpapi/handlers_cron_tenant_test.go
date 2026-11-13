// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"database/sql"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCronTenant_CrossTenantHTTP(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-minh-an")
	t.Setenv("GOSO_VIEW_TOKEN", "")
	for _, be := range cronHTTPBackends(t) {
		t.Run(be.name, func(t *testing.T) {
			st := be.open(t)
			h := Router(st, "183")
			_, sales := cronHTTPPair(t, st, "minh-an-sales", "co-van-sales")
			_, home := cronHTTPPair(t, st, "default", "co-van-home")

			body := `{"spec":"every:1h","session_id":"` + sales.ID + `","message":"nhắc lịch tư vấn","tenant_id":"minh-an-sales"}`
			w := tenantDo(h, http.MethodPost, "/api/cron", body, "admin-minh-an", "default")
			if w.Code != http.StatusNotFound {
				t.Fatalf("create as default %d %s", w.Code, w.Body.String())
			}
			w = tenantDo(h, http.MethodPost, "/v1/cron", body, "admin-minh-an", "default")
			if w.Code != http.StatusNotFound {
				t.Fatalf("v1 create as default %d %s", w.Code, w.Body.String())
			}
			w = tenantDo(h, http.MethodPost, "/api/cron", body, "", "minh-an-sales")
			if w.Code != http.StatusNotFound {
				t.Fatalf("create without admin %d %s", w.Code, w.Body.String())
			}
			if len(st.ListCronJobs()) != 0 {
				t.Fatal("rejected create still stored a job")
			}

			w = tenantDo(h, http.MethodPost, "/api/cron", `{"spec":"every:1h","session_id":"`+sales.ID+`","message":"nhắc lịch tư vấn","tenant_id":"default"}`, "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusCreated {
				t.Fatalf("create sales %d %s", w.Code, w.Body.String())
			}
			var created store.CronJob
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if created.TenantID != "minh-an-sales" || created.ID == "" {
				t.Fatalf("created %+v", created)
			}
			stored, err := st.GetCronJob(created.ID)
			if err != nil || stored.TenantID != "minh-an-sales" {
				t.Fatalf("stored %v %+v", err, stored)
			}

			homeBody := `{"spec":"every:1h","session_id":"` + home.ID + `","message":"nhắc lịch tư vấn","tenant_id":"minh-an-sales"}`
			w = tenantDo(h, http.MethodPost, "/api/cron", homeBody, "admin-minh-an", "default")
			if w.Code != http.StatusCreated {
				t.Fatalf("create default %d %s", w.Code, w.Body.String())
			}
			var homeJob store.CronJob
			if err := json.Unmarshal(w.Body.Bytes(), &homeJob); err != nil {
				t.Fatal(err)
			}
			if homeJob.TenantID != "default" {
				t.Fatalf("body tenant stuck: %+v", homeJob)
			}

			w = tenantDo(h, http.MethodGet, "/api/cron", "", "admin-minh-an", "default")
			if w.Code != http.StatusOK || strings.Contains(w.Body.String(), created.ID) {
				t.Fatalf("default list %d %s", w.Code, w.Body.String())
			}
			w = tenantDo(h, http.MethodGet, "/api/cron", "", "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), created.ID) || strings.Contains(w.Body.String(), homeJob.ID) {
				t.Fatalf("sales list %d %s", w.Code, w.Body.String())
			}

			w = tenantDo(h, http.MethodPatch, "/api/cron/"+created.ID, `{"enabled":false}`, "admin-minh-an", "default")
			if w.Code != http.StatusNotFound {
				t.Fatalf("patch default %d %s", w.Code, w.Body.String())
			}
			still, err := st.GetCronJob(created.ID)
			if err != nil || !still.Enabled {
				t.Fatalf("cross-tenant patch mutated %+v %v", still, err)
			}
			w = tenantDo(h, http.MethodPatch, "/api/cron/"+created.ID, `{"enabled":false}`, "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":false`) {
				t.Fatalf("patch sales %d %s", w.Code, w.Body.String())
			}

			w = tenantDo(h, http.MethodDelete, "/api/cron/"+created.ID, "", "admin-minh-an", "default")
			if w.Code != http.StatusNotFound {
				t.Fatalf("delete default %d %s", w.Code, w.Body.String())
			}
			if _, err := st.GetCronJob(created.ID); err != nil {
				t.Fatal(err)
			}
			w = tenantDo(h, http.MethodDelete, "/api/cron/"+created.ID, "", "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusOK {
				t.Fatalf("delete sales %d %s", w.Code, w.Body.String())
			}
			if _, err := st.GetCronJob(created.ID); err != store.ErrNotFound {
				t.Fatalf("deleted job %v", err)
			}
			if _, err := st.GetCronJob(homeJob.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCronTenant_CapPerTenantHTTP(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-minh-an")
	t.Setenv("GOSO_VIEW_TOKEN", "")
	for _, be := range cronHTTPBackends(t) {
		t.Run(be.name, func(t *testing.T) {
			st := be.open(t)
			h := Router(st, "183")
			_, sales := cronHTTPPair(t, st, "minh-an-sales", "co-van-cap")
			_, home := cronHTTPPair(t, st, "default", "co-van-cap-home")
			salesBody := `{"spec":"every:1h","session_id":"` + sales.ID + `","message":"nhắc lịch tư vấn"}`
			for i := 0; i < store.CronJobCap; i++ {
				w := tenantDo(h, http.MethodPost, "/api/cron", salesBody, "admin-minh-an", "minh-an-sales")
				if w.Code != http.StatusCreated {
					t.Fatalf("job %d %d %s", i, w.Code, w.Body.String())
				}
			}
			w := tenantDo(h, http.MethodPost, "/api/cron", salesBody, "admin-minh-an", "minh-an-sales")
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "cron cap: max 20 jobs per tenant") {
				t.Fatalf("cap %d %s", w.Code, w.Body.String())
			}
			w = tenantDo(h, http.MethodPost, "/api/cron", `{"spec":"every:1h","session_id":"`+home.ID+`","message":"nhắc lịch tư vấn"}`, "admin-minh-an", "default")
			if w.Code != http.StatusCreated {
				t.Fatalf("other tenant %d %s", w.Code, w.Body.String())
			}
		})
	}
}

type cronHTTPBackend struct {
	name string
	open func(t *testing.T) store.StoreIface
}

func cronHTTPBackends(t *testing.T) []cronHTTPBackend {
	t.Helper()
	return []cronHTTPBackend{
		{name: "memory", open: func(t *testing.T) store.StoreIface { return store.New() }},
		{name: "sqlite", open: func(t *testing.T) store.StoreIface {
			st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "cron.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			return st
		}},
		{name: "postgres", open: openCronHTTPPostgres},
	}
}

func openCronHTTPPostgres(t *testing.T) store.StoreIface {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if base == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	const schema = "goso_cron183_http"
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db, err := sql.Open("pgx", base)
		if err != nil {
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	st, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func cronHTTPPair(t *testing.T, st store.StoreIface, tenant, key string) (*store.Agent, *store.Session) {
	t.Helper()
	ag, err := st.CreateAgent(store.Agent{AgentKey: key, DisplayName: "Cố vấn Minh An", TenantID: tenant})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: tenant, Label: "tư vấn"})
	if err != nil {
		t.Fatal(err)
	}
	return ag, sess
}
