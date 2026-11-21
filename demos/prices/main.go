// prices: showcase "price & stock monitoring on Wildberries and Ozon": competitor cards are collected through
// the scraping module, our catalogue / prices / reviews go through the marketplaces module, the daily report through
// notify, rows into sheets-storage, review reply drafts through ai. Works with no keys (mock providers).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	accountskit "github.com/sedyukov/sedlab-modules/modules/accounts/go/kit"
	ai "github.com/sedyukov/sedlab-modules/modules/ai/go"
	aikit "github.com/sedyukov/sedlab-modules/modules/ai/go/kit"
)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

var llmNames = map[string]string{"gigachat": "GigaChat", "yandexgpt": "YandexGPT", "openai": "OpenAI", "anthropic": "Anthropic"}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := env("ADDR", ":8080")
	edition := strings.ToLower(env("EDITION", ""))
	if edition != "" && edition != EditionRU && edition != EditionIntl {
		logger.Error("config", "err", "EDITION must be ru, intl or empty")
		os.Exit(1)
	}
	// AI: AI_LLM_PROVIDER (+ the module's AI_* keys); default mock. The INTL edition never goes live.
	// legal-ru: only depersonalised text may go to GigaChat; reviews are buyers' texts, so GigaChat never goes live here
	// (no anonymiser yet). YandexGPT is the live LLM of the RU edition.
	if strings.EqualFold(env("AI_LLM_PROVIDER", ""), "gigachat") {
		logger.Warn("config", "msg", "GigaChat is never live in this demo (legal-ru): using the mock")
		os.Setenv("AI_LLM_PROVIDER", "mock")
	}
	llm, err := ai.FromEnv(aikit.Env{Prefix: "AI_"})
	if err != nil {
		logger.Error("ai config", "err", err.Error()) // module config errors never contain secret values
		os.Exit(1)
	}
	llmName := llmNames[env("AI_LLM_PROVIDER", "")]
	if llmName == "" {
		llmName = "YandexGPT"
		if edition == EditionIntl {
			llmName = "OpenAI"
		}
	}
	maxSessions, _ := strconv.Atoi(env("MAX_SESSIONS", "2000"))
	app := &App{Edition: edition, OtherURL: env("OTHER_EDITION_URL", ""), Sessions: NewSessions(max(10, maxSessions)),
		LLM: llm, LLMName: llmName, Collector: DefaultCollector(), Schedule: env("DAILY_AT", "09:00")}
	if edition == EditionIntl {
		app.Sessions.Lang = "en"
		if llmMode(llm) == "live" { // International edition: mock only (owner's decision)
			logger.Error("config", "err", "EDITION=intl runs the AI in mock mode only: unset AI_LLM_PROVIDER")
			os.Exit(1)
		}
	}
	if edition == EditionRU {
		acc, err := accounts.FromEnv(accountskit.Env{Prefix: "ACCOUNTS_"})
		if err != nil {
			logger.Error("accounts config", "err", err.Error())
			os.Exit(1)
		}
		app.Acc, app.AuthWidgetURL = acc, env("AUTH_WIDGET_URL", "")
		app.LLMResource = "yandex-llm"
		logger.Info("accounts", "provider", acc.ProviderName(), "demo", acc.Demo(), "widget", app.AuthWidgetURL != "")
	}
	if d := os.Getenv("DEMO_DOMAIN"); d != "" && app.OtherURL == "" {
		switch edition {
		case EditionRU:
			app.OtherURL = "https://prices-en." + d
		case EditionIntl:
			app.OtherURL = "https://prices." + d
		}
	}
	go janitor(app, logger, parseHour(env("WIPE_HOUR_UTC", "3")))

	srv := &http.Server{Addr: addr, Handler: NewServer(app, DefaultGuard()), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("listening", "addr", addr, "edition", edition, "llm", llmMode(llm))
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
		if u := now.UTC(); u.Hour() == hour && u.YearDay() != lastWipe {
			lastWipe = u.YearDay()
			n += app.Sessions.Purge(0)
			logger.Info("daily wipe")
		}
		if n > 0 {
			logger.Info("purged sessions", "n", n, "left", app.Sessions.Len())
		}
	}
}
