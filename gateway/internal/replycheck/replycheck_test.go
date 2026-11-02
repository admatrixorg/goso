// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package replycheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testShop = "Sofa Milan: 12.500.000đ. Giảm 10% sofa Milan với mã MOCAN11 đến 30/11. Hotline 1900 6868. Nội thành giao 2-4 ngày."

func TestAnonymizeSides(t *testing.T) {
	k := newKnownSides([]string{testShop}, []string{"Chị Hoa, sđt 0901 111 222"})
	r := anonymize("Dạ chị Hoa, em gọi 0901.111.222 hoặc hotline 1900 6868; anh Bình 0977 555 666, mail b@x.vn", k)
	for _, want := range []string{"chị [TÊN_KHÁCH]", "[SĐT_KHÁCH]", "[SĐT_SHOP]", "anh [TÊN_NGƯỜI_KHÁC] [SĐT_NGƯỜI_KHÁC]", "[EMAIL_NGƯỜI_KHÁC]"} {
		if !strings.Contains(r.Text, want) {
			t.Fatalf("missing %q in %q", want, r.Text)
		}
	}
	for _, raw := range []string{"0901", "0977", "Bình", "Hoa", "b@x.vn"} {
		if strings.Contains(r.Text, raw) {
			t.Fatalf("raw %q left in %q", raw, r.Text)
		}
	}
	if r.OtherPhones != 1 || r.OtherEmails != 1 || r.OtherNames != 1 {
		t.Fatalf("counts %+v", r)
	}
	// Prices are not phone numbers.
	if got := anonymize("giá 12.500.000đ hoặc 1.500.000.000đ", k).Text; strings.Contains(got, "[SĐT") {
		t.Fatalf("price masked as phone: %q", got)
	}
}

func TestExtractFacts(t *testing.T) {
	keys := func(s string) string {
		var out []string
		for _, f := range extractFacts(s) {
			out = append(out, f.Kind+":"+f.Key)
		}
		return strings.Join(out, " ")
	}
	cases := map[string]string{
		"giá 12.500.000đ, nhập mã MOCAN11 giảm 10%":         "money:12500000 percent:10 code:MOCAN11",
		"chỉ 1tr5 hoặc 500k, ship 12,5 triệu":               "money:1500000 money:500000 money:12500000",
		"giao ngày 15/11, nội thành 2-4 ngày":               "date:15/11",
		"đơn MA-1043 đã giao, căn 1205, năm 2026":           "code:MA-1043",
		"voucher VIPHOA200 của chị, hotline 1900 6868":      "code:VIPHOA200",
		"ngày 5 tháng 12 có mã quà tặng GIFTNOV":            "code:GIFTNOV date:5/12",
		"Dạ em chào chị, cần tư vấn mẫu nào ạ? 2 khách đặt": "",
	}
	for in, want := range cases {
		if got := keys(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestUnverified(t *testing.T) {
	got := unverified(extractFacts("Sofa 12.500.000đ (12,5 triệu), mã mocan11 giảm 10%, giao 30/11; ghế 1tr6, mã MOCAN20"), testShop)
	var raw []string
	for _, f := range got {
		raw = append(raw, f.Raw)
	}
	if strings.Join(raw, "|") != "1tr6|MOCAN20" {
		t.Fatalf("unverified = %v", raw)
	}
}

// fakeClef records request bodies and answers with fixed scores.
type fakeClef struct {
	srv                      *httptest.Server
	calls                    atomic.Int32
	bodies                   chan string
	offer, unsupported, leak float64
	delay                    time.Duration
	status                   int
}

func newFakeClef(t *testing.T) *fakeClef {
	f := &fakeClef{bodies: make(chan string, 16), status: 200}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(401)
			return
		}
		b, _ := io.ReadAll(r.Body)
		select {
		case f.bodies <- string(b):
		default:
		}
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		if f.status != 200 {
			w.WriteHeader(f.status)
			return
		}
		ans := func(p float64) map[string]any { return map[string]any{"type": "noul", "noul": p} }
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{
			"answers": map[string]any{qOffer: ans(f.offer), qUnsupported: ans(f.unsupported), qLeak: ans(f.leak)},
			"usage":   map[string]any{"input_tokens": 321},
		}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func testCfg(url string) Config {
	t := Config{Mode: ModeEnforce, HoldOnError: true, Timeout: 300 * time.Millisecond, URL: url, Token: "test-token",
		Model: "clef", OfferThreshold: 0.7, UnsupThreshold: 0.5, LeakThreshold: 0.5, FactsMaxBytes: 3000}
	return t
}

func in(reply string) Input {
	return Input{Reply: reply, Shop: []string{testShop}, Memory: []string{"Khách chị Hoa, 0901 111 222"}, Customer: []string{"Shop ơi"}}
}

func TestCheckOffSkipsEverything(t *testing.T) {
	f := newFakeClef(t)
	cfg := testCfg(f.srv.URL)
	cfg.Mode = ModeOff
	v := New(cfg, nil).Check(context.Background(), in("mã MOCAN99 giảm 50%"))
	if v.Hold || v.ClefCalled || f.calls.Load() != 0 {
		t.Fatalf("off mode acted: %+v", v)
	}
	var nilChecker *Checker
	if nilChecker.Enabled() || nilChecker.Check(context.Background(), in("x")).Hold {
		t.Fatal("nil checker must be off")
	}
}

func TestCheckCodeFlagHoldsWithoutClef(t *testing.T) {
	f := newFakeClef(t)
	v := New(testCfg(f.srv.URL), nil).Check(context.Background(), in("Dạ nhập mã MOCAN20 giảm 20% ạ"))
	if !v.Hold || v.ClefCalled || f.calls.Load() != 0 {
		t.Fatalf("want code-only hold: %+v calls=%d", v, f.calls.Load())
	}
	if strings.Join(v.Reasons, ",") != "unverified_percent:20%,unverified_code:MOCAN20" {
		t.Fatalf("reasons %v", v.Reasons)
	}
	v = New(testCfg(f.srv.URL), nil).Check(context.Background(), in("Dạ chị gọi anh Bình 0977 555 666 nhé"))
	if !v.Hold || v.Reasons[0] != "other_person_phone" {
		t.Fatalf("other phone: %+v", v)
	}
}

func TestCheckClefDecisions(t *testing.T) {
	f := newFakeClef(t)
	ck := New(testCfg(f.srv.URL), nil)
	f.offer, f.unsupported, f.leak = 0.95, 0.04, 0.02
	v := ck.Check(context.Background(), in("Dạ chị Hoa ơi, sofa Milan giá 12.500.000đ, hotline 1900 6868 ạ"))
	if v.Hold || !v.ClefCalled || v.InputTokens != 321 {
		t.Fatalf("grounded reply held: %+v", v)
	}
	body := <-f.bodies
	if strings.Contains(body, "Hoa") || strings.Contains(body, "1900 6868") || !strings.Contains(body, "[TÊN_KHÁCH]") || !strings.Contains(body, "[SĐT_SHOP]") {
		t.Fatalf("Clef saw unmasked text: %s", body)
	}
	if !strings.Contains(body, `"shop_facts"`) || !strings.Contains(body, `"type":"noul"`) {
		t.Fatalf("questions shape: %s", body)
	}

	f.unsupported = 0.9
	if v := ck.Check(context.Background(), in("Dạ mua sofa tặng bàn trà ạ")); !v.Hold || v.Reasons[0] != "clef_unsupported_offer" {
		t.Fatalf("unsupported: %+v", v)
	}
	f.unsupported, f.leak = 0.04, 0.9
	if v := ck.Check(context.Background(), in("Dạ có khách tên Phương vừa mua ạ")); !v.Hold || v.Reasons[0] != "clef_other_customer_leak" {
		t.Fatalf("leak: %+v", v)
	}
	// An unverified date holds only when Clef reads the reply as a promise.
	f.leak, f.offer = 0.02, 0.9
	if v := ck.Check(context.Background(), in("Dạ đơn giao ngày 5/11 ạ")); !v.Hold || v.Reasons[0] != "unverified_date:5/11" {
		t.Fatalf("date: %+v", v)
	}
	f.offer = 0.1
	if v := ck.Check(context.Background(), in("Dạ hôm 5/11 chị có nhắn shop ạ")); v.Hold {
		t.Fatalf("date mention held: %+v", v)
	}
}

func TestCheckClefFailureFailsClosed(t *testing.T) {
	f := newFakeClef(t)
	f.delay = 2 * time.Second
	ck := New(testCfg(f.srv.URL), nil)
	start := time.Now()
	v := ck.Check(context.Background(), in("Dạ em chào chị ạ"))
	if el := time.Since(start); el > 600*time.Millisecond {
		t.Fatalf("timeout not enforced: %v", el)
	}
	if !v.Hold || v.ClefError != "clef_timeout" || v.Reasons[0] != "clef_unavailable:clef_timeout" {
		t.Fatalf("timeout must hold: %+v", v)
	}

	f.delay, f.status = 0, 500
	if v := ck.Check(context.Background(), in("Dạ em chào chị ạ")); !v.Hold || v.ClefError != "clef_http_500" {
		t.Fatalf("5xx must hold: %+v", v)
	}
	cfg := testCfg("")
	cfg.Token = ""
	if v := New(cfg, nil).Check(context.Background(), in("Dạ em chào chị ạ")); !v.Hold || v.ClefError != "clef_not_configured" {
		t.Fatalf("missing creds must hold: %+v", v)
	}
}

func TestCheckClefFailureOnErrorSend(t *testing.T) {
	f := newFakeClef(t)
	f.status = 503
	cfg := testCfg(f.srv.URL)
	cfg.HoldOnError = false
	ck := New(cfg, nil)
	if v := ck.Check(context.Background(), in("Dạ em chào chị ạ")); v.Hold || v.ClefError != "clef_http_503" {
		t.Fatalf("on_error=send must pass a clean reply: %+v", v)
	}
	// Code checks still hold without Clef.
	if v := ck.Check(context.Background(), in("Dạ mã MOCAN99 nhé")); !v.Hold {
		t.Fatalf("code check skipped: %+v", v)
	}
	// So does an unverified date when Clef cannot vouch for it.
	if v := ck.Check(context.Background(), in("Dạ giao ngày 5/11 ạ")); !v.Hold || v.Reasons[0] != "unverified_date:5/11" {
		t.Fatalf("date with Clef down: %+v", v)
	}
}

func TestFromEnv(t *testing.T) {
	for _, k := range []string{"GOSO_REPLY_CHECK", "GOSO_REPLY_CHECK_ON_ERROR", "GOSO_REPLY_CHECK_TIMEOUT_MS", "GOSO_CLEF_URL",
		"GOSO_REPLY_CHECK_LEAK_THRESHOLD", "GOSO_REPLY_CHECK_UNSUPPORTED_THRESHOLD", "GOSO_REPLY_CHECK_OFFER_THRESHOLD"} {
		t.Setenv(k, "")
	}
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct")
	t.Setenv("CLOUDFLARE_AUTH_TOKEN", "tok")
	c := FromEnv()
	if c.Mode != ModeOff || !c.HoldOnError || c.Timeout != 900*time.Millisecond || c.Model != "clef" ||
		c.URL != "https://api.cloudflare.com/client/v4/accounts/acct/ai/run/@cf/cloudflare/clef" {
		t.Fatalf("defaults %+v", c)
	}
	t.Setenv("GOSO_REPLY_CHECK", "on")
	t.Setenv("GOSO_REPLY_CHECK_ON_ERROR", "send")
	t.Setenv("GOSO_REPLY_CHECK_TIMEOUT_MS", "700")
	t.Setenv("GOSO_REPLY_CHECK_LEAK_THRESHOLD", "0.6")
	t.Setenv("GOSO_CLEF_URL", "http://clef.test/run")
	c = FromEnv()
	if c.Mode != ModeEnforce || c.HoldOnError || c.Timeout != 700*time.Millisecond || c.LeakThreshold != 0.6 || c.URL != "http://clef.test/run" {
		t.Fatalf("parsed %+v", c)
	}
	t.Setenv("GOSO_REPLY_CHECK", "shadow")
	if FromEnv().Mode != ModeShadow {
		t.Fatal("shadow")
	}
}
