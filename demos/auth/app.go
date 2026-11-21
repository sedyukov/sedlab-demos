package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	maxcore "github.com/sedyukov/sedlab-modules/modules/max-core/go"
	mkit "github.com/sedyukov/sedlab-modules/modules/max-core/go/kit"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	nkit "github.com/sedyukov/sedlab-modules/modules/notify/go/kit"
)

//go:embed web
var webFS embed.FS

const (
	cookieAuth    = "sl_auth"
	cookieLogin   = "sl_login"   // binds a bot deep-link login to this browser
	cookiePending = "sl_pending" // proven identity waiting for consent (no DB row yet)
	sessionTTL    = 30 * 24 * 3600
	nonceTTL      = 5 * 60
	pendingTTL    = 15 * 60
	inactiveTTL   = 365 * 24 * 3600
)

type App struct {
	cfg       *Config
	store     *Store
	signer    *Signer
	limits    *Limits
	providers *Providers
	max       *maxcore.Bot
	notify    *notify.Client
	tpl       *template.Template
	pendKey   []byte
	now       func() int64
	rl        *rateLimiter
}

func NewApp(cfg *Config) (*App, error) {
	db, err := openDB(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	st := NewStore(db, "sqlite")
	if err := st.Migrate(context.Background()); err != nil {
		return nil, err
	}
	return newAppWithStore(cfg, st)
}

func newAppWithStore(cfg *Config, st *Store) (*App, error) {
	signer, err := NewSigner(cfg.SigningKey, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	lim, err := LoadLimits(cfg.LimitsFile)
	if err != nil {
		return nil, err
	}
	// MAX_* / NOTIFY_* — module env (mock by default). Telegram is not used for login (149-ФЗ, owner's decision).
	// Live provider requested but no token yet (bot not created): start anyway on mock and hide Max sign-in,
	// so the service can be deployed before the owner creates the bot.
	maxLive := true
	if os.Getenv("MAX_PROVIDER") == "max" && os.Getenv("MAX_BOT_TOKEN") == "" {
		slog.Warn("MAX_BOT_TOKEN not set: Max sign-in disabled until the bot token is configured")
		os.Setenv("MAX_PROVIDER", "mock")
		maxLive = false
	}
	if os.Getenv("NOTIFY_MAX_PROVIDER") == "max" && os.Getenv("NOTIFY_MAX_BOT_TOKEN") == "" {
		slog.Warn("NOTIFY_MAX_BOT_TOKEN not set: lead notifications go to the mock journal")
		os.Setenv("NOTIFY_MAX_PROVIDER", "mock")
	}
	maxBot, err := maxcore.FromEnv(mkit.Env{Prefix: "MAX_"})
	if err != nil {
		return nil, err
	}
	nc, err := notify.FromEnv(nkit.Env{Prefix: "NOTIFY_"})
	if err != nil {
		return nil, err
	}
	tpl, err := template.New("").Funcs(template.FuncMap{
		"t": func(lang, ru, en string) string {
			if lang == "en" {
				return en
			}
			return ru
		},
	}).ParseFS(webFS, "web/*.html")
	if err != nil {
		return nil, err
	}
	pk := hmac.New(sha256.New, []byte(cfg.SigningKey))
	pk.Write([]byte("sedlab-auth-pending"))
	prov := NewProviders(cfg)
	prov.maxOff = !maxLive
	return &App{cfg: cfg, store: st, signer: signer, limits: lim, providers: prov, max: maxBot, notify: nc, tpl: tpl,
		pendKey: pk.Sum(nil), now: func() int64 { return time.Now().Unix() }, rl: newRateLimiter(20, time.Minute)}, nil
}

func (a *App) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	m.HandleFunc("GET /.well-known/jwks.json", a.jwks)
	static, _ := fs.Sub(webFS, "web/static")
	m.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	m.HandleFunc("GET /widget.js", a.widgetJS)

	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/account", http.StatusFound) })
	m.HandleFunc("GET /login", a.loginPage)
	m.HandleFunc("GET /login/oauth/{id}/start", a.oauthStart)
	m.HandleFunc("GET /login/oauth/{id}/callback", a.oauthCallback)
	m.HandleFunc("POST /api/login/start", a.limited(a.loginStart))
	m.HandleFunc("GET /api/login/poll", a.loginPoll)
	m.HandleFunc("POST /api/login/webapp", a.limited(a.loginWebApp))
	m.HandleFunc("GET /consent", a.consentPage)
	m.HandleFunc("POST /consent", a.consentSubmit)
	m.HandleFunc("POST /api/login/consent", a.consentAPI)
	m.HandleFunc("POST /logout", a.logout)
	m.HandleFunc("GET /account", a.accountPage)
	m.HandleFunc("GET /account/export", a.accountExport)
	m.HandleFunc("POST /account/delete", a.accountDelete)
	m.HandleFunc("POST /account/contact", a.accountContact)
	m.HandleFunc("GET /privacy", a.legalPage("privacy"))
	m.HandleFunc("GET /terms", a.legalPage("terms"))
	m.HandleFunc("GET /consent/demo", a.legalPage("consent-demo"))
	m.HandleFunc("GET /consent/contact", a.legalPage("consent-contact"))
	m.HandleFunc("GET /consent/voice", a.legalPage("consent-voice"))   // legal §1а (основной адрес: sedlab.dev/consent/voice)
	m.HandleFunc("GET /consent/prices", a.legalPage("consent-prices")) // legal §1б

	m.HandleFunc("GET /api/v1/me", a.cors(a.apiMe))
	m.HandleFunc("GET /api/v1/me/limits", a.cors(a.apiMeLimits))
	m.HandleFunc("OPTIONS /api/v1/", a.cors(func(w http.ResponseWriter, r *http.Request) {}))

	m.HandleFunc("POST /max/webhook", a.maxWebhook)

	m.HandleFunc("POST /internal/v1/{op}", a.internal)
	m.HandleFunc("GET /internal/v1/leads.jsonl", a.leadsExport)

	m.HandleFunc("GET /admin/leads", a.adminLeads)
	m.HandleFunc("GET /admin/leads/{id}", a.adminLeads)

	if a.cfg.Dev {
		m.HandleFunc("GET /dev/login", a.devLogin)
	}
	return secureHeaders(m)
}

func secureHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if !strings.HasPrefix(r.URL.Path, "/widget.js") && !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			w.Header().Set("X-Frame-Options", "DENY")
		}
		h.ServeHTTP(w, r)
	})
}

// ---- helpers ----------------------------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// moduleErr: ModuleError shape (kit errors.schema.json).
func moduleErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"code": code, "message": msg, "retryable": status >= 500, "provider": "", "http_status": 0})
}

func langOf(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l == "en" || l == "ru" {
		return l
	}
	if c, err := r.Cookie("sl_lang"); err == nil && (c.Value == "en" || c.Value == "ru") {
		return c.Value
	}
	al := strings.ToLower(r.Header.Get("Accept-Language"))
	if al != "" && !strings.HasPrefix(al, "ru") {
		return "en"
	}
	return "ru"
}

// safeReturn: only https://<x>.demo.sedlab.dev/... (or the auth host itself); anything else → /account.
func (a *App) safeReturn(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return "/account"
	}
	if u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(raw, "//") {
		return u.RequestURI()
	}
	host := strings.ToLower(u.Hostname())
	okScheme := u.Scheme == "https" || (a.cfg.Dev && u.Scheme == "http")
	if isIntlHost(host, a.cfg.ReturnSuffix) { // INTL editions (<app>-en) don't use the RU account
		return "/account"
	}
	if okScheme && u.User == nil && (strings.HasSuffix(host, a.cfg.ReturnSuffix) || host == strings.TrimPrefix(a.cfg.ReturnSuffix, ".") ||
		(a.cfg.Dev && (host == "localhost" || host == "127.0.0.1"))) {
		return u.String()
	}
	return "/account"
}

func (a *App) setCookie(w http.ResponseWriter, name, val string, maxAge int, wide bool) {
	c := &http.Cookie{Name: name, Value: val, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: !a.cfg.Dev, SameSite: http.SameSiteLaxMode}
	if wide && !a.cfg.Dev {
		c.Domain = a.cfg.CookieDomain
	}
	http.SetCookie(w, c)
}

func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if c, err := r.Cookie(cookieAuth); err == nil {
		return c.Value
	}
	return ""
}

// authUser: verify token offline, then the session online (revocation). nil = anonymous.
func (a *App) authUser(ctx context.Context, token string) (*User, *Session) {
	if token == "" {
		return nil, nil
	}
	c, err := a.signer.Verify(token, a.now())
	if err != nil {
		return nil, nil
	}
	s, err := a.store.GetSession(ctx, c.Sid)
	if err != nil || !s.Active(a.now()) || s.UserID != c.Sub {
		return nil, nil
	}
	u, err := a.store.GetUser(ctx, c.Sub)
	if err != nil {
		return nil, nil
	}
	if u.ConsentVersion == "" { // created but PD consent never recorded (crash between steps): not an account yet
		return nil, nil
	}
	a.store.Touch(ctx, u, a.now())
	return u, s
}

func (a *App) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	lang := langOf(r)
	data["Lang"] = lang
	data["Path"] = r.URL.Path
	data["Query"] = r.URL.RawQuery
	data["PrivacyURL"] = a.cfg.PrivacyURL
	data["PDEmail"] = os.Getenv("AUTH_PD_EMAIL")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := a.tpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render", "tpl", name, "err", err)
	}
}

// ---- pending identity (signed cookie / token until consent) -----------------------------------------------

type pending struct {
	Prov string `json:"p"`
	UID  string `json:"u"`
	Name string `json:"n"`
	User string `json:"un,omitempty"`
	Lang string `json:"l,omitempty"`
	Ret  string `json:"r,omitempty"`
	Demo string `json:"d,omitempty"`
	Exp  int64  `json:"e"`
}

func (a *App) sealPending(p pending) string {
	p.Exp = a.now() + pendingTTL
	b, _ := json.Marshal(p)
	m := hmac.New(sha256.New, a.pendKey)
	m.Write(b)
	return b64.EncodeToString(b) + "." + b64.EncodeToString(m.Sum(nil))
}

func (a *App) openPending(s string) (*pending, error) {
	body, mac, ok := strings.Cut(s, ".")
	if !ok {
		return nil, errors.New("bad pending")
	}
	b, err1 := b64.DecodeString(body)
	sig, err2 := b64.DecodeString(mac)
	if err1 != nil || err2 != nil {
		return nil, errors.New("bad pending")
	}
	m := hmac.New(sha256.New, a.pendKey)
	m.Write(b)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return nil, errors.New("bad pending")
	}
	var p pending
	if json.Unmarshal(b, &p) != nil || p.Exp < a.now() {
		return nil, errors.New("pending expired")
	}
	return &p, nil
}

// finishLogin: known identity → session now; new one → consent first. Returns token ("" if consent needed) and pending.
func (a *App) finishLogin(ctx context.Context, p pending) (token, pend string, err error) {
	uid, err := a.store.FindIdentity(ctx, p.Prov, p.UID)
	if err == nil {
		if u, gerr := a.store.GetUser(ctx, uid); gerr == nil && u.ConsentVersion == "" {
			err = errNotFound // identity without recorded consent → consent screen again
		}
	}
	if errors.Is(err, errNotFound) {
		return "", a.sealPending(p), nil
	}
	if err != nil {
		return "", "", err
	}
	if _, _, err := a.store.UpsertUser(ctx, Profile{Provider: p.Prov, ProviderUID: p.UID, DisplayName: p.Name, Username: p.User, Lang: p.Lang}, nil, "", a.now()); err != nil {
		return "", "", err
	}
	tok, err := a.issue(ctx, uid, p.Prov)
	return tok, "", err
}

func (a *App) issue(ctx context.Context, userID, prov string) (string, error) {
	now := a.now()
	sid, err := a.store.CreateSession(ctx, userID, prov, sessionTTL, now)
	if err != nil {
		return "", err
	}
	return a.signer.Issue(userID, sid, prov, now, now+sessionTTL), nil
}

// browser-side completion: cookie + redirect (or consent page).
func (a *App) completeBrowser(w http.ResponseWriter, r *http.Request, p pending) {
	tok, pend, err := a.finishLogin(r.Context(), p)
	if err != nil {
		slog.Error("login", "err", err)
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	if pend != "" {
		a.setCookie(w, cookiePending, pend, pendingTTL, false)
		http.Redirect(w, r, "/consent", http.StatusSeeOther)
		return
	}
	a.setCookie(w, cookieAuth, tok, sessionTTL, true)
	http.Redirect(w, r, a.safeReturn(p.Ret), http.StatusSeeOther)
}

// ---- rate limit (in memory, per IP; IPs are never stored) -------------------------------------------------

type rateLimiter struct {
	mu     sync.Mutex
	n      int
	win    time.Duration
	counts map[string]int
	reset  time.Time
}

func newRateLimiter(n int, win time.Duration) *rateLimiter {
	return &rateLimiter{n: n, win: win, counts: map[string]int{}, reset: time.Now().Add(win)}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().After(l.reset) {
		l.counts, l.reset = map[string]int{}, time.Now().Add(l.win)
	}
	l.counts[key]++
	return l.counts[key] <= l.n
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" { // set by our Caddy edge
		ip, _, _ = strings.Cut(ip, ",")
		return strings.TrimSpace(ip)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func (a *App) limited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.rl.allow(clientIP(r)) {
			moduleErr(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts")
			return
		}
		h(w, r)
	}
}

// ---- background -------------------------------------------------------------------------------------------

func (a *App) Background(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	last := time.Time{}
	for {
		a.notifyLeads(ctx)
		if time.Since(last) > 6*time.Hour {
			a.store.PurgeNonces(ctx, a.now())
			if n, err := a.store.PurgeInactive(ctx, a.now()-inactiveTTL); err == nil && n > 0 {
				slog.Info("purged inactive accounts", "count", n)
			}
			last = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b64.EncodeToString(b)
}

// isIntlHost: <app>-en.demo.sedlab.dev is the INTL edition (DO, mock only, no RU sign-in). Only RU editions
// (<app>.demo.sedlab.dev on the RU host) may use this service as return target or CORS origin.
func isIntlHost(host, suffix string) bool {
	return strings.HasSuffix(host, suffix) && strings.HasSuffix(strings.TrimSuffix(host, suffix), "-en")
}
