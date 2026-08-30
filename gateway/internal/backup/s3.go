// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// S3Public is the GET body. Secrets are never included; configured is the status bit.
type S3Public struct {
	Configured   bool   `json:"configured"`
	Endpoint     string `json:"endpoint,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Region       string `json:"region,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	AccessKeySet bool   `json:"access_key_set"`
	EnvOwned     bool   `json:"env_owned"`
}

// S3Write is the PUT body. Access/secret are write-only.
type S3Write struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"access_key"`
	Secret    string `json:"secret"`
}

type s3mem struct {
	endpoint  string
	bucket    string
	region    string
	prefix    string
	accessKey string
	secret    string
}

// Remote is optional S3-compatible storage for snapshots.
type Remote struct {
	mu     sync.Mutex
	mem    s3mem
	Client *http.Client
	now    func() time.Time
}

// NewRemote returns an empty S3 overlay. Environment values overlay GET.
func s3HTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func NewRemote() *Remote {
	return &Remote{
		Client: s3HTTPClient(),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (r *Remote) client() *http.Client {
	if r != nil && r.Client != nil {
		return r.Client
	}
	return s3HTTPClient()
}

func (r *Remote) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now().UTC()
}

func envS3() s3mem {
	return s3mem{
		endpoint:  strings.TrimSpace(os.Getenv("GOSO_BACKUP_S3_ENDPOINT")),
		bucket:    strings.TrimSpace(os.Getenv("GOSO_BACKUP_S3_BUCKET")),
		region:    strings.TrimSpace(os.Getenv("GOSO_BACKUP_S3_REGION")),
		prefix:    strings.TrimSpace(os.Getenv("GOSO_BACKUP_S3_PREFIX")),
		accessKey: strings.TrimSpace(os.Getenv("GOSO_BACKUP_S3_ACCESS_KEY")),
		secret:    strings.TrimSpace(os.Getenv("GOSO_BACKUP_S3_SECRET")),
	}
}

func (r *Remote) merged() (s3mem, bool) {
	env := envS3()
	mem := s3mem{}
	if r != nil {
		r.mu.Lock()
		mem = r.mem
		r.mu.Unlock()
	}
	out := mem
	if env.endpoint != "" {
		out.endpoint = env.endpoint
	}
	if env.bucket != "" {
		out.bucket = env.bucket
	}
	if env.region != "" {
		out.region = env.region
	}
	if env.prefix != "" {
		out.prefix = env.prefix
	}
	if env.accessKey != "" {
		out.accessKey = env.accessKey
	}
	if env.secret != "" {
		out.secret = env.secret
	}
	envOwned := env.accessKey != "" || env.secret != ""
	return out, envOwned
}

// Public returns non-secret S3 status. configured is true only when a usable target exists.
func (r *Remote) Public() S3Public {
	cfg, envOwned := r.merged()
	set := cfg.accessKey != "" && cfg.secret != ""
	return S3Public{
		Configured:   set && cfg.endpoint != "" && cfg.bucket != "",
		Endpoint:     cfg.endpoint,
		Bucket:       cfg.bucket,
		Region:       cfg.region,
		Prefix:       cfg.prefix,
		AccessKeySet: cfg.accessKey != "",
		EnvOwned:     envOwned,
	}
}

// Put stores write-only credentials in process memory. Env-owned secrets are refused.
func (r *Remote) Put(in S3Write) (S3Public, error) {
	_, envOwned := r.merged()
	if envOwned && (strings.TrimSpace(in.AccessKey) != "" || strings.TrimSpace(in.Secret) != "") {
		return r.Public(), ErrEnvOwned
	}
	if r == nil {
		return S3Public{}, ErrNotConfigured
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if ep := strings.TrimSpace(in.Endpoint); ep != "" {
		r.mem.endpoint = ep
	}
	if b := strings.TrimSpace(in.Bucket); b != "" {
		r.mem.bucket = b
	}
	if rg := strings.TrimSpace(in.Region); rg != "" {
		r.mem.region = rg
	}
	r.mem.prefix = strings.TrimSpace(in.Prefix)
	if ak := strings.TrimSpace(in.AccessKey); ak != "" {
		r.mem.accessKey = ak
	}
	if sec := strings.TrimSpace(in.Secret); sec != "" {
		r.mem.secret = sec
	}
	return r.publicLocked(), nil
}

func (r *Remote) publicLocked() S3Public {
	env := envS3()
	cfg := r.mem
	if env.endpoint != "" {
		cfg.endpoint = env.endpoint
	}
	if env.bucket != "" {
		cfg.bucket = env.bucket
	}
	if env.region != "" {
		cfg.region = env.region
	}
	if env.prefix != "" {
		cfg.prefix = env.prefix
	}
	if env.accessKey != "" {
		cfg.accessKey = env.accessKey
	}
	if env.secret != "" {
		cfg.secret = env.secret
	}
	set := cfg.accessKey != "" && cfg.secret != ""
	return S3Public{
		Configured:   set && cfg.endpoint != "" && cfg.bucket != "",
		Endpoint:     cfg.endpoint,
		Bucket:       cfg.bucket,
		Region:       cfg.region,
		Prefix:       cfg.prefix,
		AccessKeySet: cfg.accessKey != "",
		EnvOwned:     env.accessKey != "" || env.secret != "",
	}
}

// Clear drops memory credentials after confirm matches bucket or "s3".
func (r *Remote) Clear(confirm string) (S3Public, error) {
	_, envOwned := r.merged()
	if envOwned {
		return r.Public(), ErrEnvOwned
	}
	if r == nil {
		return S3Public{}, ErrNotConfigured
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	want := r.mem.bucket
	if want == "" {
		want = "s3"
	}
	got := strings.TrimSpace(confirm)
	if got != want && !strings.EqualFold(got, "s3") {
		return r.publicLocked(), ErrConfirm
	}
	r.mem = s3mem{}
	return r.publicLocked(), nil
}

// Test writes and heads a probe object. Never logs credentials.
func (r *Remote) Test() error {
	cfg, _ := r.merged()
	if cfg.endpoint == "" || cfg.bucket == "" || cfg.accessKey == "" || cfg.secret == "" {
		return ErrNotConfigured
	}
	key := strings.TrimSuffix(cfg.prefix, "/")
	if key != "" {
		key += "/"
	}
	key += ".goso-backup-probe"
	body := []byte("goso-backup-probe")
	if err := r.putObject(cfg, key, body); err != nil {
		return err
	}
	return r.headObject(cfg, key)
}

// UploadFile copies a local snapshot to the configured bucket without buffering the file.
func (r *Remote) UploadFile(localPath string) (string, error) {
	cfg, _ := r.merged()
	if cfg.endpoint == "" || cfg.bucket == "" || cfg.accessKey == "" || cfg.secret == "" {
		return "", ErrNotConfigured
	}
	st, err := os.Stat(localPath)
	if err != nil {
		return "", err
	}
	sum, err := sha256File(localPath)
	if err != nil {
		return "", err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	base := path.Base(strings.ReplaceAll(localPath, "\\", "/"))
	key := strings.Trim(cfg.prefix, "/")
	if key != "" {
		key += "/"
	}
	key += base
	req, err := r.signedRequestReader(cfg, http.MethodPut, key, f, st.Size(), sum)
	if err != nil {
		return "", err
	}
	res, err := r.client().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode/100 != 2 {
		return "", fmt.Errorf("s3 put status %d", res.StatusCode)
	}
	return key, nil
}

func (r *Remote) putObject(cfg s3mem, key string, body []byte) error {
	req, err := r.signedRequest(cfg, http.MethodPut, key, body)
	if err != nil {
		return err
	}
	res, err := r.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("s3 put status %d", res.StatusCode)
	}
	return nil
}

func (r *Remote) headObject(cfg s3mem, key string) error {
	req, err := r.signedRequest(cfg, http.MethodHead, key, nil)
	if err != nil {
		return err
	}
	res, err := r.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("s3 head status %d", res.StatusCode)
	}
	return nil
}

func (r *Remote) signedRequest(cfg s3mem, method, key string, body []byte) (*http.Request, error) {
	payload := body
	if payload == nil {
		payload = []byte{}
	}
	return r.signedRequestReader(cfg, method, key, bytes.NewReader(payload), int64(len(payload)), sha256Hex(payload))
}

func (r *Remote) signedRequestReader(cfg s3mem, method, key string, body io.Reader, size int64, payloadHash string) (*http.Request, error) {
	endpoint := strings.TrimRight(cfg.endpoint, "/")
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	region := cfg.region
	if region == "" {
		region = "us-east-1"
	}
	key = strings.TrimPrefix(key, "/")
	u.Path = "/" + cfg.bucket + "/" + key
	now := r.clock()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	host := u.Host
	canonicalURI := u.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := s3SigningKey(cfg.secret, dateStamp, region, "s3")
	sig := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	auth := "AWS4-HMAC-SHA256 Credential=" + cfg.accessKey + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + sig
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	req.Header.Set("Authorization", auth)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("Host", host)
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	return req, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(data)
	return m.Sum(nil)
}

func s3SigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
