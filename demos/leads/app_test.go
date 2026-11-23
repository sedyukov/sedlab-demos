package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type harness struct {
	t   *testing.T
	srv *httptest.Server
	app *App
}

func newHarness(t *testing.T, g *Guard) *harness { return newEditionHarness(t, g, EditionCombined) }

func newEditionHarness(t *testing.T, g *Guard, edition string) *harness {
	t.Helper()
	profiles, err := LoadProfiles(func(string) (string, bool) { return "", false }, edition)
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(profiles, "https://leads.test", "", 100)
	app.Edition = edition
	app.Retry.Sleep = func(context.Context, time.Duration) error { return nil }
	if g == nil {
		g = DefaultGuard()
		g.Submit, g.API, g.Global = nil, nil, nil
		g.SandboxContacts = false // most tests exercise real-contact handling (client deployments)
	}
	h := &harness{t: t, app: app}
	srv := httptest.NewServer(NewServer(app, g))
	t.Cleanup(srv.Close)
	app.SelfURL = srv.URL
	h.srv = srv
	return h
}

func (h *harness) post(path string, body any) (int, map[string]any) {
	h.t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(h.srv.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func sid(n int) string { return fmt.Sprintf("test-session-%04d-abcdef", n) }

func form(session, profile, subID string) map[string]any {
	return map[string]any{"session": session, "profile": profile, "submission_id": subID, "name": "Ivan Petrov",
		"email": "ivan@example.com", "phone": "8 (999) 123-45-67", "company": "Acme", "plan": "business", "elapsed_ms": 5000, "lang": "en"}
}

// waitFor polls the lead until cond holds.
func (h *harness) waitFor(session, leadID string, cond func(*Lead) bool) *Lead {
	h.t.Helper()
	s := h.app.Sessions.Peek(session)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, l := range s.Leads("") {
			if l.ID == leadID && cond(l) {
				return l
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, e := range h.app.Hub.Events(session) {
		h.t.Logf("%s %s %s", e.Step, e.Status, e.Message)
	}
	h.t.Fatalf("timeout waiting for lead %s", leadID)
	return nil
}

func (h *harness) count(session, step, status string) int {
	n := 0
	for _, e := range h.app.Hub.Events(session) {
		if (step == "" || e.Step == step) && e.Status == status {
			n++
		}
	}
	return n
}

func (h *harness) waitEvent(session, step, status string, n int) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if h.count(session, step, status) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("no %d× %s/%s event", n, step, status)
}

func (h *harness) submitAndWait(session, profile, sub string) *Lead {
	h.t.Helper()
	code, out := h.post("/api/leads", form(session, profile, sub))
	if code != http.StatusAccepted {
		h.t.Fatalf("submit: %d %v", code, out)
	}
	return h.waitFor(session, out["lead_id"].(string), func(l *Lead) bool { return l.Status == "awaiting_payment" })
}

func TestFullFlowRU(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(1)
	lead := h.submitAndWait(s, "ru", "sub-1")
	if lead.DealID == "" || lead.ContactID == "" || lead.PaymentID == "" || lead.Amount.Currency != "RUB" || lead.Amount.Value != 1500000 {
		t.Fatalf("lead not wired: %+v", lead)
	}
	if !strings.Contains(lead.ConfirmationURL, "https://leads.test/checkout/") {
		t.Fatalf("checkout url %q", lead.ConfirmationURL)
	}
	sess := h.app.Sessions.Peek(s)
	st := h.app.stack(sess, "ru")
	msgs := sentOnly(st.Notify.ListDeliveries("telegram"))
	if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "New lead") {
		t.Fatalf("telegram: %+v", msgs)
	}
	h.waitEvent(s, "webhook.crm", "ok", 1) // deal.created came back as a signed CRM webhook

	if code, out := h.post("/api/leads/"+lead.ID+"/pay", map[string]any{"session": s}); code != 200 {
		t.Fatalf("pay: %d %v", code, out)
	}
	paid := h.waitFor(s, lead.ID, func(l *Lead) bool { return l.Status == "paid" && l.DealStage == "Won" })
	if paid.PaymentStatus != "succeeded" {
		t.Fatalf("payment status %q", paid.PaymentStatus)
	}
	h.waitEvent(s, "sheets.update_row", "ok", 1)
	rows := sheetRows(t, h, s, "ru")
	if len(rows) != 2 || rows[1][0] != lead.ID || rows[1][9] != "Won" {
		t.Fatalf("sheet: %v", rows)
	}
	h.waitEvent(s, "notify.telegram", "ok", 2)
	if h.count(s, "", "error") != 0 {
		t.Fatal("unexpected errors in log")
	}
}

func sheetRows(t *testing.T, h *harness, s, profile string) [][]string {
	resp, err := http.Get(h.srv.URL + "/api/state?s=" + s + "&profile=" + profile)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st struct {
		Sheet [][]string `json:"sheet"`
	}
	json.NewDecoder(resp.Body).Decode(&st)
	return st.Sheet
}

func TestInternationalProfile(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(2)
	lead := h.submitAndWait(s, "intl", "sub-1")
	if lead.Amount != (Money{29900, "USD"}) {
		t.Fatalf("amount %+v", lead.Amount)
	}
	msgs := sentOnly(h.app.stack(h.app.Sessions.Peek(s), "intl").Notify.ListDeliveries(""))
	if len(msgs) != 2 || msgs[0].Channel != "telegram" || msgs[1].Channel != "email" {
		t.Fatalf("deliveries: %+v", msgs)
	}
	for _, e := range h.app.Hub.Events(s) {
		if e.Service == "crm" && e.Provider != "CRM · simulation" {
			t.Fatalf("provider label %q", e.Provider)
		}
	}
}

func TestDuplicateSubmissionIsIdempotent(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(3)
	lead := h.submitAndWait(s, "ru", "same-id")
	code, out := h.post("/api/leads", form(s, "ru", "same-id"))
	if code != 202 || out["duplicate"] != true || out["lead_id"] != lead.ID {
		t.Fatalf("dup submit: %d %v", code, out)
	}
	if n := len(h.app.Sessions.Peek(s).Leads("")); n != 1 {
		t.Fatalf("leads: %d", n)
	}
	if rows := sheetRows(t, h, s, "ru"); len(rows) != 2 {
		t.Fatalf("sheet rows: %d", len(rows))
	}
	// same contact on a new submission: matched, not duplicated
	h.submitAndWait(s, "ru", "other-id")
	found := false
	for _, e := range h.app.Hub.Events(s) {
		if e.Step == "crm.upsert_contact" && strings.HasPrefix(e.Message, "Contact matched") {
			found = true
		}
	}
	if !found {
		t.Fatal("second lead from the same person should match the existing contact")
	}
}

func TestWebhookReplayAndForgery(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(4)
	lead := h.submitAndWait(s, "ru", "sub-1")
	if code, _ := h.post("/api/leads/"+lead.ID+"/replay-webhook", map[string]any{"session": s}); code != 409 {
		t.Fatalf("replay before pay should be 409, got %d", code)
	}
	h.post("/api/leads/"+lead.ID+"/pay", map[string]any{"session": s})
	h.waitFor(s, lead.ID, func(l *Lead) bool { return l.DealStage == "Won" })
	h.waitEvent(s, "notify.telegram", "ok", 2)

	h.post("/api/leads/"+lead.ID+"/replay-webhook", map[string]any{"session": s})
	h.waitEvent(s, "webhook.payment", "duplicate", 1)

	h.post("/api/leads/"+lead.ID+"/forge-webhook", map[string]any{"session": s})
	h.waitEvent(s, "webhook.payment", "rejected", 1)
	time.Sleep(50 * time.Millisecond)
	if n := h.count(s, "crm.update_deal", "ok"); n != 1 {
		t.Fatalf("deal updated %d times, want 1", n)
	}
	msgs := sentOnly(h.app.stack(h.app.Sessions.Peek(s), "ru").Notify.ListDeliveries(""))
	if len(msgs) != 2 {
		t.Fatalf("notifications: %d, want 2 (new + paid)", len(msgs))
	}
}

func TestForgedWebhookHTTPStatus(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(5)
	lead := h.submitAndWait(s, "ru", "sub-1")
	body := fmt.Sprintf(`{"id":"evt_x","type":"payment.succeeded","payment":{"id":%q,"metadata":{"session":%q,"lead_id":%q}}}`, lead.PaymentID, s, lead.ID)
	resp, err := http.Post(h.srv.URL+"/webhooks/payments/ru", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unsigned webhook: %d, want 401", resp.StatusCode)
	}
	if l := h.app.Sessions.Peek(s).lead(lead.ID); l.Status != "awaiting_payment" {
		t.Fatalf("status changed to %s", l.Status)
	}
}

func TestCanceledPaymentMarksDealLost(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(6)
	lead := h.submitAndWait(s, "intl", "sub-1")
	h.post("/api/leads/"+lead.ID+"/pay", map[string]any{"session": s, "outcome": "canceled"})
	h.waitFor(s, lead.ID, func(l *Lead) bool { return l.Status == "canceled" && l.DealStage == "Lost" })
}

func TestChaosRetriesStillComplete(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(7)
	h.post("/api/session/chaos", map[string]any{"session": s, "on": true})
	lead := h.submitAndWait(s, "ru", "sub-1")
	if lead.Steps["crm.create_deal"] != 1 || lead.Steps["sheets.append_rows"] != 1 {
		t.Fatalf("steps: %v", lead.Steps)
	}
	if n := h.count(s, "", "retry"); n < 4 {
		t.Fatalf("retries: %d", n)
	}
	for _, e := range h.app.Hub.Events(s) {
		if e.Step == "sheets.append_rows" && e.Status == "retry" && e.Data["retry_in_ms"].(int64) < 1200 {
			t.Fatalf("Retry-After not honoured: %v", e.Data)
		}
	}
	if rows := sheetRows(t, h, s, "ru"); len(rows) != 2 {
		t.Fatalf("sheet rows after retries: %d", len(rows))
	}
}

func TestHoneypotAndTimeTrap(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(8)
	f := form(s, "ru", "bot-1")
	f["website"] = "http://spam.example"
	if code, _ := h.post("/api/leads", f); code != 202 {
		t.Fatalf("honeypot should look accepted, got %d", code)
	}
	f = form(s, "ru", "bot-2")
	f["elapsed_ms"] = 200
	h.post("/api/leads", f)
	if n := len(h.app.Sessions.Peek(s).Leads("")); n != 0 {
		t.Fatalf("bot leads created: %d", n)
	}
	if h.count(s, "guard", "rejected") != 2 {
		t.Fatal("guard events missing")
	}
}

func TestRateLimit(t *testing.T) {
	g := DefaultGuard()
	h := newHarness(t, g)
	s := sid(9)
	codes := []int{}
	for i := 0; i < 5; i++ {
		c, _ := h.post("/api/leads", form(s, "ru", fmt.Sprintf("rl-%d", i)))
		codes = append(codes, c)
	}
	if codes[0] != 202 || codes[2] != 202 || codes[3] != 429 {
		t.Fatalf("codes: %v", codes)
	}
}

func TestValidation(t *testing.T) {
	h := newHarness(t, nil)
	f := form(sid(10), "ru", "v1")
	f["email"], f["phone"] = "", ""
	if c, _ := h.post("/api/leads", f); c != 422 {
		t.Fatalf("no contact: %d", c)
	}
	f = form(sid(10), "ru", "v2")
	f["email"] = "not-an-email"
	if c, _ := h.post("/api/leads", f); c != 422 {
		t.Fatalf("bad email: %d", c)
	}
	f = form("short", "ru", "v3")
	if c, _ := h.post("/api/leads", f); c != 400 {
		t.Fatalf("bad session: %d", c)
	}
	f = form(sid(10), "mars", "v4")
	if c, _ := h.post("/api/leads", f); c != 400 {
		t.Fatalf("bad profile: %d", c)
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	h := newHarness(t, nil)
	h.submitAndWait(sid(11), "ru", "a")
	resp, _ := http.Get(h.srv.URL + "/api/state?s=" + sid(12) + "&profile=ru")
	var st struct {
		Leads []any      `json:"leads"`
		Sheet [][]string `json:"sheet"`
	}
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if len(st.Leads) != 0 || len(st.Sheet) != 0 {
		t.Fatalf("other session leaks: %+v", st)
	}
}

func TestSSEStream(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(13)
	h.submitAndWait(s, "ru", "a")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", h.srv.URL+"/api/events?s="+s, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	n := 0
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data: ") {
			var ev Event
			if err := json.Unmarshal([]byte(sc.Text()[6:]), &ev); err != nil {
				t.Fatal(err)
			}
			if n++; n >= 5 {
				break
			}
		}
	}
	if n < 5 {
		t.Fatalf("got %d events", n)
	}
}

func TestResetAndPurge(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(14)
	h.submitAndWait(s, "ru", "a")
	h.post("/api/session/reset", map[string]any{"session": s})
	if h.app.Sessions.Peek(s) != nil || len(h.app.Hub.Events(s)) != 0 {
		t.Fatal("reset left data")
	}
	h.submitAndWait(sid(15), "ru", "a")
	if n := h.app.Sessions.Purge(0); n != 1 {
		t.Fatalf("purged %d", n)
	}
}

func TestSessionCapEvictsOldest(t *testing.T) {
	ss := NewSessions(2, nil)
	clock := time.Unix(0, 0)
	ss.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	ss.Get(sid(1))
	ss.Get(sid(2))
	ss.Get(sid(1)) // touch
	ss.Get(sid(3))
	if ss.Peek(sid(2)) != nil || ss.Peek(sid(1)) == nil || ss.Len() != 2 {
		t.Fatal("LRU eviction wrong")
	}
}

func TestWidgetAndCORS(t *testing.T) {
	h := newHarness(t, nil)
	resp, err := http.Get(h.srv.URL + "/widget.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("widget: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("OPTIONS", h.srv.URL+"/api/leads", nil)
	req.Header.Set("Origin", "https://landing.example")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://landing.example" {
		t.Fatal("CORS preflight")
	}
}

func sentOnly(es []notify.LogEntry, _ error) []notify.LogEntry {
	var out []notify.LogEntry
	for _, e := range es {
		if e.Status != "failed" {
			out = append(out, e)
		}
	}
	return out
}

// The public page must never receive anything that looks like a contact (marketplace moderation).
func TestPageShowsNoContacts(t *testing.T) {
	h := newHarness(t, nil)
	s := sid(16)
	lead := h.submitAndWait(s, "intl", "pii-1")
	h.post("/api/leads/"+lead.ID+"/pay", map[string]any{"session": s})
	h.waitFor(s, lead.ID, func(l *Lead) bool { return l.Status == "paid" })
	h.waitEvent(s, "pipeline", "ok", 2)
	f := form(s, "ru", "pii-2")
	f["email"], f["phone"], f["demo_contact"] = "", "", true
	if c, _ := h.post("/api/leads", f); c != 202 {
		t.Fatalf("demo contact submit: %d", c)
	}
	h.waitEvent(s, "pipeline", "ok", 3)

	var pages []string
	for _, p := range []string{"ru", "intl"} {
		resp, _ := http.Get(h.srv.URL + "/api/state?s=" + s + "&profile=" + p)
		b := new(bytes.Buffer)
		b.ReadFrom(resp.Body)
		resp.Body.Close()
		pages = append(pages, b.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", h.srv.URL+"/api/events?s="+s, nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		b := new(bytes.Buffer)
		b.ReadFrom(resp.Body) // until ctx timeout
		resp.Body.Close()
		pages = append(pages, b.String())
	}
	all := strings.Join(pages, "\n")
	// exact raw contacts (not bare digit runs: timestamps like "…46.999Z" would match)
	for _, bad := range []string{"@", "example.com", "123-45-67", "+79991234567", "(999)"} {
		if strings.Contains(all, bad) {
			i := strings.Index(all, bad)
			t.Fatalf("page data contains %q: …%s…", bad, all[max(0, i-60):min(len(all), i+40)])
		}
	}
	for _, want := range []string{"client-1 (email)", "Sales manager (messenger)", "client-2"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

// Public demo mode: typed contacts are discarded, nothing real is stored anywhere.
func TestSandboxContactsDiscardTypedContacts(t *testing.T) {
	g := DefaultGuard()
	g.Submit, g.API, g.Global = nil, nil, nil
	h := newHarness(t, g)
	s := sid(17)
	lead := h.submitAndWait(s, "ru", "sbx-1")
	if lead.Phone != "" || lead.Email != "client-1@example.com" {
		t.Fatalf("typed contacts stored: email=%q phone=%q", lead.Email, lead.Phone)
	}
	f := form(s, "ru", "sbx-2")
	f["email"], f["phone"] = "", ""
	if c, _ := h.post("/api/leads", f); c != 202 {
		t.Fatalf("sandbox mode must not require contacts: %d", c)
	}
	resp, _ := http.Get(h.srv.URL + "/api/config")
	var cfg map[string]any
	json.NewDecoder(resp.Body).Decode(&cfg)
	resp.Body.Close()
	if cfg["sandbox_contacts"] != true {
		t.Fatal("config must announce sandbox_contacts")
	}
}

func TestEditionRU(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	s := sid(18)
	f := form(s, "intl", "x")
	if c, _ := h.post("/api/leads", f); c != 400 {
		t.Fatalf("intl profile in ru edition: %d, want 400", c)
	}
	for _, prof := range []string{"ru", "ru_b24"} {
		lead := h.submitAndWait(s, prof, "ed-"+prof)
		if lead.Lang != "ru" || lead.Amount.Currency != "RUB" {
			t.Fatalf("%s: lang %q currency %q", prof, lead.Lang, lead.Amount.Currency)
		}
		msgs := sentOnly(h.app.stack(h.app.Sessions.Peek(s), prof).Notify.ListDeliveries(""))
		if len(msgs) != 2 || msgs[0].Channel != "telegram" || msgs[1].Channel != "max" || !strings.Contains(msgs[1].Text, "Новая заявка") ||
			strings.Contains(msgs[0].Text, "Ivan") {
			t.Fatalf("%s deliveries: %+v", prof, msgs)
		}
	}
	// mocks never carry a real provider's name: "CRM · имитация", not "Битрикс24 · песочница"
	want := map[string]string{"crm": "CRM · имитация", "sheets": "Таблица · имитация", "payments": "Оплата · имитация", "notify": "Мессенджер · имитация"}
	seen := map[string]bool{}
	for _, e := range h.app.Hub.Events(s) {
		if p, ok := want[e.Service]; ok && e.Provider == p {
			seen[e.Service] = true
		}
		for _, real := range []string{"Битрикс24", "amoCRM", "ЮKassa", "Яндекс", "Telegram", "Max", "Google", "HubSpot", "Stripe", "песочн"} {
			if strings.Contains(e.Provider, real) {
				t.Fatalf("real provider name on a simulated step: %q", e.Provider)
			}
		}
	}
	if len(seen) < 3 {
		t.Fatalf("labels seen: %v", seen)
	}
	resp, _ := http.Get(h.srv.URL + "/api/config")
	var cfg struct {
		Edition  string     `json:"edition"`
		Profiles []*Profile `json:"profiles"`
	}
	json.NewDecoder(resp.Body).Decode(&cfg)
	resp.Body.Close()
	if cfg.Edition != "ru" || len(cfg.Profiles) != 2 || cfg.Profiles[0].Key != "ru" || cfg.Profiles[1].Key != "ru_b24" {
		t.Fatalf("config: %+v", cfg)
	}
}

func TestEditionIntl(t *testing.T) {
	h := newEditionHarness(t, nil, EditionIntl)
	s := sid(19)
	if c, _ := h.post("/api/leads", form(s, "ru", "x")); c != 400 {
		t.Fatalf("ru profile in intl edition: %d, want 400", c)
	}
	f := form(s, "", "def")
	f["lang"] = "ru" // ignored: the edition speaks English
	code, out := h.post("/api/leads", f)
	if code != 202 {
		t.Fatalf("default profile: %d %v", code, out)
	}
	lead := h.waitFor(s, out["lead_id"].(string), func(l *Lead) bool { return l.Status == "awaiting_payment" })
	if lead.Profile != "intl" || lead.Lang != "en" || lead.Amount.Currency != "USD" {
		t.Fatalf("lead: %+v", lead)
	}
	if h.app.DefaultProfile() != "intl" {
		t.Fatal("default profile")
	}
}

func TestUnknownEdition(t *testing.T) {
	if _, err := LoadProfiles(func(string) (string, bool) { return "", false }, "mars"); err == nil {
		t.Fatal("unknown edition must fail")
	}
}

func TestRUEditionLogIsRussian(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	s := sid(20)
	lead := h.submitAndWait(s, "ru", "lang-1")
	h.post("/api/leads/"+lead.ID+"/pay", map[string]any{"session": s})
	h.waitFor(s, lead.ID, func(l *Lead) bool { return l.DealStage == "Won" })
	h.waitEvent(s, "pipeline", "ok", 2)
	h.waitEvent(s, "sheets.update_row", "ok", 1)
	rows := sheetRows(t, h, s, "ru")
	if len(rows) != 2 || rows[0][0] != "Заявка" || rows[1][6] != "Бизнес" || rows[1][9] != "Успешно" {
		t.Fatalf("ru sheet: %v", rows)
	}
	for _, e := range h.app.Hub.Events(s) {
		for _, en := range []string{"Form submitted", "Contact created", "created, stage", "Row appended", "Sent to", "Payment", "Webhook", "Lead processed", "sandbox"} {
			if strings.Contains(e.Message, en) || strings.Contains(e.Provider, en) {
				t.Fatalf("English in ru edition: %q / %q", e.Message, e.Provider)
			}
		}
	}
}

// Live Bitrix24 is switched on per profile by env (RU_B24_CRM_*); the amoCRM stack stays on its own settings.
func TestBitrix24LiveByEnv(t *testing.T) {
	env := map[string]string{"RU_B24_CRM_PROVIDER": "bitrix24", "RU_B24_CRM_BITRIX24_WEBHOOK_URL": "https://portal.example/rest/1/token/"}
	ps, err := LoadProfiles(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, EditionRU)
	if err != nil {
		t.Fatal(err)
	}
	if ps["ru_b24"].Modes.CRM != "live" || ps["ru_b24"].shared.CRM.Name() != "bitrix24" {
		t.Fatalf("ru_b24 crm: %s %v", ps["ru_b24"].Modes.CRM, ps["ru_b24"].shared.CRM)
	}
	if ps["ru"].Modes.CRM != "mock" {
		t.Fatalf("ru crm must stay mock: %s", ps["ru"].Modes.CRM)
	}
	delete(env, "RU_B24_CRM_BITRIX24_WEBHOOK_URL")
	if _, err := LoadProfiles(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, EditionRU); err == nil {
		t.Fatal("missing webhook URL must be a config error")
	}
}

func TestYandexDiskTableLiveByEnv(t *testing.T) {
	env := map[string]string{"RU_SHEETS_PROVIDER": "yandex_disk_table", "RU_SHEETS_YADISK_TOKEN": "t", "RU_SHEETS_SHEET_ID": "app:/leads/leads.xlsx"}
	ps, err := LoadProfiles(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, EditionRU)
	if err != nil {
		t.Fatal(err)
	}
	if ps["ru"].Modes.Sheets != "live" || ps["ru"].sheetID != "app:/leads/leads.xlsx" || ps["ru_b24"].Modes.Sheets != "mock" {
		t.Fatalf("modes: ru=%s ru_b24=%s id=%s", ps["ru"].Modes.Sheets, ps["ru_b24"].Modes.Sheets, ps["ru"].sheetID)
	}
}
