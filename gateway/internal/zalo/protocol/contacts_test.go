package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// contactServer answers the three routes FetchContacts walks, crypting every
// reply the way Zalo does, so the test proves the parse and not the network.
func contactServer(t *testing.T, secret string) *httptest.Server {
	t.Helper()
	key, err := DecodeSecretKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	seal := func(inner any) string {
		blob, merr := json.Marshal(inner)
		if merr != nil {
			t.Fatal(merr)
		}
		enc, eerr := EncodeAESCBC(key, string(blob), false)
		if eerr != nil {
			t.Fatal(eerr)
		}
		return enc
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/social/friend/getfriends", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("params") == "" {
			t.Error("friend list request carried no params")
		}
		if r.URL.Query().Get("zpw_ver") == "" {
			t.Error("friend list request carried no client version")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": 0,
			"data": seal(map[string]any{
				"error_code": 0,
				"data": []map[string]any{
					{"userId": "u2", "displayName": "Binh"},
					{"userId": "u1", "displayName": "An", "zaloName": "an.zalo"},
				},
			}),
		})
	})
	mux.HandleFunc("/api/group/getlg/v4", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": 0,
			"data": seal(map[string]any{
				"error_code": 0,
				"data":       map[string]any{"gridVerMap": map[string]string{"g1": "7"}},
			}),
		})
	})
	mux.HandleFunc("/api/group/getmg-v2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("group details used %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.PostForm.Get("params") == "" {
			t.Error("group details request carried no params")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": 0,
			"data": seal(map[string]any{
				"error_code": 0,
				"data": map[string]any{
					"gridInfoMap": map[string]any{
						"g1": map[string]any{"name": "Team Ban Hang", "avt": "https://avt/g1", "totalMember": 12},
					},
				},
			}),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func contactSession(t *testing.T, base, secret string) *Session {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	return &Session{
		SecretKey: secret,
		IMEI:      "imei-1",
		BaseURL:   u,
		HTTP:      http.DefaultClient,
		Services: &ServiceMap{
			Profile:   []string{base},
			Group:     []string{base},
			GroupPoll: []string{base},
		},
	}
}

func TestFetchContactsReadsFriendsAndGroups(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 16)))
	srv := contactServer(t, secret)
	sess := contactSession(t, srv.URL, secret)

	contacts, err := sess.FetchContacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts.Friends) != 2 {
		t.Fatalf("friends %d", len(contacts.Friends))
	}
	if contacts.Friends[0].DisplayName != "An" || contacts.Friends[1].DisplayName != "Binh" {
		t.Fatalf("friends not sorted by name: %+v", contacts.Friends)
	}
	if got := contacts.Friends[0].ZaloName; got != "an.zalo" {
		t.Fatalf("zalo name %q", got)
	}
	if len(contacts.Groups) != 1 {
		t.Fatalf("groups %d", len(contacts.Groups))
	}
	if contacts.Groups[0].Name != "Team Ban Hang" || contacts.Groups[0].TotalMember != 12 {
		t.Fatalf("group %+v", contacts.Groups[0])
	}
}

// A credential without a payload key cannot crypt a request, and must say so
// instead of sending an unsigned call.
func TestFetchContactsNeedsThePayloadKey(t *testing.T) {
	sess := &Session{Services: &ServiceMap{Profile: []string{"http://example.test"}}, HTTP: http.DefaultClient}
	_, err := sess.FetchContacts(context.Background())
	if err == nil || !strings.Contains(err.Error(), "secret key") {
		t.Fatalf("error %v", err)
	}
}
