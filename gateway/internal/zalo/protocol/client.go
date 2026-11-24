package protocol

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefaultBaseURL is the Zalo web origin the client talks to.
const DefaultBaseURL = "https://chat.zalo.me"

// Cookie is one stored session cookie, the shape GoClaw persists.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
}

// Credential is the cookie set an operator pastes for a personal account.
type Credential struct {
	Cookies   []Cookie `json:"cookies"`
	IMEI      string   `json:"imei,omitempty"`
	UserAgent string   `json:"user_agent,omitempty"`
	Language  string   `json:"language,omitempty"`
	SecretKey string   `json:"secret_key,omitempty"`
}

// ParseCredential reads the stored credential JSON. A bare cookie map is
// accepted too, so an operator can paste what the browser exports.
func ParseCredential(raw string) (*Credential, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("zalo personal: credential is empty")
	}
	var cred Credential
	if err := json.Unmarshal([]byte(raw), &cred); err != nil || len(cred.Cookies) == 0 {
		var plain map[string]string
		if err2 := json.Unmarshal([]byte(raw), &plain); err2 != nil {
			return nil, fmt.Errorf("zalo personal: credential is not valid json")
		}
		cred.Cookies = make([]Cookie, 0, len(plain))
		for k, v := range plain {
			cred.Cookies = append(cred.Cookies, Cookie{Name: k, Value: v})
		}
	}
	if len(cred.Cookies) == 0 {
		return nil, fmt.Errorf("zalo personal: credential has no cookies")
	}
	return &cred, nil
}

// Session is an authenticated Zalo web session.
type Session struct {
	UID       string
	IMEI      string
	UserAgent string
	Language  string
	SecretKey string
	Services  *ServiceMap
	WSURL     string
	wsURLs    []string
	BaseURL   *url.URL
	Jar       http.CookieJar
	HTTP      *http.Client
}

// NewSessionWithCredential seeds a jar with the stored cookies for every host
// the client talks to.
func NewSessionWithCredential(cred *Credential) (*Session, error) {
	if cred == nil || len(cred.Cookies) == 0 {
		return nil, fmt.Errorf("zalo personal: credential has no cookies")
	}
	base, err := url.Parse(DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	applyCookies(jar, base, cred.Cookies)
	sess := &Session{
		IMEI:      strings.TrimSpace(cred.IMEI),
		UserAgent: strings.TrimSpace(cred.UserAgent),
		Language:  strings.TrimSpace(cred.Language),
		SecretKey: strings.TrimSpace(cred.SecretKey),
		BaseURL:   base,
		Jar:       jar,
	}
	if sess.UserAgent == "" {
		sess.UserAgent = DefaultUserAgent
	}
	if sess.Language == "" {
		sess.Language = DefaultLanguage
	}
	sess.HTTP = &http.Client{Jar: jar, Timeout: 30 * time.Second}
	// The QR login captures cookies, and the key Zalo crypts every API payload
	// with travels in the zpw_enk cookie, so read it from there when the stored
	// credential does not carry it already.
	if sess.SecretKey == "" {
		sess.SecretKey = KeyFromCookies(cred.Cookies)
	}
	return sess, nil
}

// KeyFromCookies returns the payload key a cookie set carries. Zalo ships it in
// zpw_enk, base64 encoded, and it never appears as its own field after a login.
func KeyFromCookies(cookies []Cookie) string {
	for _, c := range cookies {
		if strings.EqualFold(strings.TrimSpace(c.Name), "zpw_enk") {
			return strings.TrimSpace(c.Value)
		}
	}
	return ""
}

// applyCookies seeds the base host and the poll subdomains, because
// net/http/cookiejar does not share a cookie across them.
func applyCookies(jar http.CookieJar, base *url.URL, cookies []Cookie) {
	for _, host := range []string{base.String(), "https://wpa.chat.zalo.me", "https://tt-group-poll-wpa.chat.zalo.me"} {
		u, err := url.Parse(host)
		if err != nil {
			continue
		}
		list := make([]*http.Cookie, 0, len(cookies))
		for _, c := range cookies {
			if strings.TrimSpace(c.Name) == "" {
				continue
			}
			list = append(list, &http.Cookie{Name: c.Name, Value: c.Value, Path: "/"})
		}
		jar.SetCookies(u, list)
	}
}

// LoginInfo is the payload Zalo returns for the signed in account.
type LoginInfo struct {
	UID       string `json:"uid"`
	Name      string `json:"display_name,omitempty"`
	SecretKey string `json:"zpw_enk,omitempty"`
}

// LoginPath is the own-info route.
const LoginPath = "/api/login/getLoginInfo"

// DefaultWPAURL is the API host GoClaw calls for login info.
const DefaultWPAURL = "https://wpa.chat.zalo.me"

// SeedCookies places the stored cookies on one host in a jar.
func SeedCookies(jar http.CookieJar, host string, cookies []Cookie) {
	if jar == nil || strings.TrimSpace(host) == "" {
		return
	}
	u, err := url.Parse(host)
	if err != nil {
		return
	}
	list := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		list = append(list, &http.Cookie{Name: c.Name, Value: c.Value, Path: "/"})
	}
	jar.SetCookies(u, list)
}

// GetLoginInfo calls the production own-info endpoint on the WPA host.
func (s *Session) GetLoginInfo(ctx context.Context) (*LoginInfo, error) {
	return s.GetLoginInfoOn(ctx, DefaultWPAURL)
}

// GetLoginInfoOn calls the own-info endpoint on a given base, so a test can
// drive a local server without touching the network.
func (s *Session) GetLoginInfoOn(ctx context.Context, base string) (*LoginInfo, error) {
	if s == nil || s.HTTP == nil {
		return nil, fmt.Errorf("zalo personal: session is not ready")
	}
	endpoint := strings.TrimRight(base, "/") + LoginPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	s.Decorate(req)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zalo personal: login info request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("zalo personal: login info http %d", resp.StatusCode)
	}
	var envelope struct {
		ErrorCode int             `json:"error_code"`
		ErrorMsg  string          `json:"error_message"`
		Data      json.RawMessage `json:"data"`
		UID       string          `json:"uid"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("zalo personal: login info is not json")
	}
	if envelope.ErrorCode != 0 {
		msg := strings.TrimSpace(envelope.ErrorMsg)
		if msg == "" {
			msg = "error " + strconv.Itoa(envelope.ErrorCode)
		}
		return nil, fmt.Errorf("zalo personal: login rejected: %s", msg)
	}
	info := &LoginInfo{UID: strings.TrimSpace(envelope.UID)}
	if len(envelope.Data) > 0 {
		var data struct {
			UID          string   `json:"uid"`
			Name         string   `json:"display_name"`
			ZPWEnk       string   `json:"zpw_enk"`
			ZpwWebsocket []string `json:"zpw_ws"`
			ServiceMap   *struct {
				Chat      []string `json:"chat"`
				Group     []string `json:"group"`
				File      []string `json:"file"`
				Profile   []string `json:"profile"`
				GroupPoll []string `json:"group_poll"`
			} `json:"zpw_service_map_v3"`
		}
		if json.Unmarshal(envelope.Data, &data) == nil {
			if info.UID == "" {
				info.UID = strings.TrimSpace(data.UID)
			}
			info.Name = strings.TrimSpace(data.Name)
			info.SecretKey = strings.TrimSpace(data.ZPWEnk)
			// The payload key arrives with the login reply, so a session that
			// never had one becomes able to crypt API calls after this read.
			if s.SecretKey == "" && info.SecretKey != "" {
				s.SecretKey = info.SecretKey
			}
			if len(data.ZpwWebsocket) > 0 {
				s.wsURLs = append([]string(nil), data.ZpwWebsocket...)
			}
			if data.ServiceMap != nil {
				s.Services = &ServiceMap{Chat: data.ServiceMap.Chat, Group: data.ServiceMap.Group, File: data.ServiceMap.File, Profile: data.ServiceMap.Profile, GroupPoll: data.ServiceMap.GroupPoll}
			}
		}
	}
	if info.UID == "" {
		return nil, fmt.Errorf("zalo personal: login info has no uid")
	}
	s.UID = info.UID
	return info, nil
}

// Decorate adds the headers the Zalo web client sends.
func (s *Session) Decorate(req *http.Request) {
	if s.UserAgent != "" {
		req.Header.Set("User-Agent", s.UserAgent)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if s.BaseURL != nil {
		req.Header.Set("Origin", s.BaseURL.String())
		req.Header.Set("Referer", s.BaseURL.String()+"/")
	}
}

// WebSocketURLs returns the sockets the login handshake handed out.
func (s *Session) WebSocketURLs() []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s.wsURLs))
	copy(out, s.wsURLs)
	return out
}

// cookiejarForLogin returns an empty jar for the login host flow.
func cookiejarForLogin() (http.CookieJar, error) {
	return cookiejar.New(nil)
}

// newIMEI builds the device id Zalo expects on API calls.
func newIMEI(userAgent string) string {
	sum := md5.Sum([]byte(userAgent))
	return uuid.New().String() + "-" + hex.EncodeToString(sum[:])
}
