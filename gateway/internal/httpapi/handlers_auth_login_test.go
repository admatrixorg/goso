package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthLoginIssuesAdminToken(t *testing.T) {
	loginResetFailsForTest()
	t.Setenv("GOSO_LOGIN_EMAIL", "owner@local.test")
	t.Setenv("GOSO_LOGIN_PASSWORD", "s3cret")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-token")

	mux := http.NewServeMux()
	registerAuthLoginRoutes(mux, nil)

	body, _ := json.Marshal(map[string]string{"email": "Owner@local.test", "password": "s3cret"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.AccessToken != "admin-token" || got.User.Email != "owner@local.test" || got.Tenant.Role != "owner" {
		t.Fatalf("response %+v", got)
	}
}

func TestAuthLoginAcceptsSecondAccountAndLocalPart(t *testing.T) {
	loginResetFailsForTest()
	t.Setenv("GOSO_LOGIN_EMAIL", "admin@omni.local")
	t.Setenv("GOSO_LOGIN_PASSWORD", "omni-pass")
	t.Setenv("GOSO_LOGIN_EMAIL_2", "admin@admatrix.vn")
	t.Setenv("GOSO_LOGIN_PASSWORD_2", "admatrix-pass")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-token")

	mux := http.NewServeMux()
	registerAuthLoginRoutes(mux, nil)

	try := func(email, pass string, want int) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "password": pass})
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s status %d want %d body %s", email, w.Code, want, w.Body.String())
		}
	}
	try("admin@admatrix.vn", "admatrix-pass", 200)
	try("admin@omni.local", "omni-pass", 200)
	try("admin", "omni-pass", 200)
	try("admin@admatrix.vn", "omni-pass", 401)
}

func TestAuthLoginRejectsBadPassword(t *testing.T) {
	loginResetFailsForTest()
	t.Setenv("GOSO_LOGIN_EMAIL", "owner@local.test")
	t.Setenv("GOSO_LOGIN_PASSWORD", "s3cret")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-token")

	mux := http.NewServeMux()
	registerAuthLoginRoutes(mux, nil)
	body, _ := json.Marshal(map[string]string{"email": "owner@local.test", "password": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
}
