package main

// Store: the only way the service touches the database. Portable SQL (SQLite now, Postgres as a second driver):
// `?` placeholders are rebound per dialect, epoch seconds UTC, ON CONFLICT … DO, no SQLite-only features.
// TODO(accounts module): s2 moves Store + storetest into the private module; this file then becomes the driver wiring.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var errNotFound = errors.New("not found")

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Lang        string `json:"lang"`
	ConsentVer  string `json:"consent_version"`
	ConsentAt   int64  `json:"consent_at"`
	TermsVer    string `json:"terms_version"`
	TermsAt     int64  `json:"terms_at"`
	ContactOK   bool   `json:"contact_ok"`
	ContactAt   int64  `json:"contact_consent_at"`
	CreatedAt   int64  `json:"created_at"`
	LastSeenAt  int64  `json:"last_seen_at"`
}

type Profile struct {
	Provider    string
	ProviderUID string
	DisplayName string
	Username    string
	Lang        string
}

// Consent: what the person ticked on the consent screen (separate checkboxes, 152-ФЗ ч.1 ст.9 в ред. 156-ФЗ).
type Consent struct {
	Demo    string // PD consent version, e.g. demo-v1 (required)
	Terms   string // Demo Terms version, e.g. terms-v1 (required)
	Contact string // contact-v1 when the optional box was ticked, else ""
}

type Session struct {
	Sid       string
	UserID    string
	Provider  string
	CreatedAt int64
	ExpiresAt int64
	RevokedAt int64
}

type LoginNonce struct {
	Nonce, Provider, BrowserHash, Status string
	ProviderUID, DisplayName, Username   string
	ExpiresAt                            int64
}

type Limit struct {
	PerUserDay   int64 // <0 unlimited, 0 disabled
	PerDayGlobal int64 // <0 unlimited
	Monthly      int64 // <0 unlimited
}

type Decision struct {
	Allowed       bool   `json:"allowed"`
	Reason        string `json:"reason"`
	UserLeft      int64  `json:"user_left"`
	UserLimit     int64  `json:"user_limit"`
	BudgetLeft    int64  `json:"budget_left"`
	ResetsAt      int64  `json:"resets_at"`
	ConsumptionID string `json:"consumption_id,omitempty"`
}

type Lead struct {
	ID          string  `json:"lead_id"`
	UserID      string  `json:"user_id"`
	Provider    string  `json:"provider"`
	FirstDemo   string  `json:"first_demo"`
	CreatedAt   int64   `json:"created_at"`
	NotifiedAt  int64   `json:"notified_at"`
	DisplayName string  `json:"display_name"`
	Username    string  `json:"username"`
	ContactOK   bool    `json:"contact_ok"`
	Demos       []Visit `json:"demos"`
	LiveUsed    int64   `json:"live_used"`
}

type Visit struct {
	Demo    string `json:"demo"`
	FirstAt int64  `json:"first_at"`
	LastAt  int64  `json:"last_at"`
	Visits  int64  `json:"count"`
}

type Store struct {
	db      *sql.DB
	dialect string // sqlite | postgres
}

func NewStore(db *sql.DB, dialect string) *Store { return &Store{db: db, dialect: dialect} }

// rebind turns `?` into `$n` for Postgres.
func (s *Store) rebind(q string) string {
	if s.dialect != "postgres" {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func (s *Store) exec(ctx context.Context, e execer, q string, a ...any) (int64, error) {
	r, err := e.ExecContext(ctx, s.rebind(q), a...)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

func (s *Store) row(ctx context.Context, e execer, q string, a ...any) *sql.Row {
	return e.QueryRowContext(ctx, s.rebind(q), a...)
}

func (s *Store) tx(ctx context.Context, f func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Migrate applies migrations/NNNN_name.sql (or NNNN_name.<dialect>.sql when present) in order.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at BIGINT NOT NULL)`); err != nil {
		return err
	}
	ents, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	byVer := map[string]string{}
	for _, e := range ents {
		n := e.Name()
		ver := strings.SplitN(n, "_", 2)[0]
		parts := strings.Split(n, ".")
		switch {
		case len(parts) == 3 && parts[1] == s.dialect:
			byVer[ver] = n // dialect-specific wins
		case len(parts) == 2:
			if _, ok := byVer[ver]; !ok {
				byVer[ver] = n
			}
		}
	}
	vers := make([]string, 0, len(byVer))
	for v := range byVer {
		vers = append(vers, v)
	}
	sort.Strings(vers)
	for _, v := range vers {
		var x string
		err := s.row(ctx, s.db, `SELECT version FROM schema_migrations WHERE version = ?`, v).Scan(&x)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		body, _ := migrationsFS.ReadFile("migrations/" + byVer[v])
		if err := s.tx(ctx, func(tx *sql.Tx) error {
			for _, stmt := range strings.Split(string(body), ";") {
				if strings.TrimSpace(stripComments(stmt)) == "" {
					continue
				}
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("migration %s: %w", byVer[v], err)
				}
			}
			_, err := s.exec(ctx, tx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, v, time.Now().Unix())
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func stripComments(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// ---- ids --------------------------------------------------------------------------------------------------

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newULID: 48-bit ms timestamp + 80 random bits, Crockford base32 (sortable).
func newULID(now time.Time) string {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	_, _ = rand.Read(b[6:])
	var out [26]byte
	// 128 bits → 26 chars (first char carries 3 bits)
	var acc uint64
	bits := 0
	j := 25
	for i := 15; i >= 0; i-- {
		acc |= uint64(b[i]) << bits
		bits += 8
		for bits >= 5 && j >= 0 {
			out[j] = crockford[acc&31]
			acc >>= 5
			bits -= 5
			j--
		}
	}
	if j >= 0 {
		out[j] = crockford[acc&31]
	}
	return string(out[:])
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b64.EncodeToString(b)
}

// ---- users / identities -----------------------------------------------------------------------------------

func (s *Store) FindIdentity(ctx context.Context, provider, uid string) (string, error) {
	var id string
	err := s.row(ctx, s.db, `SELECT user_id FROM identities WHERE provider = ? AND provider_uid = ?`, provider, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	return id, err
}

// UpsertUser: existing identity → refresh profile; otherwise create user + identity + lead (created=true).
func (s *Store) UpsertUser(ctx context.Context, p Profile, c *Consent, firstDemo string, now int64) (u *User, created bool, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var id string
		e := s.row(ctx, tx, `SELECT user_id FROM identities WHERE provider = ? AND provider_uid = ?`, p.Provider, p.ProviderUID).Scan(&id)
		switch {
		case e == nil:
			_, e = s.exec(ctx, tx, `UPDATE users SET display_name = ?, username = ?, last_seen_at = ? WHERE id = ?`, p.DisplayName, p.Username, now, id)
			return e
		case !errors.Is(e, sql.ErrNoRows):
			return e
		}
		if c == nil || c.Demo == "" || c.Terms == "" {
			return errors.New("consent required")
		}
		id = newULID(time.Unix(now, 0))
		created = true
		contactOK, contactAt := 0, int64(0)
		if c.Contact != "" {
			contactOK, contactAt = 1, now
		}
		if _, e = s.exec(ctx, tx, `INSERT INTO users (id, display_name, username, lang, consent_version, consent_at, terms_version, terms_at, contact_ok, contact_consent_at, created_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, p.DisplayName, p.Username, p.Lang, c.Demo, now, c.Terms, now, contactOK, contactAt, now, now); e != nil {
			return e
		}
		if _, e = s.exec(ctx, tx, `INSERT INTO identities (provider, provider_uid, user_id, created_at) VALUES (?, ?, ?, ?)`, p.Provider, p.ProviderUID, id, now); e != nil {
			return e
		}
		_, e = s.exec(ctx, tx, `INSERT INTO leads (id, user_id, provider, first_demo, created_at) VALUES (?, ?, ?, ?, ?)`, newULID(time.Unix(now, 0)), id, p.Provider, firstDemo, now)
		return e
	})
	if err != nil {
		return nil, false, err
	}
	id, err := s.FindIdentity(ctx, p.Provider, p.ProviderUID)
	if err != nil {
		return nil, false, err
	}
	u, err = s.GetUser(ctx, id)
	return u, created, err
}

func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	var contact int64
	err := s.row(ctx, s.db, `SELECT id, display_name, username, lang, consent_version, consent_at, terms_version, terms_at, contact_ok, contact_consent_at, created_at, last_seen_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.DisplayName, &u.Username, &u.Lang, &u.ConsentVer, &u.ConsentAt, &u.TermsVer, &u.TermsAt, &contact, &u.ContactAt, &u.CreatedAt, &u.LastSeenAt)
	u.ContactOK = contact == 1
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return &u, err
}

// DeleteUser removes everything personal. Global counters (usage_global) stay: they hold no user id.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, t := range []string{"consumptions", "usage_daily", "demo_visits", "leads", "sessions", "identities"} {
			if _, err := s.exec(ctx, tx, `DELETE FROM `+t+` WHERE user_id = ?`, id); err != nil {
				return err
			}
		}
		_, err := s.exec(ctx, tx, `DELETE FROM users WHERE id = ?`, id)
		return err
	})
}

// PurgeInactive deletes users not seen since `before`; returns their count.
func (s *Store) PurgeInactive(ctx context.Context, before int64) (int, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`SELECT id FROM users WHERE last_seen_at < ?`), before)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.DeleteUser(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// SetContact: optional "you may message me" consent (contact-v1); withdrawal is immediate.
func (s *Store) SetContact(ctx context.Context, userID string, ok bool, now int64) error {
	v := 0
	if ok {
		v = 1
	}
	_, err := s.exec(ctx, s.db, `UPDATE users SET contact_ok = ?, contact_consent_at = ? WHERE id = ?`, v, now, userID)
	return err
}

func (s *Store) Touch(ctx context.Context, userID string, now int64) {
	_, _ = s.exec(ctx, s.db, `UPDATE users SET last_seen_at = ? WHERE id = ? AND last_seen_at < ?`, now, userID, now-300)
}

// ---- sessions ---------------------------------------------------------------------------------------------

func (s *Store) CreateSession(ctx context.Context, userID, provider string, now, exp int64) (string, error) {
	sid := randToken(16)
	_, err := s.exec(ctx, s.db, `INSERT INTO sessions (sid, user_id, provider, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`, sid, userID, provider, now, exp)
	return sid, err
}

func (s *Store) GetSession(ctx context.Context, sid string) (*Session, error) {
	var x Session
	err := s.row(ctx, s.db, `SELECT sid, user_id, provider, created_at, expires_at, revoked_at FROM sessions WHERE sid = ?`, sid).
		Scan(&x.Sid, &x.UserID, &x.Provider, &x.CreatedAt, &x.ExpiresAt, &x.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return &x, err
}

func (s *Store) RevokeSession(ctx context.Context, sid string, now int64) error {
	_, err := s.exec(ctx, s.db, `UPDATE sessions SET revoked_at = ? WHERE sid = ? AND revoked_at = 0`, now, sid)
	return err
}

// ---- login nonces -----------------------------------------------------------------------------------------

func (s *Store) CreateNonce(ctx context.Context, provider, browserHash string, exp int64) (string, error) {
	n := randToken(16)
	_, err := s.exec(ctx, s.db, `INSERT INTO login_nonces (nonce, provider, browser_hash, status, expires_at) VALUES (?, ?, ?, 'pending', ?)`, n, provider, browserHash, exp)
	return n, err
}

func (s *Store) GetNonce(ctx context.Context, nonce string) (*LoginNonce, error) {
	var x LoginNonce
	err := s.row(ctx, s.db, `SELECT nonce, provider, browser_hash, status, provider_uid, display_name, username, expires_at FROM login_nonces WHERE nonce = ?`, nonce).
		Scan(&x.Nonce, &x.Provider, &x.BrowserHash, &x.Status, &x.ProviderUID, &x.DisplayName, &x.Username, &x.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return &x, err
}

// ConfirmNonce: bot side — the user pressed "Confirm" in the messenger.
func (s *Store) ConfirmNonce(ctx context.Context, nonce, provider, uid, name, username string, now int64) (bool, error) {
	n, err := s.exec(ctx, s.db, `UPDATE login_nonces SET status = 'confirmed', provider_uid = ?, display_name = ?, username = ? WHERE nonce = ? AND provider = ? AND status = 'pending' AND expires_at > ?`,
		uid, name, username, nonce, provider, now)
	return n == 1, err
}

// UseNonce: browser side — only the browser that started the login, once.
func (s *Store) UseNonce(ctx context.Context, nonce, browserHash string, now int64) (*LoginNonce, error) {
	x, err := s.GetNonce(ctx, nonce)
	if err != nil {
		return nil, err
	}
	if x.BrowserHash != browserHash || x.ExpiresAt <= now {
		return nil, errNotFound
	}
	if x.Status != "confirmed" {
		return x, nil // still pending: caller keeps polling
	}
	n, err := s.exec(ctx, s.db, `UPDATE login_nonces SET status = 'used' WHERE nonce = ? AND status = 'confirmed'`, nonce)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, errNotFound
	}
	x.Status = "used"
	return x, nil
}

func (s *Store) PurgeNonces(ctx context.Context, now int64) {
	_, _ = s.exec(ctx, s.db, `DELETE FROM login_nonces WHERE expires_at < ?`, now-3600)
}

// ---- visits -----------------------------------------------------------------------------------------------

func (s *Store) RecordVisit(ctx context.Context, userID, demo string, now int64) error {
	_, err := s.exec(ctx, s.db, `INSERT INTO demo_visits (user_id, demo, first_at, last_at, visits) VALUES (?, ?, ?, ?, 1)
		ON CONFLICT (user_id, demo) DO UPDATE SET last_at = excluded.last_at, visits = demo_visits.visits + 1`, userID, demo, now, now)
	return err
}

func (s *Store) Visits(ctx context.Context, userID string) ([]Visit, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`SELECT demo, first_at, last_at, visits FROM demo_visits WHERE user_id = ? ORDER BY first_at`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Visit
	for rows.Next() {
		var v Visit
		if err := rows.Scan(&v.Demo, &v.FirstAt, &v.LastAt, &v.Visits); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- limits -----------------------------------------------------------------------------------------------

func dayOf(now int64) string   { return time.Unix(now, 0).UTC().Format("2006-01-02") }
func monthOf(now int64) string { return time.Unix(now, 0).UTC().Format("2006-01") }
func nextDay(now int64) int64 {
	t := time.Unix(now, 0).UTC()
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC).Unix()
}

func (s *Store) used(ctx context.Context, e execer, q string, a ...any) (int64, error) {
	var n int64
	err := s.row(ctx, e, q, a...).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

func left(limit, used int64) int64 {
	if limit < 0 {
		return -1
	}
	if used >= limit {
		return 0
	}
	return limit - used
}

// decide computes the decision for `units` more without writing.
func (s *Store) decide(ctx context.Context, e execer, userID, demo, resource string, units int64, l Limit, now int64) (*Decision, error) {
	day, month := dayOf(now), monthOf(now)
	uu, err := s.used(ctx, e, `SELECT used FROM usage_daily WHERE user_id = ? AND demo = ? AND resource = ? AND day = ?`, userID, demo, resource, day)
	if err != nil {
		return nil, err
	}
	gd, err := s.used(ctx, e, `SELECT used FROM usage_global WHERE resource = ? AND period = ?`, resource, day)
	if err != nil {
		return nil, err
	}
	gm, err := s.used(ctx, e, `SELECT used FROM usage_global WHERE resource = ? AND period = ?`, resource, month)
	if err != nil {
		return nil, err
	}
	d := &Decision{UserLimit: l.PerUserDay, UserLeft: left(l.PerUserDay, uu), BudgetLeft: left(l.Monthly, gm), ResetsAt: nextDay(now)}
	switch {
	case l.PerUserDay == 0:
		d.Reason = "disabled"
	case l.PerUserDay > 0 && uu+units > l.PerUserDay:
		d.Reason = "user_daily"
	case l.PerDayGlobal >= 0 && gd+units > l.PerDayGlobal:
		d.Reason = "global_daily"
	case l.Monthly >= 0 && gm+units > l.Monthly:
		d.Reason = "budget_month"
	default:
		d.Allowed, d.Reason = true, "ok"
	}
	return d, nil
}

func (s *Store) Check(ctx context.Context, userID, demo, resource string, units int64, l Limit, now int64) (*Decision, error) {
	return s.decide(ctx, s.db, userID, demo, resource, units, l, now)
}

// Consume: check + reserve atomically (conditional UPDATEs in one transaction). Same idempotency key → same answer.
func (s *Store) Consume(ctx context.Context, userID, demo, resource string, units int64, idem string, l Limit, now int64) (*Decision, error) {
	var out *Decision
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var cid, cstatus string
		e := s.row(ctx, tx, `SELECT id, status FROM consumptions WHERE demo = ? AND idempotency_key = ? AND user_id = ?`, demo, idem, userID).Scan(&cid, &cstatus)
		if e == nil {
			d, err := s.decide(ctx, tx, userID, demo, resource, 0, l, now)
			if err != nil {
				return err
			}
			d.Allowed, d.Reason, d.ConsumptionID = cstatus == "ok", "ok", cid
			if cstatus != "ok" {
				d.Reason = "refunded"
			}
			out = d
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		d, err := s.decide(ctx, tx, userID, demo, resource, units, l, now)
		if err != nil {
			return err
		}
		if !d.Allowed {
			out = d
			return nil
		}
		day, month := dayOf(now), monthOf(now)
		steps := []struct {
			ins, upd string
			args     []any
			limit    int64
			reason   string
		}{
			{`INSERT INTO usage_daily (user_id, demo, resource, day, used) VALUES (?, ?, ?, ?, 0) ON CONFLICT (user_id, demo, resource, day) DO NOTHING`,
				`UPDATE usage_daily SET used = used + ? WHERE user_id = ? AND demo = ? AND resource = ? AND day = ? AND (? < 0 OR used + ? <= ?)`,
				[]any{userID, demo, resource, day}, l.PerUserDay, "user_daily"},
			{`INSERT INTO usage_global (resource, period, used) VALUES (?, ?, 0) ON CONFLICT (resource, period) DO NOTHING`,
				`UPDATE usage_global SET used = used + ? WHERE resource = ? AND period = ? AND (? < 0 OR used + ? <= ?)`,
				[]any{resource, day}, l.PerDayGlobal, "global_daily"},
			{`INSERT INTO usage_global (resource, period, used) VALUES (?, ?, 0) ON CONFLICT (resource, period) DO NOTHING`,
				`UPDATE usage_global SET used = used + ? WHERE resource = ? AND period = ? AND (? < 0 OR used + ? <= ?)`,
				[]any{resource, month}, l.Monthly, "budget_month"},
		}
		for _, st := range steps {
			if _, err := s.exec(ctx, tx, st.ins, st.args...); err != nil {
				return err
			}
			args := append([]any{units}, st.args...)
			args = append(args, st.limit, units, st.limit)
			n, err := s.exec(ctx, tx, st.upd, args...)
			if err != nil {
				return err
			}
			if n != 1 { // lost a race: deny, roll back the whole reservation
				out = &Decision{Reason: st.reason, UserLimit: l.PerUserDay, ResetsAt: nextDay(now)}
				return errDenied
			}
		}
		cid = newULID(time.Unix(now, 0))
		if _, err := s.exec(ctx, tx, `INSERT INTO consumptions (id, user_id, demo, resource, units, idempotency_key, status, day, created_at) VALUES (?, ?, ?, ?, ?, ?, 'ok', ?, ?)`,
			cid, userID, demo, resource, units, idem, day, now); err != nil {
			return err
		}
		d.ConsumptionID = cid
		d.UserLeft = left(l.PerUserDay, l.PerUserDay-d.UserLeft+units)
		if d.BudgetLeft >= 0 {
			d.BudgetLeft = max(0, d.BudgetLeft-units)
		}
		out = d
		return nil
	})
	if errors.Is(err, errDenied) {
		return out, nil
	}
	return out, err
}

var errDenied = errors.New("denied")

// Refund returns the units of an `ok` consumption (provider call failed). Second refund is a no-op.
func (s *Store) Refund(ctx context.Context, cid, demo string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var userID, resource, status, day string
		var units int64
		e := s.row(ctx, tx, `SELECT user_id, resource, units, status, day FROM consumptions WHERE id = ? AND demo = ?`, cid, demo).Scan(&userID, &resource, &units, &status, &day)
		if errors.Is(e, sql.ErrNoRows) {
			return errNotFound
		}
		if e != nil || status != "ok" {
			return e
		}
		if _, err := s.exec(ctx, tx, `UPDATE consumptions SET status = 'refunded' WHERE id = ?`, cid); err != nil {
			return err
		}
		if _, err := s.exec(ctx, tx, `UPDATE usage_daily SET used = used - ? WHERE user_id = ? AND demo = ? AND resource = ? AND day = ? AND used >= ?`, units, userID, demo, resource, day, units); err != nil {
			return err
		}
		for _, p := range []string{day, day[:7]} {
			if _, err := s.exec(ctx, tx, `UPDATE usage_global SET used = used - ? WHERE resource = ? AND period = ? AND used >= ?`, units, resource, p, units); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) LiveUsed(ctx context.Context, userID string) (int64, error) {
	return s.used(ctx, s.db, `SELECT COALESCE(SUM(units), 0) FROM consumptions WHERE user_id = ? AND status = 'ok'`, userID)
}

// ---- leads ------------------------------------------------------------------------------------------------

func (s *Store) Leads(ctx context.Context, since int64) ([]Lead, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`SELECT l.id, l.user_id, l.provider, l.first_demo, l.created_at, l.notified_at, u.display_name, u.username, u.contact_ok
		FROM leads l JOIN users u ON u.id = l.user_id WHERE l.created_at >= ? ORDER BY l.created_at`), since)
	if err != nil {
		return nil, err
	}
	var out []Lead
	for rows.Next() {
		var l Lead
		var contact int64
		if err := rows.Scan(&l.ID, &l.UserID, &l.Provider, &l.FirstDemo, &l.CreatedAt, &l.NotifiedAt, &l.DisplayName, &l.Username, &contact); err != nil {
			rows.Close()
			return nil, err
		}
		l.ContactOK = contact == 1
		out = append(out, l)
	}
	rows.Close()
	for i := range out {
		out[i].Demos, _ = s.Visits(ctx, out[i].UserID)
		out[i].LiveUsed, _ = s.LiveUsed(ctx, out[i].UserID)
	}
	return out, nil
}

func (s *Store) LeadByUser(ctx context.Context, userID string) (*Lead, error) {
	var l Lead
	err := s.row(ctx, s.db, `SELECT id, user_id, provider, first_demo, created_at, notified_at FROM leads WHERE user_id = ?`, userID).
		Scan(&l.ID, &l.UserID, &l.Provider, &l.FirstDemo, &l.CreatedAt, &l.NotifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return &l, err
}

func (s *Store) MarkLeadNotified(ctx context.Context, id string, now int64) error {
	_, err := s.exec(ctx, s.db, `UPDATE leads SET notified_at = ? WHERE id = ?`, now, id)
	return err
}

func (s *Store) UnnotifiedLeads(ctx context.Context) ([]Lead, error) {
	all, err := s.Leads(ctx, 0)
	if err != nil {
		return nil, err
	}
	var out []Lead
	for _, l := range all {
		if l.NotifiedAt == 0 {
			out = append(out, l)
		}
	}
	return out, nil
}

// ExportUser: everything stored about a person (GDPR access / «скачать мои данные»).
func (s *Store) ExportUser(ctx context.Context, id string) (map[string]any, error) {
	u, err := s.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	visits, _ := s.Visits(ctx, id)
	var ids []map[string]any
	rows, err := s.db.QueryContext(ctx, s.rebind(`SELECT provider, provider_uid, created_at FROM identities WHERE user_id = ?`), id)
	if err == nil {
		for rows.Next() {
			var p, uid string
			var at int64
			if rows.Scan(&p, &uid, &at) == nil {
				ids = append(ids, map[string]any{"provider": p, "provider_uid": uid, "created_at": at})
			}
		}
		rows.Close()
	}
	var usage []map[string]any
	rows, err = s.db.QueryContext(ctx, s.rebind(`SELECT demo, resource, day, used FROM usage_daily WHERE user_id = ? ORDER BY day`), id)
	if err == nil {
		for rows.Next() {
			var d, r, day string
			var n int64
			if rows.Scan(&d, &r, &day, &n) == nil {
				usage = append(usage, map[string]any{"demo": d, "resource": r, "day": day, "used": n})
			}
		}
		rows.Close()
	}
	return map[string]any{"user": u, "identities": ids, "demos": visits, "live_usage": usage}, nil
}
