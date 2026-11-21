// auth — единый аккаунт sedlab для всех *.demo.sedlab.dev: вход Max / Яндекс ID / VK ID, согласие, лимиты живого режима, лиды.
// Design: freelance-ops accounts/DESIGN.md. Runs on a RU host (152-ФЗ); demos verify tokens offline by public key.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	Port         string
	DataDir      string
	PublicURL    string // https://auth.demo.sedlab.dev
	Issuer       string
	CookieDomain string // .demo.sedlab.dev
	ReturnSuffix string // allowed return hosts: *.demo.sedlab.dev
	SigningKey   string
	ServiceKeys  map[string]string // service key → demo
	InternalIPs  map[string]bool   // AUTH_INTERNAL_ALLOW_IPS: callers allowed on /internal (empty = any)
	MaxBotUser   string
	MaxBotLink   string          // deep-link base, default https://max.ru/<bot>
	OwnerMaxUser string          // owner's Max user_id: lead notifications
	Admins       map[string]bool // "max:<uid>" identities allowed into /admin
	PrivacyURL   string          // link target for "Политика" (sedlab.dev/privacy once the storefront is live)
	LimitsFile   string
	Dev          bool   // http cookies, local emulator login
	ConsentVer   string // demo-v1  (legal/consents.md §1)
	TermsVer     string // terms-v1 (legal/terms-demo.*.md)
	ContactVer   string // contact-v1 (legal/consents.md §3)
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func LoadConfig() (*Config, error) {
	c := &Config{
		Port:         env("PORT", "8080"),
		DataDir:      env("DATA_DIR", "./data"),
		PublicURL:    strings.TrimRight(env("AUTH_PUBLIC_URL", "https://auth.demo.sedlab.dev"), "/"),
		Issuer:       env("AUTH_ISSUER", "auth.demo.sedlab.dev"),
		CookieDomain: env("AUTH_COOKIE_DOMAIN", ".demo.sedlab.dev"),
		ReturnSuffix: env("AUTH_RETURN_SUFFIX", ".demo.sedlab.dev"),
		SigningKey:   os.Getenv("AUTH_SIGNING_KEY"),
		MaxBotUser:   strings.TrimPrefix(env("AUTH_MAX_BOT_USERNAME", ""), "@"),
		OwnerMaxUser: env("AUTH_OWNER_MAX_USER_ID", ""),
		LimitsFile:   env("AUTH_LIMITS_FILE", ""),
		Dev:          env("AUTH_DEV", "") == "1",
		ConsentVer:   env("AUTH_CONSENT_VERSION", "demo-v2"),
		TermsVer:     env("AUTH_TERMS_VERSION", "terms-v2"),
		ContactVer:   env("AUTH_CONTACT_VERSION", "contact-v1"),
		PrivacyURL:   env("AUTH_PRIVACY_URL", "/privacy"),
		ServiceKeys:  map[string]string{},
		Admins:       map[string]bool{},
		InternalIPs:  map[string]bool{},
	}
	c.MaxBotLink = env("AUTH_MAX_BOT_LINK", "https://max.ru/"+c.MaxBotUser)
	// AUTH_SVC_<DEMO>=<key> — one service key per demo; AUTH_SVC_ADMIN for lead export.
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "AUTH_SVC_") && len(v) >= 24 {
			c.ServiceKeys[v] = strings.ToLower(strings.TrimPrefix(k, "AUTH_SVC_"))
		}
	}
	for _, ip := range strings.Split(env("AUTH_INTERNAL_ALLOW_IPS", ""), ",") {
		if ip = strings.TrimSpace(ip); ip != "" {
			c.InternalIPs[ip] = true
		}
	}
	def := ""
	if c.OwnerMaxUser != "" {
		def = "max:" + c.OwnerMaxUser
	}
	for _, id := range strings.Split(env("AUTH_ADMINS", def), ",") { // e.g. "max:123,yandex:456"
		if id = strings.TrimSpace(id); id != "" {
			c.Admins[id] = true
		}
	}
	if c.SigningKey == "" {
		if !c.Dev {
			return nil, fmt.Errorf("AUTH_SIGNING_KEY is required (generate: auth keygen)")
		}
		c.SigningKey, _, _ = GenerateKeypair()
	}
	return c, nil
}

func openDB(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dataDir, "auth.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; serialises consume transactions
	return db, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keygen":
			seed, pub, kid := GenerateKeypair()
			fmt.Printf("AUTH_SIGNING_KEY=%s\nACCOUNTS_PUBLIC_KEY=%s\n# kid=%s\n", seed, pub, kid)
			return
		case "healthcheck":
			r, err := http.Get("http://127.0.0.1:" + env("PORT", "8080") + "/health")
			if err != nil || r.StatusCode != 200 {
				os.Exit(1)
			}
			return
		case "backup": // auth backup /data/snapshot.db — consistent copy for restic (pre-backup.sh)
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: auth backup <path>")
				os.Exit(2)
			}
			db, err := openDB(env("DATA_DIR", "./data"))
			if err == nil {
				_ = os.Remove(os.Args[2])
				err = NewStore(db, "sqlite").Snapshot(context.Background(), os.Args[2])
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "backup:", err)
				os.Exit(1)
			}
			return
		}
	}
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	app, err := NewApp(cfg)
	if err != nil {
		slog.Error("init", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: app.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.Background(ctx)
	go func() {
		slog.Info("auth listening", "port", cfg.Port, "public", cfg.PublicURL, "dev", cfg.Dev, "kid", app.signer.Kid())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
}
