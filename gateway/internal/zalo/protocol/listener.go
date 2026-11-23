package protocol

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// InboundMessage is one decoded message from the listener.
type InboundMessage struct {
	ID         string `json:"id"`
	FromID     string `json:"from_id"`
	FromName   string `json:"from_name,omitempty"`
	ThreadID   string `json:"thread_id"`
	ThreadType string `json:"thread_type"`
	Text       string `json:"text"`
	Timestamp  int64  `json:"ts"`
}

// Listener reads Zalo frames and hands decoded messages to a handler.
type Listener struct {
	sess    *Session
	wsURL   string
	mu      sync.RWMutex
	conn    *websocket.Conn
	cipher  string
	closed  bool
	onMsg   func(InboundMessage)
	done    chan struct{}
}

// NewListener builds a listener for the first websocket URL the login
// handshake returned. Without one there is nothing to connect to.
func NewListener(sess *Session, onMessage func(InboundMessage)) (*Listener, error) {
	if sess == nil {
		return nil, fmt.Errorf("zalo personal: session is not ready")
	}
	if strings.TrimSpace(sess.WSURL) == "" {
		return nil, fmt.Errorf("zalo personal: login info carries no websocket url")
	}
	if onMessage == nil {
		onMessage = func(InboundMessage) {}
	}
	return &Listener{
		sess:  sess,
		wsURL: sess.WSURL,
		onMsg: onMessage,
		done:  make(chan struct{}),
	}, nil
}

// Start dials the websocket and reads frames until the context ends.
func (l *Listener) Start(ctx context.Context) error {
	hdr := http.Header{}
	hdr.Set("Origin", DefaultBaseURL)
	hdr.Set("User-Agent", l.sess.UserAgent)
	hdr.Set("Accept-Language", "en-US,en;q=0.9")
	if l.sess.Jar != nil {
		SeedCookies(l.sess.Jar, strings.Replace(l.wsURL, "wss://", "https://", 1), nil)
	}
	dialer := websocket.Dialer{EnableCompression: true, HandshakeTimeout: 30e9}
	conn, _, err := dialer.DialContext(ctx, l.wsURL, hdr)
	if err != nil {
		return fmt.Errorf("zalo personal: ws dial: %w", err)
	}
	conn.SetReadLimit(1 << 20)
	l.mu.Lock()
	l.conn = conn
	l.mu.Unlock()
	defer close(l.done)
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, data, rerr := conn.ReadMessage()
		if rerr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("zalo personal: ws read: %w", rerr)
		}
		l.HandleFrame(data)
	}
}

// Stop closes the socket.
func (l *Listener) Stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	conn := l.conn
	l.closed = true
	l.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// Done closes when the read loop exits.
func (l *Listener) Done() <-chan struct{} { return l.done }

// HandleFrame decodes one Zalo frame: a version byte, a little endian command,
// a sub command, then a JSON envelope.
func (l *Listener) HandleFrame(data []byte) {
	if len(data) < 5 {
		return
	}
	version := data[0]
	cmd := binary.LittleEndian.Uint16(data[1:3])
	subCmd := data[3]
	var envelope struct {
		Key     *string `json:"key"`
		Encrypt uint    `json:"encrypt"`
		Data    string  `json:"data"`
	}
	if err := json.Unmarshal(data[4:], &envelope); err != nil {
		return
	}
	switch fmt.Sprintf("%d_%d_%d", version, cmd, subCmd) {
	case "1_1_1":
		if envelope.Key != nil {
			l.mu.Lock()
			l.cipher = strings.TrimSpace(*envelope.Key)
			l.mu.Unlock()
		}
	case "1_501_0":
		l.handleMessages(envelope.Data, envelope.Encrypt, "user")
	case "1_521_0":
		l.handleMessages(envelope.Data, envelope.Encrypt, "group")
	}
}

func (l *Listener) handleMessages(data string, encType uint, threadType string) {
	l.mu.RLock()
	cipher := l.cipher
	l.mu.RUnlock()
	payload, err := l.decryptEventData(data, encType, cipher)
	if err != nil {
		return
	}
	var envelope struct {
		Data struct {
			Msgs []json.RawMessage `json:"msgs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return
	}
	for _, raw := range envelope.Data.Msgs {
		msg, ok := decodeInbound(raw, l.sess.UID, threadType)
		if !ok {
			continue
		}
		l.onMsg(msg)
	}
}

// decodeInbound reads the fields Zalo sends and skips our own echoes.
func decodeInbound(raw json.RawMessage, ownUID, threadType string) (InboundMessage, bool) {
	var wire struct {
		MsgID    string          `json:"msgId"`
		UIDFrom  string          `json:"uidFrom"`
		IDTo     string          `json:"idTo"`
		DName    string          `json:"dName"`
		TS       string          `json:"ts"`
		Content  json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return InboundMessage{}, false
	}
	if ownUID != "" && wire.UIDFrom == ownUID {
		return InboundMessage{}, false
	}
	text := decodeContentText(wire.Content)
	if strings.TrimSpace(text) == "" {
		return InboundMessage{}, false
	}
	thread := wire.IDTo
	if threadType == "group" && thread == "" {
		thread = wire.IDTo
	}
	return InboundMessage{
		ID: wire.MsgID, FromID: wire.UIDFrom, FromName: wire.DName,
		ThreadID: thread, ThreadType: threadType, Text: text,
		Timestamp: parseInt64(wire.TS),
	}, true
}

// decodeContentText accepts either a string or an object with a title.
func decodeContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Title   string `json:"title"`
		Href    string `json:"href"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if strings.TrimSpace(obj.Title) != "" {
			return obj.Title
		}
		return obj.Href
	}
	return ""
}

func parseInt64(s string) int64 {
	var out int64
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			break
		}
		out = out*10 + int64(c-'0')
	}
	return out
}

// decryptEventData handles the four encodings the listener can receive.
func (l *Listener) decryptEventData(data string, encType uint, cipherKey string) ([]byte, error) {
	var result []byte
	var err error
	switch encType {
	case 0:
		result = []byte(data)
	case 1:
		raw, derr := base64.StdEncoding.DecodeString(data)
		if derr != nil {
			return nil, derr
		}
		result, err = gunzip(raw)
	case 2:
		raw, derr := l.decryptGCM(data, cipherKey)
		if derr != nil {
			return nil, derr
		}
		result, err = gunzip(raw)
	case 3:
		result, err = l.decryptGCM(data, cipherKey)
	default:
		return nil, fmt.Errorf("unknown encryption type %d", encType)
	}
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(result) {
		return nil, errors.New("decrypted payload is not valid UTF-8")
	}
	return result, nil
}

// decryptGCM decodes the iv, aad and ciphertext Zalo prefixes on every
// encrypted event.
func (l *Listener) decryptGCM(data, cipherKey string) ([]byte, error) {
	if strings.TrimSpace(cipherKey) == "" {
		return nil, errors.New("cipher key required for encrypted data")
	}
	unescaped, err := url.PathUnescape(data)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(unescaped)
	if err != nil {
		return nil, err
	}
	if len(decoded) < 48 {
		return nil, fmt.Errorf("encrypted data too short (%d bytes)", len(decoded))
	}
	key, err := base64.StdEncoding.DecodeString(cipherKey)
	if err != nil {
		return nil, fmt.Errorf("decode cipher key: %w", err)
	}
	return DecodeAESGCM(key, decoded[0:16], decoded[16:32], decoded[32:])
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer r.Close()
	return io.ReadAll(r)
}
