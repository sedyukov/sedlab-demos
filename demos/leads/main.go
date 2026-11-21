// leads — showcase "form → CRM → Google Sheet → Telegram, payment → deal won" with a live event log.
// Thin glue over the sedlab modules crm, payments, sheets-storage (+ notify). Works with no keys (mock providers).
package main

import (
	"context"
	crmkit "github.com/sedyukov/sedlab-modules/modules/crm/go/kit"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := env("ADDR", ":8080")
	port := addr[strings.LastIndex(addr, ":")+1:]
	public := env("PUBLIC_URL", "http://localhost:"+port)
	if d := os.Getenv("DEMO_DOMAIN"); d != "" && os.Getenv("PUBLIC_URL") == "" {
		public = "https://leads." + d
		if strings.EqualFold(os.Getenv("EDITION"), EditionIntl) {
			public = "https://leads-en." + d
		}
	}
	edition := strings.ToLower(env("EDITION", ""))
	profiles, err := LoadProfiles(nil, edition)
	if err != nil {
		logger.Error("config", "err", err.Error()) // module config errors never contain secret values
		os.Exit(1)
	}
	maxSessions, _ := strconv.Atoi(env("MAX_SESSIONS", "2000"))
	app := NewApp(profiles, public, env("SELF_URL", "http://127.0.0.1:"+port), maxSessions)
	app.Edition = edition
	app.OtherURL = env("OTHER_EDITION_URL", "")
	if d := os.Getenv("DEMO_DOMAIN"); d != "" && app.OtherURL == "" {
		switch edition {
		case EditionRU:
			app.OtherURL = "https://leads-en." + d
		case EditionIntl:
			app.OtherURL = "https://leads." + d
		}
	}
	guard := DefaultGuard()
	if v, err := strconv.Atoi(env("SUBMIT_PER_MIN", "")); err == nil && v > 0 {
		guard.Submit = crmkit.NewKeyedLimiter(float64(v)/60, max(3, v/4))
	}
	guard.SandboxContacts = env("ACCEPT_REAL_CONTACTS", "") != "1"
	if o := env("ALLOWED_ORIGINS", ""); o != "" {
		guard.AllowedOrigins = strings.Split(o, ",")
	}
	go janitor(app, logger, parseHour(env("WIPE_HOUR_UTC", "3")))

	srv := &http.Server{Addr: addr, Handler: NewServer(app, guard), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		args := []any{"addr", addr, "public", public, "edition", edition}
		for _, k := range ProfileOrder {
			if p := profiles[k]; p != nil {
				args = append(args, k, p.Modes)
			}
		}
		logger.Info("listening", args...)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("serve", "err", err.Error())
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func parseHour(s string) int {
	h, err := strconv.Atoi(s)
	if err != nil || h < 0 || h > 23 {
		return 3
	}
	return h
}

// janitor: sessions idle > 6 h are dropped every 10 min; everything is wiped once a day at WIPE_HOUR_UTC.
func janitor(app *App, logger *slog.Logger, hour int) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	lastWipe := time.Now().UTC().YearDay()
	for now := range t.C {
		n := app.Sessions.Purge(6 * time.Hour)
		u := now.UTC()
		if u.Hour() == hour && u.YearDay() != lastWipe {
			lastWipe = u.YearDay()
			n += app.Sessions.Purge(0)
			logger.Info("daily wipe")
		}
		if n > 0 {
			logger.Info("purged sessions", "n", n, "left", app.Sessions.Len())
		}
	}
}
