package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	pay "github.com/sedyukov/sedlab-modules/modules/payments/go"
	tg "github.com/sedyukov/sedlab-modules/modules/telegram-core/go"
)

// ---------- prepayment ----------

// POST /api/bookings/{id}/pay {method: yookassa|stars}
// yookassa → {confirmation_url}: card payment via the payments module (mock checkout in the demo)
// stars    → {invoice_link}: Telegram Stars invoice for WebApp.openInvoice (telegram-core)
func (a *App) handlePay(w http.ResponseWriter, r *http.Request) {
	b, ok := a.ownBooking(w, r)
	if !ok {
		return
	}
	var in struct {
		Method string `json:"method"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !slices.Contains(a.cfg.PayMethods, in.Method) {
		writeErr(w, 400, "invalid_argument", "payment method is not enabled")
		return
	}
	now := a.now().Unix()
	if b.Status != "pending" || b.HoldUntil <= now {
		writeErr(w, 409, "conflict", T(b.Lang, "pay_fail"))
		return
	}
	svc, _ := a.store.Service(b.ServiceID)
	m, _ := a.store.Master(b.MasterID)
	desc := fmt.Sprintf("%s — %s, %s", svc.Name(b.Lang), m.Name(b.Lang), fmtWhen(b.Lang, time.Unix(b.StartAt, 0).In(a.cfg.TZ)))
	hold := now + int64(a.cfg.HoldMinutes)*60 // give the payer a fresh hold window

	switch in.Method {
	case "yookassa":
		amount := a.deposit(svc.PriceRub, 100)
		p, err := a.pay.CreatePayment(r.Context(), pay.CreatePaymentInput{
			Amount:         pay.Money{Value: amount, Currency: "RUB"},
			Description:    truncate(desc, 128),
			IdempotencyKey: fmt.Sprintf("booking-%d-%d", b.ID, b.CreatedAt),
			ReturnURL:      fmt.Sprintf("%s/app/?booking=%d", a.cfg.PublicURL, b.ID),
			Metadata:       map[string]string{"booking_id": strconv.FormatInt(b.ID, 10)},
		})
		if err != nil {
			a.log.Error("create payment", "err", err)
			writeErr(w, 502, "provider_error", "payment provider error")
			return
		}
		if err := a.store.SetPaymentStarted(b.ID, "yookassa", "RUB", amount, p.ID, hold); err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"method": "yookassa", "confirmation_url": p.ConfirmationURL, "amount": amount, "currency": "RUB"})
	case "stars":
		amount := a.deposit(svc.PriceStars, 1)
		link, err := a.msg.StarsInvoiceLink(r.Context(), StarsInvoice{
			Title: truncate(svc.Name(b.Lang), 32), Description: truncate(desc, 255),
			Payload: starsPayload(b.ID), Amount: amount,
		}, sessionFrom(r).Emulated)
		if err != nil {
			a.log.Error("stars invoice", "err", err)
			writeErr(w, 502, "provider_error", "telegram error")
			return
		}
		if err := a.store.SetPaymentStarted(b.ID, "stars", "XTR", amount, "", hold); err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"method": "stars", "invoice_link": link, "amount": amount, "currency": "XTR"})
	}
}

func starsPayload(id int64) string { return "booking:" + strconv.FormatInt(id, 10) }

func parseStarsPayload(p string) int64 {
	id, _ := strconv.ParseInt(strings.TrimPrefix(p, "booking:"), 10, 64)
	return id
}

// confirmPaid is the single place where a booking becomes confirmed (idempotent).
func (a *App) confirmPaid(ctx context.Context, bookingID int64, payID string) error {
	ok, err := a.store.Confirm(bookingID, payID, a.now().Unix())
	if err != nil || !ok {
		return err // already confirmed (duplicate webhook) or not pending → no-op
	}
	b, err := a.store.Booking(bookingID)
	if err != nil {
		return err
	}
	svc, _ := a.store.Service(b.ServiceID)
	m, _ := a.store.Master(b.MasterID)
	when := fmtWhen(b.Lang, time.Unix(b.StartAt, 0).In(a.cfg.TZ))
	a.tell(ctx, b.UserID, "confirmed", T(b.Lang, "confirmed", svc.Name(b.Lang), m.Name(b.Lang), when, fmtMoney(b.PayCurrency, b.PayAmount)),
		&WebAppButton{Text: T(b.Lang, "my_open"), URL: a.cfg.PublicURL + "/app/?screen=my"}, fmt.Sprintf("confirmed-%d", b.ID))
	for _, owner := range a.ownerChats(b) {
		lang := a.store.UserLang(owner)
		a.tell(ctx, owner, "owner_new", T(lang, "owner_new", b.ID, html.EscapeString(b.ClientName), svc.Name(lang), m.Name(lang), fmtWhen(lang, time.Unix(b.StartAt, 0).In(a.cfg.TZ)), fmtMoney(b.PayCurrency, b.PayAmount)),
			nil, fmt.Sprintf("owner-%d-%d", owner, b.ID))
	}
	a.log.Info("booking confirmed", "id", b.ID, "method", b.PayMethod)
	return nil
}

// owners to notify: real admins; for emulator bookings — the emulator owner persona (keeps the demo chat lively).
func (a *App) ownerChats(b Booking) []int64 {
	if isEmulatorUser(b.UserID) {
		return []int64{DemoOwnerID}
	}
	return a.cfg.AdminIDs
}

// cancelBooking cancels and (optionally) refunds the prepayment, then tells the client.
func (a *App) cancelBooking(ctx context.Context, b Booking, refund bool, by string) (Booking, error) {
	ok, err := a.store.SetStatus(b.ID, []string{"pending", "confirmed"}, "cancelled")
	if err != nil {
		return b, err
	}
	if !ok {
		return a.store.Booking(b.ID)
	}
	refundNote := ""
	if refund && b.PaidAt > 0 && !b.Refunded {
		var rerr error
		switch b.PayMethod {
		case "yookassa":
			_, rerr = a.pay.Refund(ctx, pay.RefundInput{PaymentID: b.PayID, IdempotencyKey: fmt.Sprintf("refund-booking-%d", b.ID)})
		case "stars":
			if isEmulatorUser(b.UserID) {
				rerr = nil // emulator payment: nothing was charged in Telegram
			} else {
				rerr = a.msg.RefundStars(ctx, b.UserID, b.PayID)
			}
		}
		if rerr != nil {
			a.log.Error("refund failed", "booking", b.ID, "err", rerr)
		} else {
			a.store.MarkRefunded(b.ID)
			refundNote = T(b.Lang, "refunded", fmtMoney(b.PayCurrency, b.PayAmount))
		}
	}
	if b.Status == "confirmed" {
		svc, _ := a.store.Service(b.ServiceID)
		m, _ := a.store.Master(b.MasterID)
		a.tell(ctx, b.UserID, "cancelled", T(b.Lang, "cancelled", svc.Name(b.Lang), m.Name(b.Lang), fmtWhen(b.Lang, time.Unix(b.StartAt, 0).In(a.cfg.TZ)), refundNote),
			&WebAppButton{Text: T(b.Lang, "open"), URL: a.cfg.PublicURL + "/app/"}, fmt.Sprintf("cancelled-%d", b.ID))
	}
	a.log.Info("booking cancelled", "id", b.ID, "by", by, "refund", refund)
	return a.store.Booking(b.ID)
}

// ---------- webhooks ----------

// POST /payments/webhook — payments module verifies (signature / re-fetch) and dedupes.
func (a *App) handlePaymentsWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		writeErr(w, 400, "invalid_argument", "read body")
		return
	}
	hdr := headersMap(r.Header)
	hdr["Remote-Addr"] = clientIP(r)
	if err := a.processPaymentWebhook(r.Context(), hdr, body); err != nil {
		a.log.Warn("payments webhook rejected", "err", err)
		writeErr(w, 400, "signature_invalid", "rejected")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) processPaymentWebhook(ctx context.Context, headers map[string]string, body []byte) error {
	ev, err := a.pay.ParseWebhook(ctx, headers, body)
	if err != nil {
		return err
	}
	if ev.Duplicate {
		return nil
	}
	b, err := a.store.BookingByPayID(ev.PaymentID)
	if err != nil {
		a.log.Warn("payment for unknown booking", "payment", ev.PaymentID)
		return nil // ack: nothing to do
	}
	switch ev.Type {
	case "payment.succeeded":
		if b.Status == "pending" {
			return a.confirmPaid(ctx, b.ID, ev.PaymentID)
		}
		if b.Status == "expired" || b.Status == "cancelled" {
			// paid after the hold expired: refund automatically
			_, err := a.pay.Refund(ctx, pay.RefundInput{PaymentID: ev.PaymentID, IdempotencyKey: fmt.Sprintf("late-refund-%d", b.ID)})
			if err == nil {
				a.store.MarkRefunded(b.ID)
			}
			return err
		}
	case "payment.canceled", "payment.failed":
		a.log.Info("payment not completed", "booking", b.ID, "type", ev.Type)
	}
	return nil
}

// POST /tg/webhook — telegram-core checks the secret header and dedupes update_id.
func (a *App) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		writeErr(w, 400, "invalid_argument", "read body")
		return
	}
	if err := a.processUpdate(r.Context(), headersMap(r.Header), body); err != nil {
		a.log.Warn("telegram update rejected", "err", err)
		writeErr(w, 400, "rejected", "rejected")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) processUpdate(ctx context.Context, headers map[string]string, body []byte) error {
	u, err := a.msg.ParseUpdate(ctx, headers, body)
	if err != nil {
		return err
	}
	if u.Duplicate {
		return nil
	}
	lang := a.store.UserLang(u.UserID)
	switch u.Kind {
	case "command":
		switch u.Command {
		case "start":
			a.store.db.Exec(`INSERT OR IGNORE INTO users(id,lang,created_at) VALUES(?,?,?)`, u.UserID, lang, a.now().Unix())
			a.tell(ctx, u.ChatID, "welcome", T(lang, "welcome", a.firstName(u.UserID)), &WebAppButton{Text: T(lang, "open"), URL: a.cfg.PublicURL + "/app/"}, "")
		case "admin":
			if a.roleFor(u.UserID, isEmulatorUser(u.UserID)) == "owner" {
				a.tell(ctx, u.ChatID, "admin", "🛠", &WebAppButton{Text: "Admin", URL: a.cfg.PublicURL + "/admin/"}, "")
			} else {
				a.tell(ctx, u.ChatID, "help", T(lang, "help"), nil, "")
			}
		case "my":
			a.tell(ctx, u.ChatID, "my", a.upcomingText(u.UserID, lang), &WebAppButton{Text: T(lang, "my_open"), URL: a.cfg.PublicURL + "/app/?screen=my"}, "")
		default:
			a.tell(ctx, u.ChatID, "help", T(lang, "help"), nil, "")
		}
	case "message":
		a.tell(ctx, u.ChatID, "help", T(lang, "help"), &WebAppButton{Text: T(lang, "open"), URL: a.cfg.PublicURL + "/app/"}, "")
	case "pre_checkout_query":
		// Telegram waits ≤10 s for this answer: check the booking is still payable.
		okPay, msg := a.checkStarsPayable(u.UserID, u.Payment)
		if isEmulatorUser(u.UserID) { // simulated by the emulator: no real query to answer
			if !okPay {
				return fmt.Errorf("pre-checkout declined: %s", msg)
			}
			return nil
		}
		return a.msg.AnswerPreCheckout(ctx, u.PreCheckoutQueryID, okPay, msg)
	case "successful_payment":
		if u.Payment == nil || u.Payment.Currency != "XTR" {
			return nil
		}
		id := parseStarsPayload(u.Payment.Payload)
		b, err := a.store.Booking(id)
		if err != nil || b.UserID != u.UserID {
			return nil
		}
		if b.Status != "pending" { // paid too late: give the Stars back
			if !isEmulatorUser(u.UserID) {
				a.msg.RefundStars(ctx, u.UserID, u.Payment.TelegramPaymentChargeID)
			}
			return nil
		}
		return a.confirmPaid(ctx, id, u.Payment.TelegramPaymentChargeID)
	}
	return nil
}

func (a *App) checkStarsPayable(userID int64, p *tg.Payment) (bool, string) {
	lang := a.store.UserLang(userID)
	if p == nil || p.Currency != "XTR" {
		return false, "Unsupported currency"
	}
	b, err := a.store.Booking(parseStarsPayload(p.Payload))
	if err != nil || b.UserID != userID {
		return false, "Booking not found"
	}
	if b.Status != "pending" || b.HoldUntil <= a.now().Unix() || b.PayMethod != "stars" {
		return false, T(lang, "pay_fail")
	}
	if p.TotalAmount != b.PayAmount {
		return false, "Amount mismatch"
	}
	return true, ""
}

func (a *App) firstName(id int64) string {
	var n string
	a.store.db.QueryRow(`SELECT first_name FROM users WHERE id=?`, id).Scan(&n)
	if n == "" {
		n = "👋"
	}
	return html.EscapeString(n)
}

func (a *App) upcomingText(userID int64, lang string) string {
	bs, _ := a.store.queryBookings(`user_id=? AND status='confirmed' AND start_at>? ORDER BY start_at LIMIT 5`, userID, a.now().Unix())
	if len(bs) == 0 {
		return T(lang, "none")
	}
	var sb strings.Builder
	sb.WriteString(T(lang, "upcoming"))
	for _, b := range bs {
		svc, _ := a.store.Service(b.ServiceID)
		m, _ := a.store.Master(b.MasterID)
		fmt.Fprintf(&sb, "\n• %s — %s, %s", fmtWhen(lang, time.Unix(b.StartAt, 0).In(a.cfg.TZ)), svc.Name(lang), m.Name(lang))
	}
	return sb.String()
}

// ---------- background: holds, reminders, completion ----------

func (a *App) tick(ctx context.Context) {
	now := a.now().Unix()
	if n, err := a.store.ExpireHolds(now); err == nil && n > 0 {
		a.log.Info("holds expired", "n", n)
	}
	a.store.CompletePast(now)
	a.sendReminders(ctx)
	if a.cfg.DemoReset {
		day := a.now().In(a.cfg.TZ).Format("2006-01-02")
		if last := a.store.Meta("reset_day"); last != day && a.now().In(a.cfg.TZ).Hour() >= 4 {
			if err := a.ResetDemo(); err != nil {
				a.log.Error("demo reset", "err", err)
			}
		}
	}
}

func (a *App) sendReminders(ctx context.Context) int {
	sent := 0
	for _, win := range []string{"24h", "2h"} {
		bs, err := a.store.DueReminders(a.now().Unix(), win)
		if err != nil {
			a.log.Error("reminders", "err", err)
			continue
		}
		for _, b := range bs {
			svc, _ := a.store.Service(b.ServiceID)
			m, _ := a.store.Master(b.MasterID)
			t := time.Unix(b.StartAt, 0).In(a.cfg.TZ).Format("15:04")
			key := map[string]string{"24h": "remind24", "2h": "remind2"}[win]
			a.tell(ctx, b.UserID, "reminder", T(b.Lang, key, svc.Name(b.Lang), m.Name(b.Lang), t), nil, fmt.Sprintf("remind-%s-%d", win, b.ID))
			a.store.MarkReminded(b.ID, win)
			sent++
		}
	}
	return sent
}

func (a *App) runLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	a.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.tick(ctx)
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
