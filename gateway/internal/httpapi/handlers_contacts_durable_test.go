// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestContactsFlagOffHandlerKeepsCap(t *testing.T) {
	for _, v := range []string{"", "0", "false", "off"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("GOSO_CONTACTS_DURABLE", v)
			t.Setenv("GOSO_LITE", "")
			path := filepath.Join(t.TempDir(), "off.db")
			st, err := store.OpenSQLite(path)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			dir := channel.NewContacts()
			h := NewRouter(Options{Store: st, Version: "t", Contacts: dir})
			for i := 0; i < 201; i++ {
				dir.Observe(channel.Sighting{
					Channel: "telegram", Dest: fmt.Sprintf("ma-%04d", i), Kind: "user",
					At: time.Unix(int64(1000+i), 0).UTC(),
				})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
			if w.Code != 200 {
				t.Fatalf("GET %d %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			if !strings.Contains(body, `"total":200`) {
				t.Fatalf("cap %s", body)
			}
			if strings.Contains(body, `"dest":"ma-0000"`) {
				t.Fatal("oldest survived the cap")
			}
			if !strings.Contains(body, `"dest":"ma-0200"`) {
				t.Fatal("newest missing")
			}
			n, err := st.CountContacts("default")
			if err != nil || n != 0 {
				t.Fatalf("flag off persisted %d %v", n, err)
			}
		})
	}
}

func TestContactsDurableHandlerUncapped(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DURABLE", "1")
	t.Setenv("GOSO_LITE", "")
	path := filepath.Join(t.TempDir(), "on.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dir := channel.NewContacts()
	h := NewRouter(Options{Store: st, Version: "t", Contacts: dir})
	const n = 205
	for i := 0; i < n; i++ {
		dir.Observe(channel.Sighting{
			Channel: "telegram", Dest: fmt.Sprintf("ext-minh-an-%04d", i), Kind: "user",
			At: time.Unix(int64(1000+i), 0).UTC(),
		})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, fmt.Sprintf(`"total":%d`, n)) {
		t.Fatalf("total %s", tail(body))
	}
	if !strings.Contains(body, `"dest":"ext-minh-an-0000"`) || !strings.Contains(body, `"dest":"ext-minh-an-0204"`) {
		t.Fatal("handler dropped a durable row")
	}
	if got, err := st.CountContacts("default"); err != nil || got != n {
		t.Fatalf("stored %d %v", got, err)
	}
}

func TestContactsHandlerTenantIsolation(t *testing.T) {
	for _, flag := range []string{"0", "1"} {
		t.Run("flag_"+flag, func(t *testing.T) {
			t.Setenv("GOSO_CONTACTS_DURABLE", flag)
			t.Setenv("GOSO_LITE", "")
			t.Setenv("GOSO_MULTI_TENANT", "1")
			t.Setenv("GOSO_ADMIN_TOKEN", "admin-minh-an")
			t.Setenv("GOSO_VIEW_TOKEN", "view-minh-an")
			path := filepath.Join(t.TempDir(), "iso.db")
			st, err := store.OpenSQLite(path)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			dir := channel.NewContacts()
			h := NewRouter(Options{Store: st, Version: "t", Contacts: dir})
			a := dir.Observe(channel.Sighting{Channel: "telegram", Dest: "ext-minh-an-a", Kind: "user", TenantID: "minh-an", At: time.Unix(10, 0).UTC()})
			b := dir.Observe(channel.Sighting{Channel: "telegram", Dest: "ext-minh-an-b", Kind: "user", TenantID: "other-co", At: time.Unix(11, 0).UTC()})
			dir.Observe(channel.Sighting{Channel: "telegram", Dest: "ext-default", Kind: "user", TenantID: "default", At: time.Unix(12, 0).UTC()})

			w := tenantDo(h, http.MethodGet, "/api/contacts", "", "admin-minh-an", "minh-an")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), "ext-minh-an-a") {
				t.Fatalf("A list %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "ext-minh-an-b") || strings.Contains(w.Body.String(), b.ID) || strings.Contains(w.Body.String(), "ext-default") {
				t.Fatalf("A saw another tenant %s", w.Body.String())
			}
			w = tenantDo(h, http.MethodGet, "/api/contacts/"+b.ID, "", "admin-minh-an", "minh-an")
			if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "ext-minh-an-b") || strings.Contains(w.Body.String(), b.ID) {
				t.Fatalf("cross get %d %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "not found") {
				t.Fatalf("cross get body %s", w.Body.String())
			}
			w = tenantDo(h, http.MethodGet, "/api/contacts?q=ext-minh-an-b", "", "admin-minh-an", "minh-an")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":0`) || strings.Contains(w.Body.String(), "ext-minh-an-b") {
				t.Fatalf("search infer %s", w.Body.String())
			}
			w = tenantDo(h, http.MethodPost, "/api/contacts/"+a.ID+"/merge", `{"source_id":"`+b.ID+`","confirm":"`+b.ID+`"}`, "admin-minh-an", "minh-an")
			if w.Code != http.StatusNotFound {
				t.Fatalf("cross merge %d %s", w.Code, w.Body.String())
			}
			w = tenantDo(h, http.MethodGet, "/api/contacts", "", "admin-minh-an", "other-co")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), "ext-minh-an-b") || strings.Contains(w.Body.String(), "ext-minh-an-a") {
				t.Fatalf("B list %s", w.Body.String())
			}
			// A view token cannot switch tenants, so the other tenant's row stays hidden.
			w = tenantDo(h, http.MethodGet, "/api/contacts", "", "view-minh-an", "other-co")
			if w.Code != 200 || strings.Contains(w.Body.String(), "ext-minh-an-b") || strings.Contains(w.Body.String(), "ext-minh-an-a") {
				t.Fatalf("view token %s", w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "ext-default") {
				t.Fatalf("view token default %s", w.Body.String())
			}
		})
	}
}

func TestContactsListDBFaultReturns500(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DURABLE", "1")
	t.Setenv("GOSO_LITE", "")
	path := filepath.Join(t.TempDir(), "fault.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := channel.NewContacts()
	dir.BindStore(st)
	const dest = "ext-minh-an-keep"
	dir.Observe(channel.Sighting{
		Channel: "telegram", Dest: dest, Kind: "user", TenantID: "minh-an",
		At: time.Unix(9, 0).UTC(),
	})
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := dir.List("minh-an", "", "", "")
	if err == nil {
		t.Fatal("list returned success after the database was closed")
	}
	if len(rows) != 0 {
		t.Fatalf("partial list %#v", rows)
	}
	if strings.Contains(err.Error(), dest) || strings.Contains(err.Error(), "minh-an") {
		t.Fatalf("error leaks identity %v", err)
	}
	h := NewRouter(Options{Store: st, Version: "t", Contacts: dir})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	req.Header.Set("X-Goso-Tenant", "minh-an")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `"contacts"`) || strings.Contains(body, dest) || strings.Contains(body, `"total"`) {
		t.Fatalf("empty or partial success %s", body)
	}
	if !strings.Contains(body, "contact list failed") {
		t.Fatalf("body %s", body)
	}

	t.Run("flag off still lists memory", func(t *testing.T) {
		t.Setenv("GOSO_CONTACTS_DURABLE", "0")
		mem := channel.NewContacts()
		mem.BindStore(st)
		mem.Observe(channel.Sighting{Channel: "telegram", Dest: "ma-memory", Kind: "user", At: time.Unix(1, 0).UTC()})
		got, err := mem.List("", "", "", "")
		if err != nil || len(got) != 1 || got[0].Dest != "ma-memory" {
			t.Fatalf("flag off %#v %v", got, err)
		}
		h := NewRouter(Options{Store: st, Version: "t", Contacts: mem})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"dest":"ma-memory"`) || strings.Contains(w.Body.String(), dest) {
			t.Fatalf("flag off http %d %s", w.Code, w.Body.String())
		}
	})
}

func tail(s string) string {
	if len(s) > 240 {
		return s[:240]
	}
	return s
}
