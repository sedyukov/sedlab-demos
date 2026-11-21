package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mx "github.com/sedyukov/sedlab-modules/modules/max-core/go"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	pay "github.com/sedyukov/sedlab-modules/modules/payments/go"
	tg "github.com/sedyukov/sedlab-modules/modules/telegram-core/go"
)

type harness struct {
	t      *testing.T
	app    *App
	srv    *httptest.Server
	now    time.Time
	notify *notify.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	tz, _ := time.LoadLocation("Europe/Moscow")
	h := &harness{t: t, now: time.Date(2026, 11, 23, 9, 0, 0, 0, tz)} // Monday 09:00
	st, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	secret := "test-webhook-secret"
	bot := tg.NewBot(tg.NewMockAPI(), tg.Config{Token: tg.MockToken, WebhookSecret: secret})
	h.srv = httptest.NewUnstartedServer(nil)
	h.srv.Start()
	t.Cleanup(h.srv.Close)
	p, _ := pay.OpenMock(pay.MockConfig{CheckoutURL: h.srv.URL + "/pay/{id}", Now: func() time.Time { return h.now }})
	cfg := Config{PublicURL: h.srv.URL, TZ: tz, PayMethods: []string{"yookassa", "stars"}, DepositPercent: 30, HoldMinutes: 15,
		LeadMinutes: 60, Emulator: true, WebhookSecret: secret, PaymentsMock: true}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// seed with the harness clock
	a := &App{cfg: cfg, store: st, pay: p, log: log, now: func() time.Time { return h.now }}
	tgm := newTelegramMessenger(bot, false, true, secret)
	a.msg, a.tgm, a.notify = tgm, tgm, notify.NewMockClient()
	a.cfg.MaxSecret = "test-max-secret"
	a.max = newMaxMessenger(mx.NewBot(mx.NewMockAPI(), mx.Config{Token: mx.MockToken, WebhookSecret: a.cfg.MaxSecret}), false, true, a.cfg.MaxSecret, "bloom_salon_bot")
	h.notify = a.notify.(*notify.Client)
	a.limiter = newIPLimiter(10000, a.now)
	if err := a.ResetDemo(); err != nil {
		t.Fatal(err)
	}
	h.app = a
	h.srv.Config.Handler = a.Routes()
	return h
}

func (h *harness) do(method, path, token string, body any) (int, map[string]any) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func (h *harness) login(persona string, guest int64, lang string) (string, int64) {
	h.t.Helper()
	code, r := h.do("POST", "/demo/initdata", "", map[string]any{"persona": persona, "guest_id": guest, "name": "Test", "lang": lang})
	if code != 200 {
		h.t.Fatalf("initdata %d %v", code, r)
	}
	code, a := h.do("POST", "/api/auth", "", map[string]any{"init_data": r["init_data"]})
	if code != 200 {
		h.t.Fatalf("auth %d %v", code, a)
	}
	return a["token"].(string), int64(r["user_id"].(float64))
}

// firstSlot picks a free slot for service 2 (men's haircut) with master 4 (Dmitry, works Mon).
func (h *harness) firstSlot(tok string, date string) map[string]any {
	h.t.Helper()
	code, r := h.do("GET", "/api/slots?service=2&master=4&date="+date, tok, nil)
	if code != 200 {
		h.t.Fatalf("slots %d %v", code, r)
	}
	sl := r["slots"].([]any)
	if len(sl) == 0 {
		h.t.Fatal("no slots")
	}
	return sl[len(sl)-1].(map[string]any)
}

func (h *harness) book(tok string, date string) map[string]any {
	h.t.Helper()
	s := h.firstSlot(tok, date)
	code, b := h.do("POST", "/api/bookings", tok, map[string]any{"service_id": 2, "master_id": 4, "start": s["start"], "name": "Ann", "phone": "+381000"})
	if code != 201 {
		h.t.Fatalf("book %d %v", code, b)
	}
	return b
}

func (h *harness) chat(uid int64) []map[string]any {
	_, r := h.do("GET", fmt.Sprintf("/demo/chat?user=%d", uid), "", nil)
	out := []map[string]any{}
	for _, m := range r["messages"].([]any) {
		out = append(out, m.(map[string]any))
	}
	return out
}

func TestYooKassaFlow(t *testing.T) {
	h := newHarness(t)
	tok, uid := h.login("guest", 42, "ru")
	b := h.book(tok, "2026-11-24")
	id := int64(b["id"].(float64))

	code, p := h.do("POST", fmt.Sprintf("/api/bookings/%d/pay", id), tok, map[string]any{"method": "yookassa"})
	if code != 200 || !strings.Contains(p["confirmation_url"].(string), "/pay/pay_mock_") {
		t.Fatalf("pay %d %v", code, p)
	}
	if p["amount"].(float64) != 45000 { // 30% of 1500 ₽
		t.Fatalf("deposit %v", p["amount"])
	}
	pid := p["confirmation_url"].(string)[strings.LastIndex(p["confirmation_url"].(string), "/")+1:]
	code, info := h.do("GET", "/demo/checkout/"+pid, "", nil)
	if code != 200 || info["booking_id"] != fmt.Sprint(id) {
		t.Fatalf("checkout info %d %v", code, info)
	}
	code, _ = h.do("POST", "/demo/checkout/"+pid, "", map[string]any{"outcome": "succeeded"})
	if code != 200 {
		t.Fatalf("checkout %d", code)
	}
	_, got := h.do("GET", fmt.Sprintf("/api/bookings/%d", id), tok, nil)
	if got["status"] != "confirmed" || got["pay_currency"] != "RUB" {
		t.Fatalf("not confirmed: %v", got)
	}
	msgs := h.chat(uid)
	if len(msgs) != 1 || !strings.Contains(msgs[0]["text"].(string), "Вы записаны") {
		t.Fatalf("chat: %v", msgs)
	}
	if owner := h.chat(DemoOwnerID); len(owner) != 1 {
		t.Fatalf("owner chat: %v", owner)
	}
	// replaying the provider webhook is a no-op (payments dedupe + idempotent confirm)
	code, _ = h.do("POST", "/demo/checkout/"+pid, "", map[string]any{"outcome": "succeeded"})
	if code != 409 || len(h.chat(uid)) != 1 {
		t.Fatalf("replay: %d", code)
	}
	// slot is now taken
	code, _ = h.do("POST", "/api/bookings", tok, map[string]any{"service_id": 2, "master_id": 4, "start": b["start_at"], "name": "Bob"})
	if code != 409 {
		t.Fatalf("double booking allowed: %d", code)
	}
	// client cancels ≥24h before → prepayment refunded
	h.now = h.now.Add(-time.Hour)
	code, c := h.do("POST", fmt.Sprintf("/api/bookings/%d/cancel", id), tok, nil)
	if code != 200 || c["status"] != "cancelled" || c["refunded"] != true {
		t.Fatalf("cancel %d %v", code, c)
	}
}

func TestStarsFlow(t *testing.T) {
	h := newHarness(t)
	tok, uid := h.login("guest", 7, "en")
	b := h.book(tok, "2026-11-24")
	id := int64(b["id"].(float64))
	code, p := h.do("POST", fmt.Sprintf("/api/bookings/%d/pay", id), tok, map[string]any{"method": "stars"})
	if code != 200 || p["currency"] != "XTR" || p["amount"].(float64) != 270 || !strings.HasPrefix(p["invoice_link"].(string), "https://t.me/") {
		t.Fatalf("stars pay %d %v", code, p)
	}
	code, got := h.do("POST", "/demo/stars/pay", tok, map[string]any{"booking_id": id})
	if code != 200 || got["status"] != "confirmed" || got["pay_amount"].(float64) != 270 {
		t.Fatalf("stars confirm %d %v", code, got)
	}
	msgs := h.chat(uid)
	if len(msgs) != 1 || !strings.Contains(msgs[0]["text"].(string), "270 ⭐") {
		t.Fatalf("chat: %v", msgs)
	}
	// second simulated payment of the same booking is declined at pre-checkout
	code, _ = h.do("POST", "/demo/stars/pay", tok, map[string]any{"booking_id": id})
	if code != 409 {
		t.Fatalf("double stars payment: %d", code)
	}
}

func TestHoldExpiresAndFreesSlot(t *testing.T) {
	h := newHarness(t)
	tok, _ := h.login("guest", 1, "en")
	b := h.book(tok, "2026-11-24")
	tok2, _ := h.login("guest", 2, "en")
	code, _ := h.do("POST", "/api/bookings", tok2, map[string]any{"service_id": 2, "master_id": 4, "start": b["start_at"], "name": "B"})
	if code != 409 {
		t.Fatalf("held slot bookable: %d", code)
	}
	h.now = h.now.Add(16 * time.Minute)
	h.app.tick(t.Context())
	code, _ = h.do("POST", "/api/bookings", tok2, map[string]any{"service_id": 2, "master_id": 4, "start": b["start_at"], "name": "B"})
	if code != 201 {
		t.Fatalf("expired hold still blocks: %d", code)
	}
	// paying the expired booking is refused
	code, _ = h.do("POST", fmt.Sprintf("/api/bookings/%v/pay", b["id"]), tok, map[string]any{"method": "yookassa"})
	if code != 409 {
		t.Fatalf("pay expired: %d", code)
	}
}

func TestRemindersOnce(t *testing.T) {
	h := newHarness(t)
	tok, uid := h.login("guest", 3, "en")
	b := h.book(tok, "2026-11-24") // tomorrow evening
	id := int64(b["id"].(float64))
	h.do("POST", fmt.Sprintf("/api/bookings/%d/pay", id), tok, map[string]any{"method": "stars"})
	h.do("POST", "/demo/stars/pay", tok, map[string]any{"booking_id": id})
	// pretend it was booked 3 days ago so the 24h reminder applies
	h.app.store.db.Exec(`UPDATE bookings SET created_at=created_at-3*86400 WHERE id=?`, id)
	start := time.Unix(int64(b["start_at"].(float64)), 0)
	h.now = start.Add(-23 * time.Hour)
	if n := h.app.sendReminders(t.Context()); n != 1 {
		t.Fatalf("24h reminders sent: %d", n)
	}
	if n := h.app.sendReminders(t.Context()); n != 0 {
		t.Fatalf("reminder repeated: %d", n)
	}
	h.now = start.Add(-90 * time.Minute)
	if n := h.app.sendReminders(t.Context()); n != 1 {
		t.Fatalf("2h reminders sent: %d", n)
	}
	msgs := h.chat(uid)
	if d, _ := h.notify.ListDeliveries("telegram"); len(d) != 4 { // confirmation + owner alert + 2 reminders
		t.Fatalf("notify deliveries: %d", len(d))
	}
	if len(msgs) != 3 || !strings.Contains(msgs[1]["text"].(string), "tomorrow") || !strings.Contains(msgs[2]["text"].(string), "2 hours") {
		t.Fatalf("chat: %v", msgs)
	}
}

func TestAuthAndRoles(t *testing.T) {
	h := newHarness(t)
	code, _ := h.do("POST", "/api/auth", "", map[string]any{"init_data": "user=%7B%22id%22%3A1%7D&auth_date=1&hash=00"})
	if code != 401 {
		t.Fatalf("forged initData: %d", code)
	}
	// initData signed with another token is rejected
	other := tg.NewBot(tg.NewMockAPI(), tg.Config{Token: "123:other"})
	r, _ := other.BuildInitData(tg.BuildInitDataInput{User: map[string]any{"id": EmulatorUserBase + 5}, AuthDate: h.now.Unix()})
	if code, _ := h.do("POST", "/api/auth", "", map[string]any{"init_data": r.InitData}); code != 401 {
		t.Fatalf("foreign signature: %d", code)
	}
	if code, _ := h.do("GET", "/api/slots?service=1&date=2026-11-24", "garbage", nil); code != 401 {
		t.Fatalf("bad token: %d", code)
	}
	client, _ := h.login("guest", 9, "en")
	if code, _ := h.do("GET", "/api/admin/overview", client, nil); code != 403 {
		t.Fatalf("client reached admin: %d", code)
	}
	owner, _ := h.login("owner", 0, "en")
	code, ov := h.do("GET", "/api/admin/overview?date=2026-11-24", owner, nil)
	if code != 200 || len(ov["masters"].([]any)) != 4 || len(ov["bookings"].([]any)) == 0 {
		t.Fatalf("overview %d %v", code, ov["kpi"])
	}
	// owner edits a service
	code, s := h.do("POST", "/api/admin/services", owner, map[string]any{"id": 2, "name_ru": "Мужская стрижка", "name_en": "Men's cut", "duration_min": 30, "price_rub": 120000, "price_stars": 700, "active": true})
	if code != 200 || s["duration_min"].(float64) != 30 {
		t.Fatalf("save service %d %v", code, s)
	}
	if code, _ := h.do("POST", "/api/admin/services", owner, map[string]any{"name_ru": "x", "duration_min": 7, "price_rub": 1, "price_stars": 1}); code != 400 {
		t.Fatalf("invalid service accepted: %d", code)
	}
}

func TestTelegramWebhook(t *testing.T) {
	h := newHarness(t)
	body := `{"update_id":5,"message":{"message_id":1,"date":1,"text":"/start","from":{"id":4000000000001234,"first_name":"Z"},"chat":{"id":4000000000001234,"type":"private"}}}`
	req, _ := http.NewRequest("POST", h.srv.URL+"/tg/webhook", strings.NewReader(body))
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 400 {
		t.Fatalf("no secret accepted: %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", h.srv.URL+"/tg/webhook", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "test-webhook-secret")
	for i := 0; i < 2; i++ { // duplicate update_id → handled once
		r2 := req.Clone(req.Context())
		r2.Body = io.NopCloser(strings.NewReader(body))
		res, _ = http.DefaultClient.Do(r2)
		if res.StatusCode != 200 {
			t.Fatalf("webhook: %d", res.StatusCode)
		}
	}
	msgs := h.chat(4000000000001234)
	if len(msgs) != 1 || msgs[0]["button"] == "" {
		t.Fatalf("start reply: %v", msgs)
	}
}

func TestMaxFlow(t *testing.T) {
	h := newHarness(t)
	code, r := h.do("POST", "/demo/initdata", "", map[string]any{"platform": "max", "guest_id": 77, "name": "Олег", "lang": "ru"})
	if code != 200 || r["platform"] != "max" {
		t.Fatalf("max initdata %d %v", code, r)
	}
	uid := int64(r["user_id"].(float64))
	// Telegram must not accept Max launch data (different sandbox tokens)
	if code, _ := h.do("POST", "/api/auth", "", map[string]any{"init_data": r["init_data"]}); code != 401 {
		t.Fatalf("telegram accepted max initData: %d", code)
	}
	code, a := h.do("POST", "/api/auth", "", map[string]any{"init_data": r["init_data"], "platform": "max"})
	if code != 200 || a["platform"] != "max" {
		t.Fatalf("max auth %d %v", code, a)
	}
	tok := a["token"].(string)
	// /start typed in the emulator goes through max-core's webhook parsing
	if code, _ := h.do("POST", "/demo/send", "", map[string]any{"user": uid, "first_name": "Олег", "lang": "ru", "text": "/start"}); code != 200 {
		t.Fatalf("max send %d", code)
	}
	if msgs := h.chat(uid); len(msgs) != 2 || !strings.Contains(msgs[1]["text"].(string), "Олег") {
		t.Fatalf("max welcome: %v", msgs)
	}
	b := h.book(tok, "2026-11-24")
	id := int64(b["id"].(float64))
	if code, _ := h.do("POST", fmt.Sprintf("/api/bookings/%d/pay", id), tok, map[string]any{"method": "stars"}); code != 400 {
		t.Fatalf("stars allowed on max: %d", code)
	}
	_, p := h.do("POST", fmt.Sprintf("/api/bookings/%d/pay", id), tok, map[string]any{"method": "yookassa"})
	pid := p["confirmation_url"].(string)[strings.LastIndex(p["confirmation_url"].(string), "/")+1:]
	h.do("POST", "/demo/checkout/"+pid, "", map[string]any{"outcome": "succeeded"})
	_, got := h.do("GET", fmt.Sprintf("/api/bookings/%d", id), tok, nil)
	if got["status"] != "confirmed" {
		t.Fatalf("max booking: %v", got)
	}
	// confirmation went to Max via max-core (open_app button), not to the Telegram notify channel
	calls := h.app.max.emu.API.(*mx.MockAPI).Calls("POST /messages")
	if len(calls) == 0 {
		t.Fatal("no max-core sendMessage")
	}
	if d, _ := h.notify.ListDeliveries("telegram"); len(d) != 1 { // owner alert only (owner persona is in Telegram)
		t.Fatalf("telegram deliveries: %d", len(d))
	}
}
