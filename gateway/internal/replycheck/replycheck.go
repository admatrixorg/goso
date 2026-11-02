// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package replycheck is the pre-send check on customer replies (bridge reply
// path). Before a drafted reply goes back to Omni-Core for sending, it:
//
//  1. anonymizes the draft in code (phones, e-mails, personal names), which is
//     mandatory before anything is sent to Cloudflare Clef;
//  2. cross-checks, in code, every concrete code, money amount, percentage and
//     date in the draft against two kinds of source. SHOP FACTS (agent
//     instructions, knowledge, vault) back anything. CUSTOMER WORDS (the
//     customer's messages and memory note) back only what the customer may
//     have told us: order/reference codes, phones, appointment dates. Offers
//     (voucher/discount/gift codes, money, percentages, promotion deadlines)
//     are verified only by SHOP FACTS;
//  3. asks Clef (one call, English questions, masked text pasted into each)
//     whether the draft states an offer/price/date, whether such a detail is
//     unsupported by the shop facts, and whether it leaks another customer's
//     information.
//
// A draft is HELD for staff (never sent to the customer) when step 2 finds an
// unverified code/price/percent, or a phone/e-mail that belongs to neither the
// shop nor this customer, or when Clef flags an unsupported offer or a leak.
//
// GOSO_REPLY_CHECK=off (default) keeps today's behaviour; shadow checks and
// logs but never holds; on/enforce holds. When Clef fails or exceeds the
// timeout, GOSO_REPLY_CHECK_ON_ERROR=hold (default, fail-closed) holds the
// reply; =send sends it with only the code checks and the fix31 prompt
// guardrail behind it.
package replycheck

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
)

// Mode is the feature flag.
type Mode int

const (
	ModeOff Mode = iota
	ModeShadow
	ModeEnforce
)

func (m Mode) String() string {
	switch m {
	case ModeShadow:
		return "shadow"
	case ModeEnforce:
		return "enforce"
	}
	return "off"
}

// Config is read from the environment by FromEnv.
type Config struct {
	Mode           Mode
	HoldOnError    bool
	Timeout        time.Duration
	URL            string
	Token          string
	Model          string
	OfferThreshold float64 // Clef "states an offer" + an unverified date ⇒ hold
	UnsupThreshold float64 // Clef "unsupported by shop facts" ⇒ hold
	LeakThreshold  float64 // Clef "other customer's info" ⇒ hold
	FactsMaxBytes  int
}

const (
	defaultTimeout = 900 * time.Millisecond
	defaultModel   = "clef"
)

// FromEnv reads:
//
//	GOSO_REPLY_CHECK              off (default) | shadow | on
//	GOSO_REPLY_CHECK_ON_ERROR     hold (default) | send
//	GOSO_REPLY_CHECK_TIMEOUT_MS   Clef budget per reply, default 900
//	GOSO_REPLY_CHECK_UNSUPPORTED_THRESHOLD / _LEAK_THRESHOLD / _OFFER_THRESHOLD
//	CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_AUTH_TOKEN   Clef credentials (never logged)
//	GOSO_CLEF_URL                 full endpoint override (tests/staging proxy)
func FromEnv() Config {
	c := Config{
		HoldOnError:    true,
		Timeout:        defaultTimeout,
		Model:          defaultModel,
		OfferThreshold: 0.7,
		UnsupThreshold: 0.5,
		LeakThreshold:  0.5,
		FactsMaxBytes:  3000,
	}
	switch strings.ToLower(env("GOSO_REPLY_CHECK")) {
	case "1", "on", "true", "yes", "enforce", "hold":
		c.Mode = ModeEnforce
	case "shadow", "log", "dry-run", "dryrun":
		c.Mode = ModeShadow
	}
	switch strings.ToLower(env("GOSO_REPLY_CHECK_ON_ERROR")) {
	case "send", "open", "fail-open", "allow":
		c.HoldOnError = false
	}
	if n, err := strconv.Atoi(env("GOSO_REPLY_CHECK_TIMEOUT_MS")); err == nil && n > 0 && n <= 10000 {
		c.Timeout = time.Duration(n) * time.Millisecond
	}
	c.OfferThreshold = envFloat("GOSO_REPLY_CHECK_OFFER_THRESHOLD", c.OfferThreshold)
	c.UnsupThreshold = envFloat("GOSO_REPLY_CHECK_UNSUPPORTED_THRESHOLD", c.UnsupThreshold)
	c.LeakThreshold = envFloat("GOSO_REPLY_CHECK_LEAK_THRESHOLD", c.LeakThreshold)
	c.Token = env("CLOUDFLARE_AUTH_TOKEN")
	if u := env("GOSO_CLEF_URL"); u != "" {
		c.URL = u
	} else if acct := env("CLOUDFLARE_ACCOUNT_ID"); acct != "" {
		c.URL = "https://api.cloudflare.com/client/v4/accounts/" + acct + "/ai/run/@cf/cloudflare/clef"
	}
	return c
}

func env(k string) string { return config.Lookup(k) }

func envFloat(k string, def float64) float64 {
	v, err := strconv.ParseFloat(env(k), 64)
	if err != nil || v <= 0 || v >= 1 {
		return def
	}
	return v
}

// Checker runs the pre-send check. The zero value is off.
type Checker struct {
	cfg    Config
	client *http.Client
	once   sync.Once
}

// New returns a checker for cfg. client may be nil.
func New(cfg Config, client *http.Client) *Checker {
	return &Checker{cfg: cfg, client: client}
}

// Config returns the checker's configuration.
func (c *Checker) Config() Config {
	if c == nil {
		return Config{}
	}
	return c.cfg
}

// Enabled reports whether checks run at all (shadow or enforce).
func (c *Checker) Enabled() bool { return c != nil && c.cfg.Mode != ModeOff }

func (c *Checker) http() *http.Client {
	if c.client != nil {
		return c.client
	}
	c.once.Do(func() {
		c.client = &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		}}
	})
	return c.client
}

// Input is one drafted reply plus what it may rely on.
type Input struct {
	Reply string
	// Shop is the agent's instructions and knowledge/vault text.
	Shop []string
	// Memory is this customer's memory note. It counts as CUSTOMER WORDS:
	// it never verifies an offer.
	Memory []string
	// Customer is this customer's own words (current prompt, user turns of
	// the history) and display name, if known. It tells the customer's own
	// phone/name from someone else's and lets the reply echo the customer's
	// order/reference codes and appointment dates; it never verifies an
	// offer, price or percentage.
	Customer []string
}

// Verdict is the outcome. Reasons never contain phone numbers, e-mails or
// names; codes/amounts appear as the reply wrote them.
type Verdict struct {
	Hold        bool
	Reasons     []string
	Masked      string
	Offer       float64 // -1 when Clef was not asked
	Unsupported float64
	Leak        float64
	ClefError   string
	ClefCalled  bool
	InputTokens int
	Latency     time.Duration
}

// Check runs the check. With Mode off it returns a zero Verdict immediately.
func (c *Checker) Check(ctx context.Context, in Input) (v Verdict) {
	v = Verdict{Offer: -1, Unsupported: -1, Leak: -1}
	if !c.Enabled() || strings.TrimSpace(in.Reply) == "" {
		return v
	}
	start := time.Now()
	defer func() { v.Latency = time.Since(start) }()

	known := newKnownSides(in.Shop, append(append([]string{}, in.Customer...), in.Memory...))
	an := anonymize(in.Reply, known)
	v.Masked = an.Text
	if an.OtherPhones > 0 {
		v.Reasons = append(v.Reasons, "other_person_phone")
	}
	if an.OtherEmails > 0 {
		v.Reasons = append(v.Reasons, "other_person_email")
	}

	// SHOP FACTS back every fact; CUSTOMER WORDS (their messages and memory
	// note) back only order/reference codes and non-offer dates.
	shopFacts := strings.Join(in.Shop, "\n")
	customerWords := strings.Join(append(append([]string{}, in.Customer...), in.Memory...), "\n")
	var softDates []fact
	for _, f := range unverified(extractFacts(in.Reply), shopFacts, customerWords) {
		if f.Kind == "date" {
			softDates = append(softDates, f)
			continue
		}
		v.Reasons = append(v.Reasons, "unverified_"+f.Kind+":"+f.Raw)
	}
	if len(v.Reasons) > 0 {
		// Code already proved an invented/leaked detail: hold without
		// spending a Clef call or its latency.
		v.Hold = true
		return v
	}

	// Clef judges offers against SHOP FACTS only: customer words and memory
	// are never passed as support for an offer.
	excerpt := groundingExcerpt(shopFacts, in.Reply, c.cfg.FactsMaxBytes)
	maskedFacts := anonymize(excerpt, known).Text
	cctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	v.ClefCalled = true
	sc, err := c.callClef(cctx, an.Text, maskedFacts)
	if err != nil {
		v.ClefError = err.Error()
		if len(softDates) > 0 {
			v.Reasons = append(v.Reasons, "unverified_date:"+softDates[0].Raw)
		}
		if c.cfg.HoldOnError {
			v.Reasons = append(v.Reasons, "clef_unavailable:"+v.ClefError)
		}
		v.Hold = len(v.Reasons) > 0
		return v
	}
	v.Offer, v.Unsupported, v.Leak, v.InputTokens = sc.Offer, sc.Unsupported, sc.Leak, sc.InputTokens
	if sc.Leak >= c.cfg.LeakThreshold {
		v.Reasons = append(v.Reasons, "clef_other_customer_leak")
	}
	if sc.Unsupported >= c.cfg.UnsupThreshold {
		v.Reasons = append(v.Reasons, "clef_unsupported_offer")
	}
	if len(softDates) > 0 && sc.Offer >= c.cfg.OfferThreshold {
		v.Reasons = append(v.Reasons, "unverified_date:"+softDates[0].Raw)
	}
	v.Hold = len(v.Reasons) > 0
	return v
}
