package main

// Internal API for demos (module `accounts`, provider http). Caddy on the RU host allows /internal/* only from
// freelance-1's IP; every call also carries X-Service-Key (one key per demo → the demo name is taken from the key).

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

func (a *App) serviceDemo(r *http.Request) (string, bool) {
	// Defense in depth: Caddy already limits /internal/* to freelance-1; the app checks again when configured.
	if len(a.cfg.InternalIPs) > 0 && !a.cfg.InternalIPs[clientIP(r)] {
		return "", false
	}
	k := r.Header.Get("X-Service-Key")
	if k == "" {
		return "", false
	}
	for key, demo := range a.cfg.ServiceKeys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(k)) == 1 {
			return demo, true
		}
	}
	return "", false
}

type internalIn struct {
	Kind           string `json:"kind"`
	Version        string `json:"version"`
	Token          string `json:"token"`
	Demo           string `json:"demo"`
	Resource       string `json:"resource"`
	Units          int64  `json:"units"`
	IdempotencyKey string `json:"idempotency_key"`
	ConsumptionID  string `json:"consumption_id"`
}

func (a *App) internal(w http.ResponseWriter, r *http.Request) {
	demo, ok := a.serviceDemo(r)
	if !ok {
		moduleErr(w, 401, "config", "bad service key")
		return
	}
	var in internalIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
		moduleErr(w, 400, "invalid_argument", "bad json")
		return
	}
	if in.Demo != "" && in.Demo != demo {
		moduleErr(w, 403, "permission_denied", "service key is for another demo")
		return
	}
	op := r.PathValue("op")
	if op == "refund" {
		if in.ConsumptionID == "" {
			moduleErr(w, 400, "invalid_argument", "consumption_id required")
			return
		}
		if err := a.store.Refund(r.Context(), in.ConsumptionID, demo, a.now()); errors.Is(err, errNotFound) {
			moduleErr(w, 404, "not_found", "no such consumption")
			return
		} else if err != nil {
			moduleErr(w, 503, "unavailable", "storage")
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	u, _ := a.authUser(r.Context(), in.Token)
	switch op {
	case "visit":
		if u != nil {
			_ = a.store.RecordVisit(r.Context(), u.ID, demo, a.now())
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	case "remaining":
		if u == nil {
			writeJSON(w, 200, map[string]any{"authenticated": false, "resources": []any{}})
			return
		}
		writeJSON(w, 200, map[string]any{"authenticated": true, "resources": a.remaining(r, u.ID, demo)})
		return
	case "consent":
		a.internalConsent(w, r, demo, u, in)
		return
	case "check", "consume":
	default:
		moduleErr(w, 404, "not_found", "unknown op")
		return
	}
	if u == nil {
		writeJSON(w, 200, Decision{Reason: "unauthenticated"})
		return
	}
	res, ok := a.limits.For(demo, in.Resource)
	if !ok {
		writeJSON(w, 200, Decision{Reason: "disabled"})
		return
	}
	if in.Units <= 0 {
		in.Units = 1
	}
	if res.MaxUnits > 0 && in.Units > res.MaxUnits {
		moduleErr(w, 400, "invalid_argument", "units over max_units "+strconv.FormatInt(res.MaxUnits, 10))
		return
	}
	var d *Decision
	var err error
	if op == "check" {
		d, err = a.store.Check(r.Context(), u.ID, demo, in.Resource, in.Units, res, a.now())
	} else {
		if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 128 {
			moduleErr(w, 400, "invalid_argument", "idempotency_key required (≤128)")
			return
		}
		d, err = a.store.Consume(r.Context(), u.ID, demo, in.Resource, in.Units, in.IdempotencyKey, res, a.now())
		if err == nil && d.Allowed {
			_ = a.store.RecordVisit(r.Context(), u.ID, demo, a.now())
		}
	}
	if err != nil {
		moduleErr(w, 503, "unavailable", "storage")
		return
	}
	writeJSON(w, 200, d)
}

// leadsExport: JSONL for the bid pipeline. Needs AUTH_SVC_ADMIN key (not a demo key).
func (a *App) leadsExport(w http.ResponseWriter, r *http.Request) {
	if demo, ok := a.serviceDemo(r); !ok || demo != "admin" {
		moduleErr(w, 401, "config", "admin service key required")
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	leads, err := a.store.Leads(r.Context(), since)
	if err != nil {
		moduleErr(w, 503, "unavailable", "storage")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	enc := json.NewEncoder(w)
	for _, l := range leads {
		_ = enc.Encode(l)
	}
}

// adminLeads: owner-only HTML (signed in with an identity listed in AUTH_ADMINS).
func (a *App) adminLeads(w http.ResponseWriter, r *http.Request) {
	u, s := a.authUser(r.Context(), tokenFromRequest(r))
	if u == nil {
		http.Redirect(w, r, "/login?return="+r.URL.Path, http.StatusSeeOther)
		return
	}
	if !a.isAdmin(r, u.ID, s.Provider) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	leads, err := a.store.Leads(r.Context(), 0)
	if err != nil {
		http.Error(w, "storage", 503)
		return
	}
	if id := r.PathValue("id"); id != "" {
		var one []Lead
		for _, l := range leads {
			if l.ID == id {
				one = append(one, l)
			}
		}
		leads = one
	}
	a.render(w, r, "admin.html", map[string]any{"Leads": leads})
}

func (a *App) isAdmin(r *http.Request, userID, prov string) bool {
	data, err := a.store.ExportUser(r.Context(), userID)
	if err != nil {
		return false
	}
	for _, i := range data.Identities {
		if a.cfg.Admins[i.Provider+":"+i.ProviderUID] {
			return true
		}
	}
	return false
}

// consentKinds: per-demo upload consents (legal/consents.md §1а, §1б). A demo's key may record only its own kind.
var consentKinds = map[string]string{"voice": "voice", "prices": "prices"}

var consentVersionRe = regexp.MustCompile(`^([a-z]+)-v[0-9]{1,3}$`)

func (a *App) internalConsent(w http.ResponseWriter, r *http.Request, demo string, u *User, in internalIn) {
	if in.Kind != "voice" && in.Kind != "prices" {
		moduleErr(w, 400, "invalid_argument", "kind must be voice or prices")
		return
	}
	if m := consentVersionRe.FindStringSubmatch(in.Version); m == nil || m[1] != in.Kind {
		moduleErr(w, 400, "invalid_argument", "version must be <kind>-v<N>")
		return
	}
	if consentKinds[demo] != in.Kind {
		moduleErr(w, 403, "permission_denied", "this service key may not record "+in.Kind+" consent")
		return
	}
	if u == nil {
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "unauthenticated"})
		return
	}
	// The module client retries once on network/5xx errors with the same idempotency_key: answer the repeat
	// from memory instead of writing a second journal row.
	dk := demo + "|" + u.ID + "|" + in.IdempotencyKey
	if in.IdempotencyKey != "" {
		if prev, ok := a.consentDedup.get(dk); ok {
			writeJSON(w, 200, prev)
			return
		}
	}
	now := a.now()
	if err := a.store.RecordKindConsent(r.Context(), u.ID, in.Kind, in.Version, now); err != nil {
		moduleErr(w, 503, "unavailable", "storage")
		return
	}
	resp := map[string]any{"ok": true, "kind": in.Kind, "version": in.Version, "recorded_at": rfc3339(now)}
	if in.IdempotencyKey != "" {
		a.consentDedup.put(dk, resp)
	}
	writeJSON(w, 200, resp)
}

// dedup: short-lived memory of answered idempotency keys (retries arrive within seconds).
type dedup struct {
	mu  sync.Mutex
	m   map[string]dedupEntry
	ttl time.Duration
}

type dedupEntry struct {
	v  map[string]any
	at time.Time
}

func newDedup(ttl time.Duration) *dedup { return &dedup{m: map[string]dedupEntry{}, ttl: ttl} }

func (d *dedup) get(k string) (map[string]any, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.m[k]
	if !ok || time.Since(e.at) > d.ttl {
		return nil, false
	}
	return e.v, true
}

func (d *dedup) put(k string, v map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.m) > 10000 {
		for key, e := range d.m {
			if time.Since(e.at) > d.ttl {
				delete(d.m, key)
			}
		}
	}
	d.m[k] = dedupEntry{v: v, at: time.Now()}
}
