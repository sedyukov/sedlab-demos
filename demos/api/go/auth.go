package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sedyukov/sedlab-modules/modules/accounts/go"
	"github.com/sedyukov/sedlab-modules/modules/accounts/go/kit"
)

const (
	tokenIssuer = "sedlab-api"
	tokenTTL    = 24 * time.Hour
)

type principal struct {
	WS        string
	AM        string
	CreatedAt string
	Exp       int64
}

// ---- JWT (HS256) ----

var b64 = base64.RawURLEncoding

func (a *App) issueToken(ws, am string, now time.Time) (string, int64) {
	exp := now.Add(tokenTTL).Unix()
	head := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"iss": tokenIssuer, "sub": ws, "am": am, "iat": now.Unix(), "exp": exp})
	signing := head + "." + b64.EncodeToString(payload)
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(signing))
	return signing + "." + b64.EncodeToString(mac.Sum(nil)), exp
}

type claims struct {
	Iss string  `json:"iss"`
	Sub string  `json:"sub"`
	Am  string  `json:"am"`
	Iat float64 `json:"iat"`
	Exp float64 `json:"exp"`
}

var errExpired = errors.New("expired")

func (a *App) verifyToken(tok string, now time.Time) (*claims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed")
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var head struct {
		Alg string `json:"alg"`
	}
	if json.Unmarshal(hb, &head) != nil || head.Alg != "HS256" {
		return nil, errors.New("alg")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, errors.New("signature")
	}
	pb, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c claims
	if err := json.Unmarshal(pb, &c); err != nil || c.Iss != tokenIssuer || c.Sub == "" {
		return nil, errors.New("claims")
	}
	if float64(now.Unix()) >= c.Exp {
		return &c, errExpired
	}
	return &c, nil
}

func (a *App) authenticate(ctx context.Context, r *http.Request) (*principal, *apiError) {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") || strings.TrimSpace(h[7:]) == "" {
		return nil, errf(401, "unauthorized", "missing bearer token")
	}
	now := time.Now()
	c, err := a.verifyToken(strings.TrimSpace(h[7:]), now)
	if errors.Is(err, errExpired) {
		return nil, errf(401, "unauthorized", "token expired")
	}
	if err != nil {
		return nil, errf(401, "unauthorized", "invalid token")
	}
	p := &principal{WS: c.Sub, AM: c.Am, Exp: int64(c.Exp)}
	if err := a.db.QueryRowContext(ctx, `SELECT auth_method, created_at FROM workspaces WHERE id = ?`, c.Sub).Scan(&p.AM, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errf(401, "unauthorized", "workspace expired")
		}
		return nil, dbErr(err)
	}
	a.touch(ctx, p.WS, now)
	return p, nil
}

// touch updates last_seen_at at most once a minute per workspace (per process).
func (a *App) touch(ctx context.Context, ws string, now time.Time) {
	a.touchMu.Lock()
	last, ok := a.touched[ws]
	if ok && now.Sub(last) < time.Minute {
		a.touchMu.Unlock()
		return
	}
	a.touched[ws] = now
	if len(a.touched) > 10000 {
		a.touched = map[string]time.Time{ws: now}
	}
	a.touchMu.Unlock()
	_, _ = a.db.ExecContext(ctx, `UPDATE workspaces SET last_seen_at = ? WHERE id = ?`, ts(now), ws)
}

func workspaceJSON(id, am, created string) map[string]any {
	return map[string]any{"id": id, "auth_method": am, "created_at": created}
}

func (a *App) tokenResponse(ws, am, created string, now time.Time) map[string]any {
	tok, _ := a.issueToken(ws, am, now)
	return map[string]any{
		"access_token": tok, "token_type": "Bearer", "expires_in": int(tokenTTL.Seconds()),
		"workspace": workspaceJSON(ws, am, created),
	}
}

func (a *App) limitIP(r *http.Request) *apiError {
	if wait := a.sandbox.take(clientIP(r), time.Now()); wait > 0 {
		e := errf(429, "rate_limited", "too many requests")
		e.RetryAfter = wait
		return e
	}
	return nil
}

func (a *App) createSandbox(ctx context.Context, q *req) (result, *apiError) {
	if e := a.limitIP(q.r); e != nil {
		return result{}, e
	}
	now := time.Now().UTC()
	ws := newID("ws_")
	if err := a.createWorkspace(ctx, ws, "sandbox", nil, now); err != nil {
		return result{}, dbErr(err)
	}
	return result{201, a.tokenResponse(ws, "sandbox", ts(now), now)}, nil
}

func (a *App) createWorkspace(ctx context.Context, ws, am string, owner *string, now time.Time) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, auth_method, owner_ref, order_seq, created_at, last_seen_at) VALUES (?, ?, ?, 0, ?, ?)`,
		ws, am, owner, ts(now), ts(now)); err != nil {
		return err
	}
	if err := seedWorkspace(ctx, tx, ws, a.cfg.Edition, now); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- sign-in through sedlab accounts (Russian edition) ----

func newAccounts() (*accounts.Client, error) {
	return accounts.FromEnv(kit.Env{Prefix: "ACCOUNTS_"})
}

func (a *App) exchangeAccounts(ctx context.Context, q *req) (result, *apiError) {
	if a.acc == nil {
		return result{}, errf(404, "feature_disabled", "sign-in via accounts is not available in this edition")
	}
	if e := a.limitIP(q.r); e != nil {
		return result{}, e
	}
	tok := ""
	if len(strings.TrimSpace(string(q.body))) > 0 {
		obj, e := parseObject(q.body)
		if e != nil {
			return result{}, e
		}
		v := newValidator(obj, false)
		v.str("accounts_token", false, false, 1, 4096)
		v.unknown()
		if len(v.details) > 0 {
			return result{}, validationErr(v.details)
		}
		if s, ok := v.out["accounts_token"].(string); ok {
			tok = s
		}
	}
	if tok == "" {
		if c, err := q.r.Cookie("sl_auth"); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		return result{}, errf(401, "unauthorized", "missing accounts session")
	}
	id, err := a.acc.VerifyToken(ctx, accounts.VerifyTokenInput{Token: tok})
	if err != nil {
		return result{}, errf(401, "unauthorized", "invalid accounts session")
	}
	owner := "acc:" + id.UserID
	now := time.Now().UTC()
	var ws, created string
	err = a.db.QueryRowContext(ctx, `SELECT id, created_at FROM workspaces WHERE owner_ref = ?`, owner).Scan(&ws, &created)
	if errors.Is(err, sql.ErrNoRows) {
		ws, created = newID("ws_"), ts(now)
		if err := a.createWorkspace(ctx, ws, "accounts", &owner, now); err != nil {
			// a concurrent exchange (maybe from the other backend) created it first
			if err2 := a.db.QueryRowContext(ctx, `SELECT id, created_at FROM workspaces WHERE owner_ref = ?`, owner).Scan(&ws, &created); err2 != nil {
				return result{}, dbErr(err)
			}
		}
	} else if err != nil {
		return result{}, dbErr(err)
	}
	go func() { // best effort: auth records that the user visited this demo
		vctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = a.acc.Visit(vctx, accounts.VisitInput{Token: tok})
	}()
	return result{200, a.tokenResponse(ws, "accounts", created, now)}, nil
}

// ---- me ----

const (
	quotaCustomers = 100
	quotaOrders    = 500
	quotaJobs      = 300
)

func (a *App) counts(ctx context.Context, db queryer, ws string) (map[string]int, error) {
	out := map[string]int{}
	for _, t := range []string{"customers", "orders", "jobs"} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+t+` WHERE workspace_id = ?`, ws).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}

func (a *App) me(ctx context.Context, q *req) (result, *apiError) {
	usage, err := a.counts(ctx, a.db, q.p.WS)
	if err != nil {
		return result{}, dbErr(err)
	}
	return result{200, map[string]any{
		"workspace":        workspaceJSON(q.p.WS, q.p.AM, q.p.CreatedAt),
		"backend":          "go",
		"edition":          a.cfg.Edition,
		"token_expires_at": ts(time.Unix(q.p.Exp, 0)),
		"quotas":           map[string]int{"customers": quotaCustomers, "orders": quotaOrders, "jobs": quotaJobs},
		"usage":            usage,
	}}, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ---- rate limiters (per process) ----

type bucket struct {
	tokens float64
	at     time.Time
}

type bucketLimiter struct {
	mu      sync.Mutex
	perMin  float64
	buckets map[string]*bucket
}

func newBucketLimiter(perMin int) *bucketLimiter {
	return &bucketLimiter{perMin: float64(perMin), buckets: map[string]*bucket{}}
}

// take returns 0 when allowed, otherwise seconds to wait.
func (l *bucketLimiter) take(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) > 20000 {
			l.buckets = map[string]*bucket{}
		}
		b = &bucket{tokens: l.perMin, at: now}
		l.buckets[key] = b
	}
	rate := l.perMin / 60
	b.tokens = math.Min(l.perMin, b.tokens+now.Sub(b.at).Seconds()*rate)
	b.at = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	return int(math.Ceil((1 - b.tokens) / rate))
}

type windowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{limit: limit, window: window, hits: map[string][]time.Time{}}
}

func (l *windowLimiter) take(key string, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 20000 {
		l.hits = map[string][]time.Time{}
	}
	h := l.hits[key]
	i := 0
	for i < len(h) && now.Sub(h[i]) >= l.window {
		i++
	}
	h = h[i:]
	if len(h) >= l.limit {
		l.hits[key] = h
		return int(math.Ceil(l.window.Seconds() - now.Sub(h[0]).Seconds()))
	}
	l.hits[key] = append(h, now)
	return 0
}

// ---- idempotency ----

func (a *App) serveIdempotent(w http.ResponseWriter, q *req, path string, h handlerFunc) {
	ctx := q.r.Context()
	key := q.r.Header.Get("Idempotency-Key")
	if key == "" {
		res, e := h(ctx, q)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, res.status, res.value)
		return
	}
	if n := len([]rune(key)); n > 100 {
		writeErr(w, validationErr([]detail{{"Idempotency-Key", "too_long"}}))
		return
	}
	sum := sha256.Sum256(q.body)
	hash := hex.EncodeToString(sum[:])
	route := "POST " + path
	var sRoute, sHash, sResp string
	var sStatus int
	err := a.db.QueryRowContext(ctx, `SELECT route, body_hash, status, response FROM idempotency_keys WHERE workspace_id = ? AND key = ?`,
		q.p.WS, key).Scan(&sRoute, &sHash, &sStatus, &sResp)
	if err == nil {
		if sRoute != route || sHash != hash {
			writeErr(w, errf(409, "idempotency_conflict", "idempotency key reused with a different request"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Idempotent-Replayed", "true")
		w.WriteHeader(sStatus)
		_, _ = w.Write([]byte(sResp))
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, dbErr(err))
		return
	}
	res, e := h(ctx, q)
	if e != nil {
		writeErr(w, e)
		return
	}
	b, _ := json.Marshal(res.value)
	_, _ = a.db.ExecContext(ctx, `INSERT INTO idempotency_keys (workspace_id, key, route, body_hash, status, response, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (workspace_id, key) DO NOTHING`,
		q.p.WS, key, route, hash, res.status, string(b), ts(time.Now()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.status)
	_, _ = w.Write(b)
}
