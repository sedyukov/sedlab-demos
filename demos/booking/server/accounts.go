package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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
func (a *App) cardProviderFor(ctx context.Context, r *http.Request, b Booking) (pay.Provider, string, acc.Mode) {
	if a.acc == nil {
		return a.pay, "", acc.Mode{Reason: acc.ReasonDisabled}
	}
	tok := a.accToken(r)
	m := a.acc.Mode(ctx, acc.ModeInput{Token: tok, Resource: liveResource, HasKey: a.livePay != nil})
	if !m.Live {
		return a.pay, "", m
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
