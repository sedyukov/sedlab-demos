package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	pay "github.com/sedyukov/sedlab-modules/modules/payments/go"
)

type Config struct {
	PublicURL      string // https://booking.demo.sedlab.dev
	TZ             *time.Location
	PayMethods     []string // yookassa, stars
	DepositPercent int64    // prepayment share of the service price
	HoldMinutes    int
	LeadMinutes    int // earliest bookable start from now
	AdminIDs       []int64
	Emulator       bool   // browser emulator: /demo/* endpoints + emulator-signed initData
	DemoReset      bool   // daily reset of demo data
	WebhookSecret  string // X-Telegram-Bot-Api-Secret-Token
	MaxSecret      string // X-Max-Bot-Api-Secret
	MaxLive        bool
	PaymentsMock   bool
	TelegramLive   bool
}

type App struct {
	cfg    Config
	store  *Store
	msg    Messenger          // Telegram (telegram-core)
	tgm    *telegramMessenger // same object as msg (emulator helpers)
	max    *maxMessenger      // Max (max-core)
	pay    pay.Provider
	notify Notifier
	log    *slog.Logger
	now    func() time.Time

	updSeq  int64 // simulated update ids (emulator)
	mu      sync.Mutex
	limiter *ipLimiter
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) })

	// Mini App API (bearer session after initData)
	mux.HandleFunc("POST /api/auth", a.handleAuth)
	mux.HandleFunc("GET /api/catalog", a.handleCatalog)
	mux.HandleFunc("GET /api/days", a.authed(a.handleDays))
	mux.HandleFunc("GET /api/slots", a.authed(a.handleSlots))
	mux.HandleFunc("POST /api/bookings", a.authed(a.handleCreateBooking))
	mux.HandleFunc("GET /api/bookings/my", a.authed(a.handleMyBookings))
	mux.HandleFunc("GET /api/bookings/{id}", a.authed(a.handleGetBooking))
	mux.HandleFunc("POST /api/bookings/{id}/pay", a.authed(a.handlePay))
	mux.HandleFunc("POST /api/bookings/{id}/cancel", a.authed(a.handleClientCancel))
	mux.HandleFunc("POST /api/lang", a.authed(a.handleLang))

	// Owner admin
	mux.HandleFunc("GET /api/admin/overview", a.owner(a.handleAdminOverview))
	mux.HandleFunc("GET /api/admin/bookings", a.owner(a.handleAdminBookings))
	mux.HandleFunc("POST /api/admin/bookings/{id}/cancel", a.owner(a.handleAdminCancel))
	mux.HandleFunc("POST /api/admin/bookings/{id}/complete", a.owner(a.handleAdminComplete))
	mux.HandleFunc("GET /api/admin/services", a.owner(a.handleAdminServices))
	mux.HandleFunc("POST /api/admin/services", a.owner(a.handleAdminSaveService))
	mux.HandleFunc("GET /api/admin/masters", a.owner(a.handleAdminMasters))
	mux.HandleFunc("POST /api/admin/masters", a.owner(a.handleAdminSaveMaster))
	mux.HandleFunc("POST /api/admin/reset", a.owner(a.handleAdminReset))

	// Provider webhooks
	mux.HandleFunc("POST /tg/webhook", a.handleTelegramWebhook)
	mux.HandleFunc("POST /max/webhook", a.handleMaxWebhook)
	mux.HandleFunc("POST /payments/webhook", a.handlePaymentsWebhook)

	// Browser emulator (sandbox only)
	if a.cfg.Emulator {
		mux.HandleFunc("POST /demo/initdata", a.handleDemoInitData)
		mux.HandleFunc("GET /demo/chat", a.handleDemoChat)
		mux.HandleFunc("POST /demo/send", a.handleDemoSend)
		mux.HandleFunc("POST /demo/stars/pay", a.authed(a.handleDemoStarsPay))
		mux.HandleFunc("POST /demo/reminders/run", a.authed(a.handleDemoRunReminders))
	}
	if a.cfg.PaymentsMock {
		mux.HandleFunc("GET /demo/checkout/{pid}", a.handleDemoCheckoutInfo)
		mux.HandleFunc("POST /demo/checkout/{pid}", a.handleDemoCheckout)
	}
	mux.HandleFunc("GET /api/config", a.handleConfig)

	mux.Handle("/", staticHandler())
	return a.middleware(mux)
}

func (a *App) middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.Method != http.MethodGet && !strings.HasPrefix(r.URL.Path, "/tg/") && !strings.HasPrefix(r.URL.Path, "/max/") && !strings.HasPrefix(r.URL.Path, "/payments/") {
			if !a.limiter.Allow(clientIP(r)) {
				writeErr(w, 429, "rate_limited", "too many requests")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// ---- auth ----

type ctxKey struct{}

type Session struct {
	UserID   int64
	Role     string // client | owner
	Emulated bool
}

func sessionFrom(r *http.Request) Session { s, _ := r.Context().Value(ctxKey{}).(Session); return s }

func (a *App) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := a.msg.VerifySession(r.Header.Get("Authorization"))
		if err != nil {
			uid, err = a.max.VerifySession(r.Header.Get("Authorization"))
		}
		if err != nil {
			writeErr(w, 401, "unauthenticated", "session expired, reopen the app")
			return
		}
		emu := isEmulatorUser(uid)
		s := Session{UserID: uid, Emulated: emu, Role: a.roleFor(uid, emu)}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, s)))
	}
}

func (a *App) owner(h http.HandlerFunc) http.HandlerFunc {
	return a.authed(func(w http.ResponseWriter, r *http.Request) {
		if sessionFrom(r).Role != "owner" {
			writeErr(w, 403, "permission_denied", "owner only")
			return
		}
		h(w, r)
	})
}

// DemoOwnerID is the emulator's "salon owner" persona.
const DemoOwnerID = EmulatorUserBase + 1

func (a *App) roleFor(uid int64, emulated bool) string {
	for _, id := range a.cfg.AdminIDs {
		if id == uid && !emulated {
			return "owner"
		}
	}
	if emulated && uid == DemoOwnerID {
		return "owner"
	}
	return "client"
}

func (a *App) handleAuth(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InitData string `json:"init_data"`
		Platform string `json:"platform"` // telegram (default) | max
	}
	if !readJSON(w, r, &in) {
		return
	}
	m := a.messenger(in.Platform)
	u, emulated, err := m.AuthWebApp(in.InitData)
	if err != nil {
		writeErr(w, 401, "unauthenticated", "invalid initData")
		return
	}
	name := u.FirstName
	if a.cfg.DemoReset && !emulated { // public demo host (outside RU): real users' names stay in the session only
		name = ""
	}
	if err := a.store.UpsertUser(u.ID, name, "" /* username not stored */, a.userLangOr(u.ID, u.Lang), m.Platform()); err != nil {
		a.fail(w, err)
		return
	}
	if emulated != isEmulatorUser(u.ID) { // emulator may only sign emulator ids, and vice versa
		writeErr(w, 401, "unauthenticated", "invalid user id")
		return
	}
	tok, exp, err := m.IssueSession(u, 12*time.Hour)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"token": tok, "expires_at": exp, "role": a.roleFor(u.ID, emulated), "emulated": emulated, "platform": m.Platform(),
		"user": map[string]any{"id": u.ID, "first_name": u.FirstName, "username": u.Username, "lang": a.store.UserLang(u.ID)},
	})
}

func (a *App) messenger(platform string) Messenger {
	if platform == "max" {
		return a.max
	}
	return a.msg
}

// platformOf: emulator users by id range, real users by the platform they signed in from.
func (a *App) platformOf(uid int64) string {
	if isEmulatorUser(uid) {
		if uid >= EmulatorUserBase+MaxEmulatorOffset {
			return "max"
		}
		return "telegram"
	}
	var p string
	if a.store.db.QueryRow(`SELECT platform FROM users WHERE id=?`, uid).Scan(&p) == nil && p != "" {
		return p
	}
	return "telegram"
}

// keep a language the user picked explicitly in the app; otherwise take Telegram's
func (a *App) userLangOr(id int64, def string) string {
	var l string
	if a.store.db.QueryRow(`SELECT lang FROM users WHERE id=?`, id).Scan(&l) == nil && l != "" {
		return l
	}
	return def
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"pay_methods": a.cfg.PayMethods, "deposit_percent": a.cfg.DepositPercent, "tz": a.cfg.TZ.String(),
		"emulator": a.cfg.Emulator, "payments_mock": a.cfg.PaymentsMock, "telegram_live": a.cfg.TelegramLive,
		"platform": a.msg.Platform(), "hold_minutes": a.cfg.HoldMinutes,
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, kind, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]string{"code": kind, "message": msg}})
}

func (a *App) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	a.log.Error("request failed", "err", err)
	writeErr(w, 500, "internal", "internal error")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err == nil && len(body) > 0 {
		err = json.Unmarshal(body, v)
	}
	if err != nil {
		writeErr(w, 400, "invalid_argument", "bad JSON")
		return false
	}
	return true
}

func pathID(r *http.Request) int64 { id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64); return id }

func headersMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k := range h {
		m[k] = h.Get(k)
	}
	return m
}

func randHex(n int) string { b := make([]byte, n); rand.Read(b); return hex.EncodeToString(b) }

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		ip, _, _ := strings.Cut(x, ",")
		return strings.TrimSpace(ip)
	}
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	return ip
}

// ipLimiter: simple fixed-window limiter for mutating requests (demo is public).
type ipLimiter struct {
	mu     sync.Mutex
	window int64
	counts map[string]int
	max    int
	now    func() time.Time
}

func newIPLimiter(max int, now func() time.Time) *ipLimiter {
	return &ipLimiter{counts: map[string]int{}, max: max, now: now}
}

func (l *ipLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.now().Unix() / 60
	if w != l.window {
		l.window, l.counts = w, map[string]int{}
	}
	l.counts[ip]++
	return l.counts[ip] <= l.max
}
