package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	acc "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	pay "github.com/sedyukov/sedlab-modules/modules/payments/go"
)

// Live mode via the sedlab accounts service (RU edition only).
//
// The demo works on mocks for everyone. A visitor who signs in on auth.demo.sedlab.dev (Max / Yandex ID / VK ID)
// gets a few live runs per day: the deposit then goes through the real YooKassa TEST shop instead of the mock.
// accounts decides (mode) and accounts for it (consume, refund on failure); it fails closed to the mock.
// INTL edition: no accounts at all (a.acc == nil), always mock.

const liveResource = "yookassa-test"

// accToken takes the accounts token from the sl_auth cookie only: our own Mini App session uses the
// Authorization header, which must not be mistaken for an accounts token.
func (a *App) accToken(r *http.Request) string {
	if a.acc == nil {
		return ""
	}
	return a.acc.TokenFromRequest(http.Header{"Cookie": r.Header.Values("Cookie")})
}

// cardProviderFor decides mock ↔ live for one card prepayment and reserves a live unit.
// Returns the provider to use, the consumption id (to refund on failure) and the mode reason for the UI.
//
// Without a sign-in (or past the account limit) a visitor still gets the real YooKassa TEST shop, limited per IP
// and per day overall (owner decision 2026-11-23: test mode moves no money, and the demo is more convincing).
func (a *App) cardProviderFor(ctx context.Context, r *http.Request, b Booking) (pay.Provider, string, acc.Mode) {
	if a.livePay == nil {
		return a.pay, "", acc.Mode{Reason: acc.ReasonNoKey}
	}
	if a.acc == nil {
		return a.ipLive(clientIP(r), b.ID, acc.Mode{Reason: acc.ReasonDisabled})
	}
	tok := a.accToken(r)
	m := a.acc.Mode(ctx, acc.ModeInput{Token: tok, Resource: liveResource, HasKey: a.livePay != nil})
	if !m.Live {
		return a.ipLive(clientIP(r), b.ID, m)
	}
	d, err := a.acc.Consume(ctx, acc.ConsumeInput{Token: tok, Resource: liveResource, Units: 1,
		IdempotencyKey: fmt.Sprintf("booking-%d-%d", b.ID, b.CreatedAt)})
	if err != nil || !d.Allowed {
		m.Live = false
		if err != nil {
			m.Reason = acc.ReasonUnavailable
		} else {
			m.Reason = d.Reason
		}
		return a.pay, "", m
	}
	m.UserLeft = d.UserLeft
	return a.livePay, d.ConsumptionID, m
}

func (a *App) refundLiveUnit(ctx context.Context, consumptionID string) {
	if strings.HasPrefix(consumptionID, "ip:") {
		a.live.refund(strings.TrimPrefix(consumptionID, "ip:"))
		return
	}
	if a.acc == nil || consumptionID == "" {
		return
	}
	if _, err := a.acc.Refund(ctx, acc.RefundInput{ConsumptionID: consumptionID}); err != nil {
		a.log.Warn("accounts refund failed", "err", err)
	}
}

// payFor: the provider that took this booking's card payment (recorded at pay time).
func (a *App) payFor(b Booking) pay.Provider {
	if b.PayLive && a.livePay != nil {
		return a.livePay
	}
	return a.pay
}

// webhookProvider: the mock signs its webhooks with X-Payments-Mock-Signature; anything else is the live provider.
func (a *App) webhookProvider(headers map[string]string) pay.Provider {
	for k := range headers {
		if strings.EqualFold(k, "X-Payments-Mock-Signature") {
			return a.pay
		}
	}
	if a.livePay != nil {
		return a.livePay
	}
	return a.pay
}

// accountsUI tells the front-end what to show: "off" (intl), "stub" (button placeholder until auth is up)
// or "widget" (auth's widget.js).
func (a *App) accountsUI() string {
	if a.cfg.Edition != "ru" {
		return "off"
	}
	switch a.cfg.AccountsUI {
	case "widget", "off":
		return a.cfg.AccountsUI
	}
	return "stub"
}

// liveQuota: per-IP and global daily allowance of live (YooKassa TEST shop) payments for visitors without a sign-in.
// In memory only; IPs are kept as salted hashes for the current day (nothing about visitors is persisted).
type liveQuota struct {
	mu        sync.Mutex
	salt      []byte
	day       string
	perIP     map[string]int
	byBooking map[int64]string // booking → ip hash: re-paying the same booking does not count twice
	total     int
	maxIP     int
	maxDay    int
}

func newLiveQuota(maxIP, maxDay int) *liveQuota {
	salt := make([]byte, 16)
	rand.Read(salt)
	return &liveQuota{salt: salt, maxIP: maxIP, maxDay: maxDay}
}

func (q *liveQuota) roll(day string) {
	if q.day != day {
		q.day, q.perIP, q.byBooking, q.total = day, map[string]int{}, map[int64]string{}, 0
	}
}

// take returns (ok, left for this IP, reason when refused).
func (q *liveQuota) take(day, ip string, booking int64) (bool, int, string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.roll(day)
	h := sha256.Sum256(append(append([]byte{}, q.salt...), ip...))
	key := hex.EncodeToString(h[:8])
	if k, ok := q.byBooking[booking]; ok && k == key {
		return true, q.maxIP - q.perIP[key], ""
	}
	if q.total >= q.maxDay {
		return false, 0, acc.ReasonGlobalDaily
	}
	if q.perIP[key] >= q.maxIP {
		return false, 0, "ip_daily"
	}
	q.perIP[key]++
	q.total++
	q.byBooking[booking] = key
	return true, q.maxIP - q.perIP[key], ""
}

func (q *liveQuota) refund(bookingKey string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var id int64
	fmt.Sscan(bookingKey, &id)
	if key, ok := q.byBooking[id]; ok {
		delete(q.byBooking, id)
		if q.perIP[key] > 0 {
			q.perIP[key]--
		}
		if q.total > 0 {
			q.total--
		}
	}
}

// ipLive: live payment for a visitor without a sign-in, within the per-IP / daily quota; otherwise the mock.
// LIVE_PER_DAY=0 turns it off: then the accounts decision (fallback) stands as is.
func (a *App) ipLive(ip string, booking int64, fallback acc.Mode) (pay.Provider, string, acc.Mode) {
	if a.live == nil || a.live.maxDay <= 0 {
		return a.pay, "", fallback
	}
	ok, left, reason := a.live.take(a.now().In(a.cfg.TZ).Format(time.DateOnly), ip, booking)
	if !ok {
		return a.pay, "", acc.Mode{Reason: reason}
	}
	return a.livePay, fmt.Sprintf("ip:%d", booking), acc.Mode{Live: true, Reason: acc.ReasonOK, UserLeft: int64(left)}
}
