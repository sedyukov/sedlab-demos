package main

// Persistence = the accounts module's Store (accounts/go/store: SQLStore over modernc SQLite, portable SQL,
// shared migrations, storetest). This file only adapts it to the service's flows (consent-first sign-up,
// leads, limits decisions).

import (
	"context"
	"database/sql"
	"errors"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	akit "github.com/sedyukov/sedlab-modules/modules/accounts/go/kit"
	"github.com/sedyukov/sedlab-modules/modules/accounts/go/store"
)

type (
	User    = store.User
	Session = store.Session
	Visit   = store.Visit
)

var errNotFound = errors.New("not found")

func mapErr(err error) error {
	var ke *akit.Error
	if errors.As(err, &ke) && (ke.Code == akit.NotFound || ke.Code == akit.Unauthenticated || ke.Code == akit.Conflict) {
		return errNotFound
	}
	return err
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
	Contact bool   // optional "you may message me" (contact-v1)
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

type Store struct {
	st store.Store
	db *sql.DB
}

func NewStore(db *sql.DB, dialect string) *Store {
	return &Store{st: &store.SQLStore{DB: db, Dialect: dialect}, db: db}
}

func (s *Store) Migrate(ctx context.Context) error { return store.Migrate(ctx, s.db, "sqlite") }

// FindIdentity: is there already an account for this provider subject? (no row is created)
// TODO(accounts module): replace the direct read with a Store method when s2 adds it.
func (s *Store) FindIdentity(ctx context.Context, provider, uid string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM identities WHERE provider = ? AND provider_uid = ?`, provider, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	return id, err
}

// UpsertUser: c == nil → refresh the profile of an existing account; otherwise create the account with consents
// and its lead (created=true for a new person).
func (s *Store) UpsertUser(ctx context.Context, p Profile, c *Consent, firstDemo string, now int64) (*User, bool, error) {
	if c == nil {
		if _, err := s.FindIdentity(ctx, p.Provider, p.ProviderUID); err != nil {
			return nil, false, errors.New("consent required")
		}
	} else if c.Demo == "" || c.Terms == "" {
		return nil, false, errors.New("consent required")
	}
	u, created, err := s.st.UpsertIdentity(ctx, p.Provider, p.ProviderUID, store.Profile{DisplayName: p.DisplayName, Username: p.Username, Lang: p.Lang}, now)
	if err != nil {
		return nil, false, err
	}
	if c != nil {
		// TODO(accounts module): store the PD consent version (c.Demo) separately from Terms — asked s2.
		if err := s.st.RecordConsents(ctx, u.ID, store.Consents{Terms: c.Terms, Contact: c.Contact, Demo: firstDemo}, now); err != nil {
			return nil, false, err
		}
		if created {
			if _, err := s.st.InsertLead(ctx, u.ID, p.Provider, firstDemo, now); err != nil {
				return nil, false, err
			}
		}
		u, err = s.st.GetUser(ctx, u.ID)
	}
	return u, created, err
}

func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	u, err := s.st.GetUser(ctx, id)
	return u, mapErr(err)
}

func (s *Store) SetContact(ctx context.Context, id string, ok bool, now int64) error {
	return s.st.SetContact(ctx, id, ok, now)
}
func (s *Store) DeleteUser(ctx context.Context, id string) error { return s.st.DeleteUser(ctx, id) }
func (s *Store) PurgeInactive(ctx context.Context, before int64) (int, error) {
	return s.st.PurgeInactive(ctx, before)
}
func (s *Store) Touch(ctx context.Context, u *User, now int64) {
	if u.LastSeenAt < now-300 {
		_ = s.st.TouchUser(ctx, u.ID, now)
	}
}
func (s *Store) Snapshot(ctx context.Context, path string) error { return s.st.Snapshot(ctx, path) }

// ---- sessions / nonces ------------------------------------------------------------------------------------

func (s *Store) CreateSession(ctx context.Context, userID, provider string, ttl, now int64) (string, error) {
	x, err := s.st.CreateSession(ctx, userID, provider, ttl, now)
	if err != nil {
		return "", err
	}
	return x.SID, nil
}

func (s *Store) GetSession(ctx context.Context, sid string) (*Session, error) {
	x, err := s.st.GetSession(ctx, sid)
	return x, mapErr(err)
}

func (s *Store) RevokeSession(ctx context.Context, sid string, now int64) error {
	return s.st.RevokeSession(ctx, sid, now)
}

func (s *Store) CreateNonce(ctx context.Context, provider, browserHash string, ttl, now int64) (string, error) {
	n, err := s.st.CreateLoginNonce(ctx, provider, browserHash, ttl, now)
	if err != nil {
		return "", err
	}
	return n.Nonce, nil
}

// ConfirmNonce: the bot's "Confirm" button — binds the provider subject; no user row is created.
func (s *Store) ConfirmNonce(ctx context.Context, nonce string, sub store.Subject, now int64) (bool, error) {
	if _, err := s.st.ConfirmLoginNonce(ctx, nonce, sub, now); err != nil {
		var ke *akit.Error
		if mapErr(err) == errNotFound || (errors.As(err, &ke) && ke.Code == akit.InvalidArgument) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// UseNonce: poll from the browser that started the login. Status pending → keep polling; used → Subject is set.
func (s *Store) UseNonce(ctx context.Context, nonce, browserHash string, now int64) (*store.LoginNonce, error) {
	x, err := s.st.ConsumeLoginNonce(ctx, nonce, browserHash, now)
	return x, mapErr(err)
}

func (s *Store) PurgeNonces(ctx context.Context, now int64) {
	_, _ = s.st.PurgeLoginNonces(ctx, now-3600)
}

func (s *Store) RecordVisit(ctx context.Context, userID, demo string, now int64) error {
	return s.st.RecordVisit(ctx, userID, demo, now)
}

func (s *Store) Visits(ctx context.Context, userID string) ([]Visit, error) {
	ex, err := s.st.ExportUser(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	return ex.Visits, nil
}

func (s *Store) ExportUser(ctx context.Context, userID string) (*store.UserExport, error) {
	ex, err := s.st.ExportUser(ctx, userID)
	return ex, mapErr(err)
}

// ---- limits -----------------------------------------------------------------------------------------------

func toDecision(r *store.Result, lim accounts.ResourceLimit, now int64) *Decision {
	d := &Decision{Allowed: r.Allowed, Reason: r.Reason, UserLeft: r.UserLeft, UserLimit: lim.PerUserDay, BudgetLeft: r.BudgetLeft,
		ResetsAt: nextDay(now), ConsumptionID: r.ConsumptionID}
	if d.Reason == "" {
		d.Reason = "ok"
	}
	return d
}

func consumeReq(userID, demo, resource string, units int64, idem string, lim accounts.ResourceLimit, now int64) store.ConsumeRequest {
	return store.ConsumeRequest{UserID: userID, Demo: demo, Resource: resource, Units: units, IdempotencyKey: idem, Limits: lim.StoreLimits(), Now: now}
}

func (s *Store) Check(ctx context.Context, userID, demo, resource string, units int64, lim accounts.ResourceLimit, now int64) (*Decision, error) {
	r, err := s.st.Check(ctx, consumeReq(userID, demo, resource, units, "", lim, now))
	if err != nil {
		return nil, err
	}
	return toDecision(r, lim, now), nil
}

func (s *Store) Consume(ctx context.Context, userID, demo, resource string, units int64, idem string, lim accounts.ResourceLimit, now int64) (*Decision, error) {
	r, err := s.st.Consume(ctx, consumeReq(userID, demo, resource, units, idem, lim, now))
	if err != nil {
		return nil, err
	}
	return toDecision(r, lim, now), nil
}

// Refund: only the demo that consumed may refund (the module refunds by id; the demo check is ours).
func (s *Store) Refund(ctx context.Context, cid, demo string, now int64) error {
	var d string
	if err := s.db.QueryRowContext(ctx, `SELECT demo FROM consumptions WHERE id = ?`, cid).Scan(&d); err != nil || d != demo {
		return errNotFound
	}
	return mapErr(s.st.Refund(ctx, cid, now))
}

// ---- leads ------------------------------------------------------------------------------------------------

func (s *Store) enrich(ctx context.Context, l store.Lead) Lead {
	out := Lead{ID: l.ID, UserID: l.UserID, Provider: l.Provider, FirstDemo: l.FirstDemo, CreatedAt: l.CreatedAt, NotifiedAt: l.NotifiedAt}
	if ex, err := s.st.ExportUser(ctx, l.UserID); err == nil {
		out.DisplayName, out.Username, out.ContactOK, out.Demos = ex.User.DisplayName, ex.User.Username, ex.User.ContactOK, ex.Visits
		for _, c := range ex.Consumptions {
			if c.Status == store.StatusOK {
				out.LiveUsed += c.Units
			}
		}
	}
	return out
}

func (s *Store) Leads(ctx context.Context, since int64) ([]Lead, error) {
	ls, err := s.st.ListLeads(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]Lead, 0, len(ls))
	for _, l := range ls {
		out = append(out, s.enrich(ctx, l))
	}
	return out, nil
}

func (s *Store) LeadByUser(ctx context.Context, userID string) (*Lead, error) {
	ex, err := s.st.ExportUser(ctx, userID)
	if err != nil || len(ex.Leads) == 0 {
		return nil, errNotFound
	}
	l := s.enrich(ctx, ex.Leads[0])
	return &l, nil
}

func (s *Store) UnnotifiedLeads(ctx context.Context) ([]Lead, error) {
	ls, err := s.st.ListLeads(ctx, 0)
	if err != nil {
		return nil, err
	}
	var out []Lead
	for _, l := range ls {
		if l.NotifiedAt == 0 {
			out = append(out, s.enrich(ctx, l))
		}
	}
	return out, nil
}

func (s *Store) MarkLeadNotified(ctx context.Context, id string, now int64) error {
	return s.st.MarkLeadNotified(ctx, id, now)
}
