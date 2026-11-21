package main

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Lead is the glue's view of one form submission across all systems.
type Lead struct {
	ID              string         `json:"id"`
	SubmissionID    string         `json:"submission_id"`
	Profile         string         `json:"profile"`
	CreatedAt       string         `json:"created_at"`
	Name            string         `json:"name"`
	Email           string         `json:"email"`
	Phone           string         `json:"phone"`
	Company         string         `json:"company"`
	Message         string         `json:"message"`
	Plan            string         `json:"plan"`
	Amount          Money          `json:"amount"`
	Status          string         `json:"status"` // processing | awaiting_payment | paid | canceled | failed
	ContactID       string         `json:"contact_id,omitempty"`
	DealID          string         `json:"deal_id,omitempty"`
	DealStage       string         `json:"deal_stage,omitempty"`
	DealURL         string         `json:"deal_url,omitempty"`
	PaymentID       string         `json:"payment_id,omitempty"`
	PaymentStatus   string         `json:"payment_status,omitempty"`
	ConfirmationURL string         `json:"confirmation_url,omitempty"`
	Alias           string         `json:"alias"` // client-N: shown instead of email/phone on the public page
	Lang            string         `json:"lang"`
	Steps           map[string]int `json:"steps"` // step → 0 pending, 1 ok, 2 retrying, 3 failed
	lastWebhook     *storedWebhook
}

type Money struct {
	Value    int64  `json:"value"`
	Currency string `json:"currency"`
}

type storedWebhook struct {
	Headers map[string]string
	Body    string
}

// Session is one visitor's isolated sandbox: own mock CRM/sheet/payments/inbox per profile.
type Session struct {
	ID       string
	mu       sync.Mutex
	stacks   map[string]*Stack
	leads    map[string]*Lead
	bySubmit map[string]string
	order    []string
	nLead    int
	Chaos    bool
	redact   []string // raw → display pairs (contacts, recipients), applied to everything the page receives
	created  time.Time
	lastSeen time.Time
}

func (s *Session) Leads(profile string) []*Lead {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Lead
	for i := len(s.order) - 1; i >= 0; i-- {
		l := s.leads[s.order[i]]
		if profile == "" || l.Profile == profile {
			c := *l
			c.Steps = map[string]int{}
			for k, v := range l.Steps {
				c.Steps[k] = v
			}
			out = append(out, &c)
		}
	}
	return out
}

func (s *Session) lead(id string) *Lead {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leads[id]
}

// update runs fn under the session lock (all Lead mutations go through it).
func (s *Session) update(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

var sessionRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// Sessions is a bounded registry; oldest-idle sessions are evicted past the cap.
type Sessions struct {
	mu   sync.Mutex
	m    map[string]*Session
	max  int
	now  func() time.Time
	drop func(id string)
}

func NewSessions(max int, drop func(string)) *Sessions {
	return &Sessions{m: map[string]*Session{}, max: max, now: time.Now, drop: drop}
}

// Get returns the session (creating it); ok=false for a malformed id.
func (ss *Sessions) Get(id string) (*Session, bool) {
	if !sessionRe.MatchString(id) {
		return nil, false
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := ss.now()
	s := ss.m[id]
	if s == nil {
		if len(ss.m) >= ss.max {
			ss.evictLocked(len(ss.m) - ss.max + 1)
		}
		s = &Session{ID: id, stacks: map[string]*Stack{}, leads: map[string]*Lead{}, bySubmit: map[string]string{}, created: now}
		ss.m[id] = s
	}
	s.lastSeen = now
	return s, true
}

func (ss *Sessions) evictLocked(n int) {
	type kv struct {
		id string
		t  time.Time
	}
	all := make([]kv, 0, len(ss.m))
	for id, s := range ss.m {
		all = append(all, kv{id, s.lastSeen})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].t.Before(all[j].t) })
	for i := 0; i < n && i < len(all); i++ {
		delete(ss.m, all[i].id)
		if ss.drop != nil {
			ss.drop(all[i].id)
		}
	}
}

// Purge removes sessions idle longer than maxIdle (maxIdle=0 → everything). Returns how many.
func (ss *Sessions) Purge(maxIdle time.Duration) int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	n := 0
	cut := ss.now().Add(-maxIdle)
	for id, s := range ss.m {
		if maxIdle == 0 || s.lastSeen.Before(cut) {
			delete(ss.m, id)
			if ss.drop != nil {
				ss.drop(id)
			}
			n++
		}
	}
	return n
}

func (ss *Sessions) Len() int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return len(ss.m)
}

// Peek returns an existing session without creating or touching it.
func (ss *Sessions) Peek(id string) *Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.m[id]
}

// Remove drops one session (Reset button).
func (ss *Sessions) Remove(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, ok := ss.m[id]; ok {
		delete(ss.m, id)
	}
	if ss.drop != nil {
		ss.drop(id)
	}
}

// addRedaction registers raw → display; empty raw values are ignored.
func (s *Session) addRedaction(pairs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i]) != "" {
			s.redact = append(s.redact, pairs[i], pairs[i+1])
		}
	}
}

// Redact replaces contacts and recipient handles in page-bound bytes (JSON / SSE). The public demo never
// shows anything that looks like a contact: portfolio screenshots go to marketplaces with strict moderation.
func (s *Session) Redact(b []byte) []byte {
	s.mu.Lock()
	pairs := append([]string(nil), s.redact...)
	s.mu.Unlock()
	if len(pairs) == 0 {
		return b
	}
	// longest raw values first so a substring never wins over the full value
	type kv struct{ k, v string }
	kvs := make([]kv, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		kvs = append(kvs, kv{pairs[i], pairs[i+1]})
	}
	sort.SliceStable(kvs, func(i, j int) bool { return len(kvs[i].k) > len(kvs[j].k) })
	flat := make([]string, 0, len(pairs))
	for _, p := range kvs {
		flat = append(flat, p.k, p.v)
	}
	return []byte(strings.NewReplacer(flat...).Replace(string(b)))
}
