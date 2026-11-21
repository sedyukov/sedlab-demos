// Orders API, Go implementation. Contract: ../contract/openapi.yaml, rules: ../contract/BEHAVIOR.md.
// Shares the SQLite database, migrations and the job queue with the Python (FastAPI) implementation.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type Config struct {
	Addr           string
	DBPath         string
	Edition        string // ru | intl
	WebDir         string
	ContractDir    string
	MigrationsDir  string
	TokenSecret    string
	SandboxPerHour int
	RatePerMin     int
	Worker         bool
	DevPyProxy     string
	AuthWidgetURL  string
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func loadConfig() Config {
	ed := env("EDITION", "intl")
	if ed != "ru" {
		ed = "intl"
	}
	return Config{
		Addr:           env("ADDR", ":8080"),
		DBPath:         env("DB_PATH", "./data/api.db"),
		Edition:        ed,
		WebDir:         env("WEB_DIR", "../web"),
		ContractDir:    env("CONTRACT_DIR", "../contract"),
		MigrationsDir:  env("MIGRATIONS_DIR", "../migrations"),
		TokenSecret:    os.Getenv("API_TOKEN_SECRET"),
		SandboxPerHour: envInt("SANDBOX_PER_HOUR", 20),
		RatePerMin:     envInt("RATE_PER_MIN", 600),
		Worker:         env("WORKER", "1") != "0",
		DevPyProxy:     os.Getenv("DEV_PY_PROXY"),
		AuthWidgetURL:  env("AUTH_WIDGET_URL", "https://auth.demo.sedlab.dev/widget.js"),
	}
}

func main() {
	cfg := loadConfig()
	db, err := openDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if len(os.Args) == 3 && os.Args[1] == "snapshot" { // consistent copy for the nightly backup (pre-backup.sh)
		_ = os.Remove(os.Args[2])
		if _, err := db.Exec(`VACUUM INTO ?`, os.Args[2]); err != nil {
			log.Fatalf("snapshot: %v", err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := migrate(ctx, db, cfg.MigrationsDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	app, err := newApp(ctx, cfg, db)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	if cfg.Worker {
		go app.runWorker(ctx)
	}
	go app.runHousekeeping(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Printf("api-go listening on %s (edition=%s, worker=%v)", cfg.Addr, cfg.Edition, cfg.Worker)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
