// booking — salon booking bot + Telegram Mini App (sedlab showcase S3).
// Thin glue over private sedlab modules: telegram-core (Mini App auth, Stars, webhooks), payments (YooKassa),
// notify (reminders; glue stub until the module ships). Max (VK) — Messenger seam, waiting for max-core.
package main

import (
	"context"
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
	tz, err := time.LoadLocation(env("SALON_TZ", "Europe/Moscow"))
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
	bot, err := tg.FromEnv(tgkit.Env{Prefix: "TELEGRAM_"})
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
	var methods []string
	for _, m := range strings.Split(env("PAY_METHODS", "yookassa,stars"), ",") {
		if m = strings.TrimSpace(m); m == "yookassa" || m == "stars" {
			methods = append(methods, m)
		}
	}
	live := env("TELEGRAM_PROVIDER", "mock") == "telegram"
	cfg := Config{
		PublicURL: public, TZ: tz, PayMethods: methods,
		DepositPercent: int64(envInt("DEPOSIT_PERCENT", 30)), HoldMinutes: envInt("HOLD_MINUTES", 15), LeadMinutes: envInt("LEAD_MINUTES", 60),
		AdminIDs: admins, Emulator: env("DEMO_EMULATOR", "true") == "true", DemoReset: env("DEMO_RESET", "false") == "true",
		WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"), PaymentsMock: p.Name() == "mock", TelegramLive: live,
	}
	// notify: NOTIFY_TELEGRAM_PROVIDER=telegram + NOTIFY_TELEGRAM_BOT_TOKEN for a live bot (default mock)
	if live && env("NOTIFY_TELEGRAM_PROVIDER", "") == "" {
		os.Setenv("NOTIFY_TELEGRAM_PROVIDER", "telegram")
		os.Setenv("NOTIFY_TELEGRAM_BOT_TOKEN", os.Getenv("TELEGRAM_BOT_TOKEN"))
	}
	nc, err := notify.FromEnv(notifykit.Env{Prefix: "NOTIFY_"})
	if err != nil {
		return nil, err
	}
	return NewApp(cfg, st, bot, p, nc, log)
}

// NewApp wires the app (also used by tests).
func NewApp(cfg Config, st *Store, bot *tg.Bot, p pay.Provider, n Notifier, log *slog.Logger) (*App, error) {
	tgm := newTelegramMessenger(bot, cfg.TelegramLive, cfg.Emulator, cfg.WebhookSecret)
	a := &App{cfg: cfg, store: st, msg: tgm, tgm: tgm, pay: p, notify: n, log: log, now: time.Now}
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
