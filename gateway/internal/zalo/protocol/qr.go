package protocol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The QR login runs against the id.zalo.me account host, which is a different
// surface from the chat API the listener and sender use.
const (
	qrLoginPageURL = "https://id.zalo.me/account?continue=https%3A%2F%2Fchat.zalo.me%2F"
	qrContinue     = "https://zalo.me/pc"
)

var qrVersionRe = regexp.MustCompile(`https:\/\/stc-zlogin\.zdn\.vn\/main-([\d.]+)\.js`)

// QRCode is one generated login code.
type QRCode struct {
	Code     string
	ImagePNG []byte
}

// QRSession holds the state of one QR login attempt.
type QRSession struct {
	Session    *Session
	VersionStr string
	Code       QRCode
}

// NewQRSession builds a session for the login host.
func NewQRSession() *QRSession {
	jar, _ := cookiejarForLogin()
	return &QRSession{Session: &Session{
		UserAgent: DefaultUserAgent,
		Language:  DefaultLanguage,
		Jar:       jar,
		HTTP: &http.Client{
			Jar:     jar,
			Timeout: 40 * time.Second,
		},
	}}
}

type qrEnvelope struct {
	ErrorCode    int             `json:"error_code"`
	ErrorMessage string          `json:"error_message"`
	Data         json.RawMessage `json:"data"`
}

func (q *QRSession) post(ctx context.Context, endpoint string, form map[string]string) (*qrEnvelope, error) {
	values := url.Values{}
	for k, v := range form {
		values.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", q.Session.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://id.zalo.me/account?continue=https%3A%2F%2Fzalo.me%2Fpc")
	resp, err := q.Session.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env qrEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("zalo personal: qr reply is not json: %w", err)
	}
	return &env, nil
}

// Version loads the login page and reads the client version it references.
func (q *QRSession) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qrLoginPageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", q.Session.UserAgent)
	resp, err := q.Session.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	match := qrVersionRe.FindSubmatch(body)
	if len(match) < 2 {
		return "", fmt.Errorf("zalo personal: version not found in the login page")
	}
	return string(match[1]), nil
}

// StartQR runs the handshake and returns a scannable code plus its PNG.
func (q *QRSession) StartQR(ctx context.Context) (*QRCode, error) {
	version, err := q.Version(ctx)
	if err != nil {
		return nil, err
	}
	q.VersionStr = version
	_, _ = q.post(ctx, "https://id.zalo.me/account/logininfo", map[string]string{"v": version, "continue": qrContinue})
	_, _ = q.post(ctx, "https://id.zalo.me/account/verify-client", map[string]string{"v": version, "type": "device", "continue": qrContinue})
	env, err := q.post(ctx, "https://id.zalo.me/account/authen/qr/generate", map[string]string{"v": version, "continue": qrContinue})
	if err != nil {
		return nil, err
	}
	if env.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo personal: generate qr error %d: %s", env.ErrorCode, env.ErrorMessage)
	}
	var data struct {
		Code  string `json:"code"`
		Image string `json:"image"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("zalo personal: qr data: %w", err)
	}
	b64 := strings.TrimPrefix(data.Image, "data:image/png;base64,")
	img, derr := base64.StdEncoding.DecodeString(b64)
	if derr != nil {
		return nil, fmt.Errorf("zalo personal: qr image: %w", derr)
	}
	out := &QRCode{Code: data.Code, ImagePNG: img}
	q.Code = *out
	return out, nil
}

// WaitForScan polls until the phone scans the code.
func (q *QRSession) WaitForScan(ctx context.Context) error {
	return q.wait(ctx, "https://id.zalo.me/account/authen/qr/waiting-scan", "scan")
}

// WaitForConfirm polls until the phone confirms the login.
func (q *QRSession) WaitForConfirm(ctx context.Context) error {
	return q.wait(ctx, "https://id.zalo.me/account/authen/qr/waiting-confirm", "confirm")
}

func (q *QRSession) wait(ctx context.Context, endpoint, stage string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		env, err := q.post(ctx, endpoint, map[string]string{"v": q.VersionStr, "code": q.Code.Code, "continue": qrContinue})
		if err != nil {
			return err
		}
		switch env.ErrorCode {
		case 8:
			time.Sleep(time.Second)
			continue
		case 0:
			return nil
		case -13:
			return fmt.Errorf("zalo personal: qr login declined at %s", stage)
		default:
			return fmt.Errorf("zalo personal: %s error %d: %s", stage, env.ErrorCode, env.ErrorMessage)
		}
	}
}

// Finish completes the login and returns the stored credential shape.
func (q *QRSession) Finish(ctx context.Context) (*Credential, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://id.zalo.me/account/checksession?continue=https%3A%2F%2Fchat.zalo.me%2Findex.html", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", q.Session.UserAgent)
	if resp, derr := q.Session.HTTP.Do(req); derr == nil {
		resp.Body.Close()
	}
	cred := &Credential{UserAgent: q.Session.UserAgent}
	for _, host := range []string{"https://chat.zalo.me", "https://id.zalo.me", "https://wpa.chat.zalo.me"} {
		u, perr := url.Parse(host)
		if perr != nil || q.Session.Jar == nil {
			continue
		}
		for _, c := range q.Session.Jar.Cookies(u) {
			if strings.TrimSpace(c.Name) == "" {
				continue
			}
			cred.Cookies = append(cred.Cookies, Cookie{Name: c.Name, Value: c.Value})
		}
	}
	return cred, nil
}
