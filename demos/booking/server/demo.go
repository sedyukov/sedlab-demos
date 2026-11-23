package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pay "github.com/sedyukov/sedlab-modules/modules/payments/go"
	tg "github.com/sedyukov/sedlab-modules/modules/telegram-core/go"
)

// Browser emulator endpoints. Enabled with DEMO_EMULATOR=true. Everything here goes through the same
// production code paths (initData validation, webhooks with signatures) — only the "Telegram side" is simulated.

// POST /demo/initdata {persona: guest|owner, guest_id, name, lang} → {init_data} signed by telegram-core.
func (a *App) handleDemoInitData(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Persona  string `json:"persona"`
		Platform string `json:"platform"` // telegram | max
		GuestID  int64  `json:"guest_id"`
		Name     string `json:"name"`
		Lang     string `json:"lang"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u := WebUser{Lang: normLang(in.Lang)}
	if in.Platform == "max" && !a.maxEnabled() {
		writeErr(w, 400, "invalid_argument", "Max is available in the Russian edition")
		return
	}
	m := a.messenger(in.Platform)
	if in.Persona == "owner" {
		m = a.msg // the owner persona lives in Telegram
		u.ID, u.FirstName, u.Username = DemoOwnerID, "Salon owner", "salon_owner"
		if u.Lang == "ru" {
			u.FirstName = "Владелец салона"
		}
	} else {
		if in.GuestID <= 0 {
			in.GuestID = time.Now().UnixNano()
		}
		u.ID = EmulatorUserBase + 1_000_000 + in.GuestID%1_000_000_000_000
		if m.Platform() == "max" {
			u.ID = EmulatorUserBase + MaxEmulatorOffset + in.GuestID%1_000_000_000_000
		}
		u.FirstName = truncate(strings.TrimSpace(in.Name), 32)
		if u.FirstName == "" {
			u.FirstName = map[string]string{"ru": "Гость", "en": "Guest"}[u.Lang]
		}
		u.Username = "demo_guest"
	}
	initData, err := m.EmulatorInitData(u, a.now().Unix())
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"init_data": initData, "user_id": u.ID, "platform": m.Platform()})
}

// GET /demo/chat?user=&after= → bot messages for an emulator user (the chat pane next to the Mini App).
func (a *App) handleDemoChat(w http.ResponseWriter, r *http.Request) {
	uid := qInt(r, "user")
	if !isEmulatorUser(uid) {
		writeErr(w, 400, "invalid_argument", "emulator users only")
		return
	}
	ms, err := a.store.Messages(uid, qInt(r, "after"))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"messages": ms})
}

// POST /demo/send {user, first_name, lang, text} — the emulator user types to the bot; we build a real
// Telegram update and run it through the webhook path (secret header + dedupe in telegram-core).
func (a *App) handleDemoSend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		User      int64  `json:"user"`
		FirstName string `json:"first_name"`
		Lang      string `json:"lang"`
		Text      string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if !isEmulatorUser(in.User) || in.Text == "" || len(in.Text) > 500 {
		writeErr(w, 400, "invalid_argument", "bad message")
		return
	}
	platform := a.platformOf(in.User)
	a.store.UpsertUser(in.User, truncate(strings.TrimSpace(in.FirstName), 32), "demo_guest", a.userLangOr(in.User, normLang(in.Lang)), platform)
	a.store.AddMessage(&Message{ChatID: in.User, Text: in.Text, Kind: "user"})
	a.mu.Lock()
	a.updSeq++
	upd := a.updSeq
	a.mu.Unlock()
	body := mustJSON(map[string]any{
		"update_id": 900_000_000 + upd,
		"message": map[string]any{
			"message_id": upd, "date": a.now().Unix(), "text": in.Text,
			"from": map[string]any{"id": in.User, "is_bot": false, "first_name": in.FirstName},
			"chat": map[string]any{"id": in.User, "type": "private"},
		},
	})
	var err error
	if platform == "max" { // Max webhook shape + secret header, handled by max-core
		mb := maxEmulatorUpdate(in.User, fmt.Sprintf("mid.emu.%d", upd), in.Text, a.now().Unix())
		err = a.processUpdateOn(r.Context(), a.max, map[string]string{"X-Max-Bot-Api-Secret": a.cfg.MaxSecret}, mb)
	} else {
		hdr := map[string]string{"X-Telegram-Bot-Api-Secret-Token": a.cfg.WebhookSecret}
		err = a.processUpdate(r.Context(), hdr, []byte(body))
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /demo/callback {user, data} — the emulator user presses an inline (callback) button.
func (a *App) handleDemoCallback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		User int64  `json:"user"`
		Data string `json:"data"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !isEmulatorUser(in.User) || in.Data == "" || len(in.Data) > 64 {
		writeErr(w, 400, "invalid_argument", "bad callback")
		return
	}
	a.mu.Lock()
	a.updSeq++
	upd := a.updSeq
	a.mu.Unlock()
	var err error
	if a.platformOf(in.User) == "max" {
		body := maxEmulatorCallback(in.User, fmt.Sprintf("cb.emu.%d", upd), in.Data, a.now().Unix())
		err = a.processUpdateOn(r.Context(), a.max, map[string]string{"X-Max-Bot-Api-Secret": a.cfg.MaxSecret}, body)
	} else {
		body := mustJSON(map[string]any{
			"update_id": 900_000_000 + upd,
			"callback_query": map[string]any{"id": fmt.Sprintf("cb%d", upd), "data": in.Data,
				"from": map[string]any{"id": in.User}, "message": map[string]any{"chat": map[string]any{"id": in.User}}},
		})
		err = a.processUpdate(r.Context(), map[string]string{"X-Telegram-Bot-Api-Secret-Token": a.cfg.WebhookSecret}, []byte(body))
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /demo/stars/pay {booking_id} — the emulator's Stars sheet "Pay" button: telegram-core MockStarsPayment
// produces pre_checkout_query + successful_payment updates which go through the regular webhook handler.
func (a *App) handleDemoStarsPay(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in struct {
		BookingID int64 `json:"booking_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	b, err := a.store.Booking(in.BookingID)
	if err != nil || b.UserID != s.UserID || !s.Emulated {
		writeErr(w, 404, "not_found", "booking not found")
		return
	}
	mp, err := a.tgm.EmulatorStarsPayment(s.UserID, starsPayload(b.ID), b.PayAmount)
	if err != nil {
		a.fail(w, err)
		return
	}
	// pre-checkout: same validation the webhook does; a declined query stops the payment
	if ok, msg := a.checkStarsPayable(s.UserID, &tg.Payment{Currency: "XTR", TotalAmount: b.PayAmount, Payload: starsPayload(b.ID)}); !ok {
		writeErr(w, 409, "conflict", msg)
		return
	}
	for _, req := range []tg.WebhookRequest{mp.PreCheckout, mp.SuccessfulPayment} {
		if err := a.processUpdate(r.Context(), req.Headers, []byte(req.Body)); err != nil {
			a.fail(w, err)
			return
		}
	}
	nb, _ := a.store.Booking(b.ID)
	writeJSON(w, 200, a.bookingView(nb))
}

// POST /demo/reminders/run — "fast-forward": send the 24h reminder for the user's next booking now.
func (a *App) handleDemoRunReminders(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	if !s.Emulated {
		writeErr(w, 403, "permission_denied", "demo only")
		return
	}
	bs, _ := a.store.queryBookings(`user_id=? AND status='confirmed' AND start_at>? ORDER BY start_at LIMIT 1`, s.UserID, a.now().Unix())
	if len(bs) == 0 {
		writeJSON(w, 200, map[string]any{"sent": 0})
		return
	}
	b := bs[0]
	svc, _ := a.store.Service(b.ServiceID)
	m, _ := a.store.Master(b.MasterID)
	t := time.Unix(b.StartAt, 0).In(a.cfg.TZ)
	text := T(b.Lang, "remind24", svc.Name(b.Lang), m.Name(b.Lang), t.Format("15:04"))
	if t.Sub(a.now()) > 36*time.Hour || t.Sub(a.now()) < 12*time.Hour {
		text = T(b.Lang, "remind2", svc.Name(b.Lang), m.Name(b.Lang), t.Format("15:04"))
		if b.Lang == "ru" {
			text = "⏰ Напоминание: " + fmtWhen("ru", t) + ", <b>" + svc.Name("ru") + "</b>, мастер " + m.Name("ru") + "."
		} else {
			text = "⏰ Reminder: " + fmtWhen("en", t) + ", <b>" + svc.Name("en") + "</b> with " + m.Name("en") + "."
		}
	}
	a.tell(r.Context(), b.UserID, "reminder", text, &WebAppButton{Text: T(b.Lang, "my_open"), URL: a.cfg.PublicURL + "/app/?screen=my"}, "")
	writeJSON(w, 200, map[string]any{"sent": 1})
}

// GET /demo/checkout/{pid} → what the fake YooKassa page shows
func (a *App) handleDemoCheckoutInfo(w http.ResponseWriter, r *http.Request) {
	p, err := a.pay.GetPayment(r.Context(), r.PathValue("pid"))
	if err != nil {
		writeErr(w, 404, "not_found", "payment not found")
		return
	}
	writeJSON(w, 200, map[string]any{"id": p.ID, "status": p.Status, "amount": p.Amount, "description": p.Description,
		"booking_id": p.Metadata["booking_id"], "return_url": fmt.Sprintf("%s/app/?booking=%s", a.cfg.PublicURL, p.Metadata["booking_id"])})
}

// POST /demo/checkout/{pid} {outcome: succeeded|canceled} — the mock provider changes state and returns a signed
// webhook; we deliver it to our own /payments/webhook logic exactly like YooKassa would.
func (a *App) handleDemoCheckout(w http.ResponseWriter, r *http.Request) {
	mock, ok := a.pay.(*pay.Mock)
	if !ok {
		writeErr(w, 404, "not_found", "mock payments disabled")
		return
	}
	var in struct {
		Outcome string `json:"outcome"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Outcome != "succeeded" && in.Outcome != "canceled" {
		writeErr(w, 400, "invalid_argument", "outcome: succeeded|canceled")
		return
	}
	wh, err := mock.MockComplete(r.Context(), r.PathValue("pid"), in.Outcome)
	if err != nil {
		writeErr(w, 409, "conflict", "payment can't change state")
		return
	}
	if _, err := a.processPaymentWebhook(r.Context(), wh.Headers, []byte(wh.Body)); err != nil {
		a.fail(w, err)
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("booking"), 10, 64)
	writeJSON(w, 200, map[string]any{"ok": true, "booking_id": id})
}
