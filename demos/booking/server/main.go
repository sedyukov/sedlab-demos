// booking — salon booking bot + Telegram Mini App (sedlab showcase S3).
// Thin glue over private sedlab modules: telegram-core (Mini App auth, Stars, webhooks), payments (YooKassa),
// notify (reminders; glue stub until the module ships). Max (VK) — Messenger seam, waiting for max-core.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	acc "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	acckit "github.com/sedyukov/sedlab-modules/modules/accounts/go/kit"
	mx "github.com/sedyukov/sedlab-modules/modules/max-core/go"
	mxkit "github.com/sedyukov/sedlab-modules/modules/max-core/go/kit"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	notifykit "github.com/sedyukov/sedlab-modules/modules/notify/go/kit"
	pay "github.com/sedyukov/sedlab-modules/modules/payments/go"
	paykit "github.com/sedyukov/sedlab-modules/modules/payments/go/kit"
	tg "github.com/sedyukov/sedlab-modules/modules/telegram-core/go"
	tgkit "github.com/sedyukov/sedlab-modules/modules/telegram-core/go/kit"
)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(env(k, "")); err == nil {
		return v
	}
	return def
}

func main() {
	if len(os.Args) > 1 {
		os.Exit(subcommand(os.Args[1:]))
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app, err := NewAppFromEnv(log)
	if err != nil {
		log.Error("startup", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if app.cfg.MaxLive && strings.HasPrefix(app.cfg.PublicURL, "https://") {
		if err := app.max.SetWebhook(ctx, app.cfg.PublicURL+"/max/webhook"); err != nil {
			log.Error("max subscription failed", "err", err)
		}
	}
	if app.cfg.TelegramLive && strings.HasPrefix(app.cfg.PublicURL, "https://") && env("TELEGRAM_SET_WEBHOOK", "true") == "true" {
		if err := app.msg.SetWebhook(ctx, app.cfg.PublicURL+"/tg/webhook"); err != nil {
			log.Error("setWebhook failed", "err", err)
		} else {
			log.Info("webhook set")
		}
	}
	go app.runLoop(ctx)

	srv := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: app.Routes(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Info("listening", "addr", srv.Addr, "public_url", app.cfg.PublicURL, "telegram_live", app.cfg.TelegramLive, "payments", app.pay.Name())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server", "err", err)
		os.Exit(1)
	}
	app.store.Close()
}

func NewAppFromEnv(log *slog.Logger) (*App, error) {
	defTZ := "Europe/London"
	if env("EDITION", "intl") == "ru" {
		defTZ = "Europe/Moscow"
	}
	tz, err := time.LoadLocation(env("SALON_TZ", defTZ))
	if err != nil {
		return nil, err
	}
	public := strings.TrimRight(env("PUBLIC_URL", "http://localhost:8080"), "/")
	dataDir := env("DATA_DIR", "./data")
	os.MkdirAll(dataDir, 0o700)

	// webhook secret: required for live; generated per process when unset (emulator updates carry it too)
	if env("TELEGRAM_WEBHOOK_SECRET", "") == "" {
		os.Setenv("TELEGRAM_WEBHOOK_SECRET", randHex(24))
	}
	// payments mock: fake checkout page lives in this app; state survives restarts
	if env("PAYMENTS_PROVIDER", "mock") == "mock" {
		if env("PAYMENTS_MOCK_CHECKOUT_URL", "") == "" {
			os.Setenv("PAYMENTS_MOCK_CHECKOUT_URL", public+"/pay/{id}")
		}
		if env("PAYMENTS_MOCK_STATE_FILE", "") == "" {
			os.Setenv("PAYMENTS_MOCK_STATE_FILE", filepath.Join(dataDir, "payments-mock.json"))
		}
	}
	if env("MAX_WEBHOOK_SECRET", "") == "" {
		os.Setenv("MAX_WEBHOOK_SECRET", randHex(24))
	}
	bot, err := tg.FromEnv(tgkit.Env{Prefix: "TELEGRAM_"})
	if err != nil {
		return nil, err
	}
	mbot, err := mx.FromEnv(mxkit.Env{Prefix: "MAX_"})
	if err != nil {
		return nil, err
	}
	p, err := pay.FromEnv(paykit.Env{Prefix: "PAYMENTS_"})
	if err != nil {
		return nil, err
	}
	st, err := OpenStore(filepath.Join(dataDir, "booking.db"))
	if err != nil {
		return nil, err
	}
	var admins []int64
	for _, s := range strings.Split(env("ADMIN_IDS", ""), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			admins = append(admins, id)
		}
	}
	edition := env("EDITION", "intl")
	if edition != "ru" {
		edition = "intl"
	}
	// ru: card (YooKassa) only — Stars are an international product; intl: Stars + card (Stripe)
	defMethods := map[string]string{"ru": "card", "intl": "stars,card"}[edition]
	var methods []string
	for _, m := range strings.Split(env("PAY_METHODS", defMethods), ",") {
		if m = strings.TrimSpace(m); m == "card" || (m == "stars" && edition == "intl") {
			methods = append(methods, m)
		}
	}
	lang := map[string]string{"ru": "ru", "intl": "en"}[edition]
	live := env("TELEGRAM_PROVIDER", "mock") == "telegram"
	cfg := Config{
		Edition: edition, Lang: lang, OtherURL: env("OTHER_EDITION_URL", ""), PDAllowed: env("PD_ALLOWED", "false") == "true",
		PublicURL: public, TZ: tz, PayMethods: methods,
		DepositPercent: int64(envInt("DEPOSIT_PERCENT", 30)), HoldMinutes: envInt("HOLD_MINUTES", 15), LeadMinutes: envInt("LEAD_MINUTES", 60),
		AdminIDs: admins, Emulator: env("DEMO_EMULATOR", "true") == "true", DemoReset: env("DEMO_RESET", "false") == "true",
		WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"), PaymentsMock: p.Name() == "mock", TelegramLive: live,
		MaxSecret: os.Getenv("MAX_WEBHOOK_SECRET"), MaxLive: env("MAX_PROVIDER", "mock") == "max",
	}
	// R9 (data localisation): the public demo host is outside RU, so it must never receive real users.
	// Live bots (real Telegram/Max ids) run only on a RU host without DEMO_RESET.
	if !cfg.PDAllowed && (cfg.TelegramLive || cfg.MaxLive) {
		return nil, fmt.Errorf("live TELEGRAM/MAX provider needs PD_ALLOWED=true (a RU host): real users' data must stay in RU")
	}
	if cfg.MaxLive && edition != "ru" {
		return nil, fmt.Errorf("Max is part of the ru edition only")
	}
	maxm := newMaxMessenger(mbot, cfg.MaxLive, cfg.Emulator, cfg.MaxSecret, env("MAX_BOT_USERNAME", "bloom_salon_bot"))
	// notify: NOTIFY_TELEGRAM_PROVIDER=telegram + NOTIFY_TELEGRAM_BOT_TOKEN for a live bot (default mock)
	if env("MAX_PROVIDER", "mock") == "max" && env("NOTIFY_MAX_PROVIDER", "") == "" {
		os.Setenv("NOTIFY_MAX_PROVIDER", "max")
		os.Setenv("NOTIFY_MAX_BOT_TOKEN", os.Getenv("MAX_BOT_TOKEN"))
	}
	if live && env("NOTIFY_TELEGRAM_PROVIDER", "") == "" {
		os.Setenv("NOTIFY_TELEGRAM_PROVIDER", "telegram")
		os.Setenv("NOTIFY_TELEGRAM_BOT_TOKEN", os.Getenv("TELEGRAM_BOT_TOKEN"))
	}
	// RU edition: accounts (live mode + limits) and the real YooKassa TEST shop for live runs
	var accClient *acc.Client
	var livePay pay.Provider
	if edition == "ru" {
		if accClient, err = acc.FromEnv(acckit.Env{Prefix: "ACCOUNTS_"}); err != nil {
			return nil, err
		}
		if env("PAYMENTS_YOOKASSA_SHOP_ID", "") != "" && env("PAYMENTS_YOOKASSA_SECRET_KEY", "") != "" {
			livePay, err = pay.FromEnv(paykit.Env{Prefix: "PAYMENTS_", Lookup: func(k string) (string, bool) {
				if k == "PAYMENTS_PROVIDER" {
					return "yookassa", true
				}
				return os.LookupEnv(k)
			}})
			if err != nil {
				return nil, err
			}
		}
	}
	cfg.AccountsUI = env("ACCOUNTS_UI", "stub")
	cfg.WidgetURL = env("ACCOUNTS_WIDGET_URL", "https://auth.demo.sedlab.dev/widget.js")
	nc, err := notify.FromEnv(notifykit.Env{Prefix: "NOTIFY_"})
	if err != nil {
		return nil, err
	}
	app, err := NewApp(cfg, st, bot, maxm, p, nc, log)
	if err != nil {
		return nil, err
	}
	app.acc, app.livePay = accClient, livePay
	return app, nil
}

// NewApp wires the app (also used by tests).
func NewApp(cfg Config, st *Store, bot *tg.Bot, maxm *maxMessenger, p pay.Provider, n Notifier, log *slog.Logger) (*App, error) {
	tgm := newTelegramMessenger(bot, cfg.TelegramLive, cfg.Emulator, cfg.WebhookSecret)
	a := &App{cfg: cfg, store: st, msg: tgm, tgm: tgm, max: maxm, pay: p, notify: n, log: log, now: time.Now}
	if a.notify == nil {
		a.notify = notify.NewMockClient()
	}
	a.limiter = newIPLimiter(envInt("RATE_PER_MIN", 120), func() time.Time { return a.now() })
	if len(st.mustServices()) == 0 {
		if err := a.ResetDemo(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (s *Store) mustServices() []Service { v, _ := s.Services(false); return v }

// subcommand: "healthcheck" (Docker HEALTHCHECK, no curl in the image) and "snapshot <file>"
// (consistent SQLite copy for backups: VACUUM INTO while the app runs).
func subcommand(args []string) int {
	switch args[0] {
	case "healthcheck":
		c := &http.Client{Timeout: 3 * time.Second}
		res, err := c.Get("http://127.0.0.1:" + env("PORT", "8080") + "/health")
		if err != nil || res.StatusCode != 200 {
			return 1
		}
		return 0
	case "snapshot":
		if len(args) < 2 {
			return 2
		}
		st, err := OpenStore(filepath.Join(env("DATA_DIR", "./data"), "booking.db"))
		if err != nil {
			return 1
		}
		defer st.Close()
		os.Remove(args[1])
		if _, err := st.db.Exec(`VACUUM INTO ?`, args[1]); err != nil {
			slog.Error("snapshot", "err", err)
			return 1
		}
		return 0
	}
	return 2
}
