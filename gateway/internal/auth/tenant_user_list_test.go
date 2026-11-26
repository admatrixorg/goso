package auth

import (
	"net/http/httptest"
	"testing"
)

func TestRequire_GatewayReadKeyListsOneTenantsUsers(t *testing.T) {
	cases := []struct {
		name   string
		grant  Grant
		method string
		path   string
		want   int
	}{
		{"gateway read key, v1", Grant{ID: "ak_r", Scopes: []string{"read"}}, "GET", "/v1/users?tenant_id=an-phat", 200},
		{"gateway read key, api", Grant{ID: "ak_r", Scopes: []string{"read"}}, "GET", "/api/users?tenant_id=an-phat", 200},
		{"default tenant spelled out", Grant{ID: "ak_r", TenantID: "default", Scopes: []string{"read"}}, "GET", "/v1/users?tenant_id=an-phat", 200},
		{"no tenant_id lists everyone", Grant{ID: "ak_r", Scopes: []string{"read"}}, "GET", "/v1/users", 403},
		{"blank tenant_id", Grant{ID: "ak_r", Scopes: []string{"read"}}, "GET", "/v1/users?tenant_id=%20", 403},
		{"one user by id", Grant{ID: "ak_r", Scopes: []string{"read"}}, "GET", "/v1/users/u_1?tenant_id=an-phat", 403},
		{"tenant-bound read key", Grant{ID: "ak_t", TenantID: "eac", Scopes: []string{"read"}}, "GET", "/v1/users?tenant_id=an-phat", 403},
		{"tenant-bound key, own tenant", Grant{ID: "ak_t", TenantID: "eac", Scopes: []string{"read"}}, "GET", "/v1/users?tenant_id=eac", 403},
		{"key without read", Grant{ID: "ak_p", Scopes: []string{"pairing"}}, "GET", "/v1/users?tenant_id=an-phat", 403},
		{"read key cannot create", Grant{ID: "ak_r", Scopes: []string{"read"}}, "POST", "/v1/users?tenant_id=an-phat", 403},
		{"read key cannot edit", Grant{ID: "ak_r", Scopes: []string{"read"}}, "PATCH", "/v1/users/u_1?tenant_id=an-phat", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			keys := fakeKeys{token: "gk_x", grant: c.grant}
			h := Require(Config{Admin: "admin-113", Keys: keys})(okHandler())
			r := httptest.NewRequest(c.method, c.path, nil)
			r.Header.Set("Authorization", "Bearer gk_x")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("%s %s = %d, want %d", c.method, c.path, w.Code, c.want)
			}
		})
	}
}

func TestRequire_ViewTokenStillCannotListUsers(t *testing.T) {
	h := Require(Config{Admin: "admin-113", View: "view-1"})(okHandler())
	r := httptest.NewRequest("GET", "/v1/users?tenant_id=an-phat", nil)
	r.Header.Set("Authorization", "Bearer view-1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatalf("view token listed users: %d", w.Code)
	}
}
