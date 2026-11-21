package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sedyukov/sedlab-modules/modules/accounts/go"
)

const maxBody = 64 << 10

type App struct {
	cfg      Config
	db       *sql.DB
	secret   []byte
	acc      *accounts.Client // nil unless EDITION=ru
	rate     *bucketLimiter
	sandbox  *windowLimiter
	routes   []route
	webFiles http.Handler
	pyProxy  http.Handler
	touchMu  sync.Mutex
	touched  map[string]time.Time
}

func newApp(ctx context.Context, cfg Config, db *sql.DB) (*App, error) {
	a := &App{
		cfg:     cfg,
		db:      db,
		rate:    newBucketLimiter(cfg.RatePerMin),
		sandbox: newWindowLimiter(cfg.SandboxPerHour, time.Hour),
		touched: map[string]time.Time{},
	}
	if cfg.TokenSecret != "" {
		a.secret = []byte(cfg.TokenSecret)
	} else {
		var s string
		if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'token_secret'`).Scan(&s); err != nil {
			return nil, err
		}
		a.secret = []byte(s)
	}
	if cfg.Edition == "ru" {
		acc, err := newAccounts()
		if err != nil {
			return nil, err
		}
		a.acc = acc
	}
	a.webFiles = http.FileServer(http.Dir(cfg.WebDir))
	if cfg.DevPyProxy != "" {
		u, err := url.Parse(cfg.DevPyProxy)
		if err != nil {
			return nil, err
		}
		a.pyProxy = http.StripPrefix("/py", httputil.NewSingleHostReverseProxy(u))
	}
	a.routes = a.apiRoutes()
	return a, nil
}

// ---- errors ----

type detail struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

type apiError struct {
	Status     int
	Code       string
	Message    string
	Details    []detail
	RetryAfter int
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func errf(status int, code, msg string) *apiError {
	return &apiError{Status: status, Code: code, Message: msg}
}

func validationErr(d []detail) *apiError {
	return &apiError{Status: 422, Code: "validation_error", Message: "request validation failed", Details: d}
}

func notFound(what string) *apiError { return errf(404, "not_found", what+" not found") }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	b, _ := json.Marshal(v)
	_, _ = w.Write(b)
}

func writeErr(w http.ResponseWriter, e *apiError) {
	body := map[string]any{"code": e.Code, "message": e.Message}
	if len(e.Details) > 0 {
		body["details"] = e.Details
	}
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", itoa(int64(e.RetryAfter)))
	}
	writeJSON(w, e.Status, map[string]any{"error": body})
}

// ---- routing ----

// req is what an API handler gets: parsed path params, the raw body and (for authed routes) the principal.
type req struct {
	r      *http.Request
	params map[string]string
	body   []byte
	p      *principal
}

// result: status + JSON value (nil value → empty body, e.g. 204).
type result struct {
	status int
	value  any
}

type handlerFunc func(ctx context.Context, q *req) (result, *apiError)

type route struct {
	method string
	parts  []string
	auth   bool
	idem   bool // POST that honours Idempotency-Key
	h      handlerFunc
}

func (a *App) apiRoutes() []route {
	mk := func(method, pattern string, auth, idem bool, h handlerFunc) route {
		return route{method: method, parts: strings.Split(strings.Trim(pattern, "/"), "/"), auth: auth, idem: idem, h: h}
	}
	return []route{
		mk("GET", "/health", false, false, a.health),
		mk("POST", "/v1/auth/sandbox", false, false, a.createSandbox),
		mk("POST", "/v1/auth/accounts", false, false, a.exchangeAccounts),
		mk("GET", "/v1/me", true, false, a.me),
		mk("GET", "/v1/customers", true, false, a.listCustomers),
		mk("POST", "/v1/customers", true, true, a.createCustomer),
		mk("GET", "/v1/customers/{id}", true, false, a.getCustomer),
		mk("PATCH", "/v1/customers/{id}", true, false, a.updateCustomer),
		mk("DELETE", "/v1/customers/{id}", true, false, a.deleteCustomer),
		mk("GET", "/v1/orders", true, false, a.listOrders),
		mk("POST", "/v1/orders", true, true, a.createOrder),
		mk("GET", "/v1/orders/{id}", true, false, a.getOrder),
		mk("PATCH", "/v1/orders/{id}", true, false, a.updateOrder),
		mk("DELETE", "/v1/orders/{id}", true, false, a.deleteOrder),
		mk("POST", "/v1/orders/{id}/jobs", true, true, a.enqueueJob),
		mk("GET", "/v1/jobs", true, false, a.listJobs),
		mk("GET", "/v1/jobs/{id}", true, false, a.getJob),
	}
}

func matchRoute(parts, path []string) (map[string]string, bool) {
	if len(parts) != len(path) {
		return nil, false
	}
	params := map[string]string{}
	for i, p := range parts {
		if strings.HasPrefix(p, "{") {
			if path[i] == "" {
				return nil, false
			}
			params[p[1:len(p)-1]] = path[i]
		} else if p != path[i] {
			return nil, false
		}
	}
	return params, true
}

var reqIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// Handler: /go/* and /* are served by Go (web page, contract, API); /py/* goes to Python (Caddy in prod,
// DEV_PY_PROXY locally).
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/py/") && a.pyProxy != nil {
			a.pyProxy.ServeHTTP(w, r)
			return
		}
		if path == "/go" || strings.HasPrefix(path, "/go/") {
			path = strings.TrimPrefix(path, "/go")
			if path == "" {
				path = "/"
			}
		}
		if path == "/health" || strings.HasPrefix(path, "/v1/") || path == "/v1" {
			a.serveAPI(w, r, path)
			return
		}
		a.serveWeb(w, r, path)
	})
}

func (a *App) serveWeb(w http.ResponseWriter, r *http.Request, path string) {
	switch path {
	case "/openapi.yaml":
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(a.cfg.ContractDir, "openapi.yaml"))
		return
	case "/ui-config.json":
		w.Header().Set("Cache-Control", "no-cache")
		cfg := map[string]any{"edition": a.cfg.Edition, "accounts": a.acc != nil}
		if a.acc != nil {
			cfg["auth_widget_url"] = a.cfg.AuthWidgetURL
		}
		writeJSON(w, 200, cfg)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErr(w, errf(405, "method_not_allowed", "method not allowed"))
		return
	}
	if path != "/" {
		if _, err := os.Stat(filepath.Join(a.cfg.WebDir, filepath.Clean("/"+path))); err != nil {
			writeErr(w, errf(404, "not_found", "route not found"))
			return
		}
	}
	if strings.HasPrefix(path, "/vendor/") {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	a.webFiles.ServeHTTP(w, r2)
}

func (a *App) serveAPI(w http.ResponseWriter, r *http.Request, path string) {
	rid := r.Header.Get("X-Request-Id")
	if !reqIDRe.MatchString(rid) {
		var b [8]byte
		_, _ = rand.Read(b[:])
		rid = hex.EncodeToString(b[:])
	}
	h := w.Header()
	h.Set("X-Backend", "go")
	h.Set("X-Request-Id", rid)
	h.Set("Cache-Control", "no-store")
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("panic %s %s: %v", r.Method, path, rec)
			writeErr(w, errf(500, "internal", "internal error"))
		}
	}()

	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	var rt *route
	var params map[string]string
	methodMismatch := false
	for i := range a.routes {
		p, ok := matchRoute(a.routes[i].parts, segs)
		if !ok {
			continue
		}
		if a.routes[i].method != r.Method {
			methodMismatch = true
			continue
		}
		rt, params = &a.routes[i], p
		break
	}
	if rt == nil {
		if methodMismatch {
			writeErr(w, errf(405, "method_not_allowed", "method not allowed"))
		} else {
			writeErr(w, errf(404, "not_found", "route not found"))
		}
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		writeErr(w, validationErr([]detail{{"body", "invalid_json"}}))
		return
	}
	if len(body) > maxBody {
		writeErr(w, errf(413, "payload_too_large", "request body too large"))
		return
	}
	q := &req{r: r, params: params, body: body}
	ctx := r.Context()
	if rt.auth {
		p, e := a.authenticate(ctx, r)
		if e != nil {
			writeErr(w, e)
			return
		}
		if wait := a.rate.take(p.WS); wait > 0 {
			e := errf(429, "rate_limited", "too many requests")
			e.RetryAfter = wait
			writeErr(w, e)
			return
		}
		q.p = p
	}
	if rt.idem && q.p != nil {
		a.serveIdempotent(w, q, path, rt.h)
		return
	}
	res, e := rt.h(ctx, q)
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, res.status, res.value)
}

// clientIP: Caddy sets X-Forwarded-For to the real client address (it ignores the one sent by the client).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *App) health(ctx context.Context, q *req) (result, *apiError) {
	var one int
	if err := a.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return result{}, errf(500, "internal", "internal error")
	}
	return result{200, map[string]any{"status": "ok", "backend": "go", "edition": a.cfg.Edition, "db": "ok"}}, nil
}

func dbErr(err error) *apiError {
	if errors.Is(err, context.Canceled) {
		return errf(500, "internal", "internal error")
	}
	log.Printf("db error: %v", err)
	return errf(500, "internal", "internal error")
}
