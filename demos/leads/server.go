package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	crm "github.com/sedyukov/sedlab-modules/modules/crm/go"
	crmkit "github.com/sedyukov/sedlab-modules/modules/crm/go/kit"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
)

//go:embed web
var webFS embed.FS

// Guard: anti-spam for a public demo — per-IP token buckets, honeypot, time trap, per-session cap.
type Guard struct {
	Submit         *crmkit.KeyedLimiter // form posts per IP
	API            *crmkit.KeyedLimiter // other API calls per IP
	Global         *crmkit.Limiter      // all form posts
	MaxLeads       int                  // per session
	MinFillTime    time.Duration
	AllowedOrigins []string // CORS for the embeddable widget; "*" = any
	// SandboxContacts: the public demo never accepts real email/phone (personal data of RF citizens must not be
	// collected on a server abroad, 152-FZ). Typed contacts are discarded and a sandbox contact is used instead.
	// Client deployments set ACCEPT_REAL_CONTACTS=1 (and run in a compliant jurisdiction with a consent/policy).
	SandboxContacts bool
}

func DefaultGuard() *Guard {
	return &Guard{
		Submit:          crmkit.NewKeyedLimiter(5.0/60, 3), // 3 at once, then 1 per 12 s
		API:             crmkit.NewKeyedLimiter(2, 30),
		Global:          crmkit.NewLimiter(1, 30),
		MaxLeads:        25,
		MinFillTime:     1500 * time.Millisecond,
		SandboxContacts: true,
	}
}

func clientIP(r *http.Request) string {
	// Behind Caddy: it appends the real peer to X-Forwarded-For, so the LAST entry is the trusted one.
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

type Server struct {
	App   *App
	Guard *Guard
	mux   *http.ServeMux
}

func NewServer(app *App, g *Guard) *Server {
	s := &Server{App: app, Guard: g, mux: http.NewServeMux()}
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	s.mux.HandleFunc("GET /checkout/{id}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := webFS.ReadFile("web/checkout.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})
	s.mux.HandleFunc("GET /api/config", s.config)
	s.mux.HandleFunc("GET /api/events", s.events)
	s.mux.HandleFunc("GET /api/state", s.limited(s.state))
	s.mux.HandleFunc("OPTIONS /api/leads", s.cors(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	s.mux.HandleFunc("POST /api/leads", s.cors(s.submit))
	s.mux.HandleFunc("POST /api/leads/{id}/{action}", s.limited(s.leadAction))
	s.mux.HandleFunc("POST /api/session/{action}", s.limited(s.sessionAction))
	s.mux.HandleFunc("POST /webhooks/payments/{profile}", s.paymentWebhook)
	s.mux.HandleFunc("POST /webhooks/crm/{profile}", s.crmWebhook)
	s.mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/widget.js" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		files.ServeHTTP(w, r)
	}))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]string{"code": errCode, "message": msg}})
}

func (s *Server) limited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Guard.API != nil && !s.Guard.API.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "5")
			apiErr(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		h(w, r)
	}
}

func (s *Server) cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		h(w, r)
	}
}

func (s *Server) originAllowed(o string) bool {
	if len(s.Guard.AllowedOrigins) == 0 {
		return true
	}
	for _, a := range s.Guard.AllowedOrigins {
		if a == "*" || strings.EqualFold(a, o) {
			return true
		}
	}
	return false
}

func (s *Server) session(w http.ResponseWriter, id string) (*Session, bool) {
	sess, ok := s.App.Sessions.Get(id)
	if !ok {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "session must be 16..64 chars [A-Za-z0-9_-]")
	}
	return sess, ok
}

func (s *Server) profile(w http.ResponseWriter, key string) (string, bool) {
	if key == "" {
		key = "ru"
	}
	if s.App.Profiles[key] == nil {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "profile must be ru or intl")
		return "", false
	}
	return key, true
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	ps := []*Profile{s.App.Profiles["ru"], s.App.Profiles["intl"]}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": ps, "retention": "24h", "sandbox_contacts": s.Guard.SandboxContacts})
}

// events streams the session's log as Server-Sent Events (backlog first, then live; Last-Event-ID resumes).
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r.URL.Query().Get("s"))
	if !ok {
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		apiErr(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if q := r.URL.Query().Get("after"); q != "" {
		after, _ = strconv.ParseInt(q, 10, 64)
	}
	backlog, ch, cancel := s.App.Hub.Subscribe(sess.ID, after)
	defer cancel()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	send := func(ev Event) error {
		b, _ := json.Marshal(ev)
		_, err := fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", ev.Seq, sess.Redact(b))
		return err
	}
	fmt.Fprint(w, "retry: 2000\n\n")
	for _, ev := range backlog {
		if send(ev) != nil {
			return
		}
	}
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				fmt.Fprint(w, "event: reset\ndata: {}\n\n")
				fl.Flush()
				return
			}
			if send(ev) != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// state is the snapshot for the side panels: leads, CRM deals, sheet rows, delivered messages.
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(w, r.URL.Query().Get("s"))
	if !ok {
		return
	}
	prof, ok := s.profile(w, r.URL.Query().Get("profile"))
	if !ok {
		return
	}
	st := s.App.stack(sess, prof)
	var chaos bool
	sess.update(func() { chaos = sess.Chaos })
	out := map[string]any{"profile": prof, "chaos": chaos, "leads": sess.Leads(prof), "modes": s.App.Profiles[prof].Modes}
	ctx := r.Context()
	if rows, err := st.Sheets.ReadRows(ctx, sheets.ReadRowsInput{SheetID: st.SheetID, Tab: st.SheetTab}); err == nil {
		out["sheet"] = rows.Rows
	} else {
		out["sheet"] = [][]string{}
	}
	msgs, err := st.Notify.ListDeliveries("")
	sent := []notify.LogEntry{}
	if err == nil {
		for _, m := range msgs {
			if m.Status != "failed" { // failed attempts are visible in the log as retries
				sent = append(sent, m)
			}
		}
	}
	out["messages"] = sent
	b, _ := json.Marshal(out)
	w.Header().Set("Content-Type", "application/json")
	w.Write(sess.Redact(b))
}

type submitBody struct {
	Session      string `json:"session"`
	Profile      string `json:"profile"`
	SubmissionID string `json:"submission_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Company      string `json:"company"`
	Message      string `json:"message"`
	Plan         string `json:"plan"`
	Lang         string `json:"lang"`
	Website      string `json:"website"`    // honeypot: humans never see it
	ElapsedMs    int64  `json:"elapsed_ms"` // time between widget render and submit
	Consent      bool   `json:"consent"`
	DemoContact  bool   `json:"demo_contact"` // "fill with test data": the server makes up a sandbox contact
}

func clean(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' {
			return -1
		}
		return r
	}, s))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var b submitBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "JSON body expected")
		return
	}
	sess, ok := s.session(w, b.Session)
	if !ok {
		return
	}
	prof, ok := s.profile(w, b.Profile)
	if !ok {
		return
	}
	reject := func(code int, errCode, reason, msg string) {
		s.App.Hub.Publish(sess.ID, Event{Step: "guard", Service: "guard", Status: "rejected", Message: msg, Data: map[string]any{"reason": reason}})
		apiErr(w, code, errCode, msg)
	}
	// Bots: pretend success (202) so they learn nothing; the visitor's log shows what happened.
	if b.Website != "" || (b.ElapsedMs > 0 && time.Duration(b.ElapsedMs)*time.Millisecond < s.Guard.MinFillTime) {
		reason := "honeypot field filled"
		if b.Website == "" {
			reason = fmt.Sprintf("form filled in %d ms (bot-fast)", b.ElapsedMs)
		}
		s.App.Hub.Publish(sess.ID, Event{Step: "guard", Service: "guard", Status: "rejected",
			Message: "Spam filter: " + reason + " → silently dropped (HTTP 202, nothing created)", Data: map[string]any{"reason": reason}})
		writeJSON(w, http.StatusAccepted, map[string]any{"lead_id": "L-000", "accepted": true})
		return
	}
	if (s.Guard.Submit != nil && !s.Guard.Submit.Allow(ip)) || (s.Guard.Global != nil && !s.Guard.Global.Allow()) {
		w.Header().Set("Retry-After", "12")
		reject(http.StatusTooManyRequests, "rate_limited", "rate_limit", "Rate limit: too many submissions from this IP — try again in a few seconds (HTTP 429)")
		return
	}
	if len(sess.Leads("")) >= s.Guard.MaxLeads {
		reject(http.StatusTooManyRequests, "rate_limited", "session_cap", fmt.Sprintf("Demo limit: %d leads per session — press Reset", s.Guard.MaxLeads))
		return
	}
	in := LeadInput{SubmissionID: clean(b.SubmissionID, 64), Profile: prof, Name: clean(b.Name, 80), Email: clean(b.Email, 120),
		Phone: clean(b.Phone, 32), Company: clean(b.Company, 80), Message: clean(b.Message, 500), Plan: clean(b.Plan, 20), Lang: b.Lang}
	if in.SubmissionID == "" {
		in.SubmissionID = fmt.Sprintf("srv-%d", time.Now().UnixNano())
	}
	if s.Guard.SandboxContacts {
		in.Email, in.Phone = "", "" // never stored: see Guard.SandboxContacts
		b.DemoContact = true
	}
	if b.DemoContact && in.Email == "" && in.Phone == "" {
		in.Email = fmt.Sprintf("client-%d@example.com", len(sess.Leads(""))+1)
	}
	if in.Name == "" || (in.Email == "" && in.Phone == "") {
		apiErr(w, http.StatusUnprocessableEntity, "invalid_argument", "name and email or phone are required")
		return
	}
	if in.Email != "" {
		if _, err := crm.NormalizeEmail(in.Email); err != nil {
			apiErr(w, http.StatusUnprocessableEntity, "invalid_argument", "invalid email")
			return
		}
	}
	if in.Phone != "" {
		if _, err := crm.NormalizePhone(in.Phone); err != nil {
			apiErr(w, http.StatusUnprocessableEntity, "invalid_argument", "invalid phone")
			return
		}
	}
	if _, ok := s.App.Profiles[prof].Plan(in.Plan); !ok {
		in.Plan = "business"
	}
	lead, dup := s.App.Submit(sess, in)
	writeJSON(w, http.StatusAccepted, map[string]any{"lead_id": lead.ID, "accepted": true, "duplicate": dup})
}

type actionBody struct {
	Session string `json:"session"`
	Outcome string `json:"outcome"`
	On      bool   `json:"on"`
}

func readAction(w http.ResponseWriter, r *http.Request) (actionBody, bool) {
	var b actionBody
	r.Body = http.MaxBytesReader(w, r.Body, 2<<10)
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "JSON body expected")
		return b, false
	}
	return b, true
}

func (s *Server) leadAction(w http.ResponseWriter, r *http.Request) {
	b, ok := readAction(w, r)
	if !ok {
		return
	}
	sess, ok := s.session(w, b.Session)
	if !ok {
		return
	}
	lead := sess.lead(r.PathValue("id"))
	if lead == nil {
		apiErr(w, http.StatusNotFound, "not_found", "lead not found")
		return
	}
	if lead.PaymentID == "" {
		apiErr(w, http.StatusConflict, "conflict", "payment link is not ready yet")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var err error
	switch r.PathValue("action") {
	case "pay":
		if b.Outcome == "" {
			b.Outcome = "succeeded"
		}
		if b.Outcome != "succeeded" && b.Outcome != "canceled" {
			apiErr(w, http.StatusBadRequest, "invalid_argument", "outcome must be succeeded or canceled")
			return
		}
		err = s.App.MockPay(ctx, sess, lead, b.Outcome)
	case "replay-webhook":
		err = s.App.ReplayWebhook(ctx, sess, lead)
	case "forge-webhook":
		err = s.App.ForgeWebhook(ctx, sess, lead)
	default:
		apiErr(w, http.StatusNotFound, "not_found", "unknown action")
		return
	}
	if err != nil {
		info := classify(err)
		apiErr(w, http.StatusConflict, info.Code, info.Message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sessionAction(w http.ResponseWriter, r *http.Request) {
	b, ok := readAction(w, r)
	if !ok {
		return
	}
	sess, ok := s.session(w, b.Session)
	if !ok {
		return
	}
	switch r.PathValue("action") {
	case "chaos":
		sess.update(func() { sess.Chaos = b.On })
		msg := "Failure simulation OFF"
		if b.On {
			msg = "Failure simulation ON: CRM 503, Sheets 429, Telegram 502/timeout on first attempts — watch the retries"
		}
		s.App.Hub.Publish(sess.ID, Event{Step: "settings", Service: "system", Status: "info", Message: msg})
	case "reset":
		s.App.Sessions.Remove(sess.ID) // drops its sandbox + log; open streams get a "reset" event
	default:
		apiErr(w, http.StatusNotFound, "not_found", "unknown action")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func headerMap(r *http.Request) map[string]string {
	h := map[string]string{}
	for k, v := range r.Header {
		if len(v) > 0 {
			h[k] = v[0]
		}
	}
	h["Remote-Addr"] = clientIP(r)
	return h
}

func (s *Server) paymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(s.App.HandlePaymentWebhook(r.Context(), r.PathValue("profile"), headerMap(r), body))
}

func (s *Server) crmWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(s.App.HandleCRMWebhook(r.Context(), r.PathValue("profile"), r.URL.Query().Get("s"), headerMap(r), body))
}
