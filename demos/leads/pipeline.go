package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	crm "github.com/sedyukov/sedlab-modules/modules/crm/go"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	payments "github.com/sedyukov/sedlab-modules/modules/payments/go"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
)

// App wires sessions, profiles, the event hub and the pipeline.
type App struct {
	Profiles  map[string]*Profile
	Sessions  *Sessions
	Hub       *Hub
	PublicURL string // https://leads.demo.sedlab.dev — used in checkout links
	SelfURL   string // where webhooks are delivered in mock mode (http://127.0.0.1:8080)
	Retry     RetryPolicy
	HTTP      *http.Client
	now       func() time.Time
}

func NewApp(profiles map[string]*Profile, publicURL, selfURL string, maxSessions int) *App {
	hub := NewHub()
	return &App{
		Profiles: profiles, Hub: hub, Sessions: NewSessions(maxSessions, hub.Drop),
		PublicURL: strings.TrimRight(publicURL, "/"), SelfURL: strings.TrimRight(selfURL, "/"),
		Retry: defaultRetry, HTTP: &http.Client{Timeout: 10 * time.Second},
		now: time.Now,
	}
}

func (a *App) stack(s *Session, profile string) *Stack {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stacks[profile]
	if st == nil {
		st = a.Profiles[profile].NewStack(a.PublicURL + "/checkout/{id}?s=" + s.ID)
		s.stacks[profile] = st
	}
	return st
}

func (a *App) providerLabel(profile, service string) string {
	p := a.Profiles[profile]
	var name, m string
	switch service {
	case "crm":
		name, m = p.CRMName, p.Modes.CRM
	case "payments", "webhook":
		name, m = p.PayName, p.Modes.Payments
	case "sheets":
		name, m = p.SheetsName, p.Modes.Sheets
	case "notify":
		name, m = "Telegram/email", p.Modes.Notify
	default:
		return ""
	}
	if m == "mock" {
		return name + " · sandbox"
	}
	return name + " · live"
}

func (a *App) emit(s *Session, lead *Lead, ev Event) Event {
	if lead != nil {
		ev.LeadID = lead.ID
	}
	return a.Hub.Publish(s.ID, ev)
}

// step runs fn with retries, emitting start / retry / ok / error events, and records the step state on the lead.
func (a *App) step(ctx context.Context, s *Session, lead *Lead, name, service string, fn func(ctx context.Context) (string, map[string]any, error)) error {
	label := a.providerLabel(lead.Profile, service)
	if ch, ok := strings.CutPrefix(name, "notify."); ok {
		label = map[string]string{"telegram": "Telegram", "email": "Email"}[ch] + " · " + map[string]string{"mock": "sandbox", "live": "live"}[a.Profiles[lead.Profile].Modes.Notify]
	}
	setState := func(v int) { s.update(func() { lead.Steps[name] = v }) }
	p := a.Retry
	if p.Sleep == nil {
		p.Sleep = sleepCtx
	}
	var chaos bool
	s.update(func() { chaos = s.Chaos })
	for attempt := 1; ; attempt++ {
		start := a.now()
		var (
			msg  string
			data map[string]any
			err  error
		)
		if plan := chaosPlan[name]; chaos && attempt <= len(plan) && strings.HasPrefix(name, "notify.") {
			msg, data, err = fn(context.WithValue(ctx, chaosCtxKey{}, plan[attempt-1].Code))
		} else if chaos && attempt <= len(plan) {
			time.Sleep(60 * time.Millisecond)
			err = chaosErr{plan[attempt-1]}
		} else {
			msg, data, err = fn(ctx)
		}
		dur := a.now().Sub(start).Milliseconds()
		if err == nil {
			setState(1)
			a.emit(s, lead, Event{Step: name, Service: service, Status: "ok", Attempt: attempt, Duration: dur, Provider: label, Message: msg, Data: data})
			return nil
		}
		info := classifyAny(err)
		if !info.Retryable || attempt >= p.MaxAttempts {
			setState(3)
			a.emit(s, lead, Event{Step: name, Service: service, Status: "error", Attempt: attempt, Duration: dur, Provider: label,
				Message: info.Code + ": " + info.Message, Data: map[string]any{"error": info}})
			return err
		}
		d := p.delay(attempt, info)
		setState(2)
		a.emit(s, lead, Event{Step: name, Service: service, Status: "retry", Attempt: attempt, Duration: dur, Provider: label,
			Message: fmt.Sprintf("%s: %s — retry in %d ms", info.Code, info.Message, d.Milliseconds()),
			Data:    map[string]any{"error": info, "retry_in_ms": d.Milliseconds()}})
		if serr := p.Sleep(ctx, d); serr != nil {
			return err
		}
	}
}

// LeadInput is a validated form submission.
type LeadInput struct {
	SubmissionID string
	Profile      string
	Name         string
	Email        string
	Phone        string
	Company      string
	Message      string
	Plan         string
	Lang         string
}

// Submit registers the lead (deduping by submission id) and runs the pipeline in the background.
// Returns the lead and whether it was a duplicate submission.
func (a *App) Submit(s *Session, in LeadInput) (*Lead, bool) {
	prof := a.Profiles[in.Profile]
	plan, _ := prof.Plan(in.Plan)
	var lead *Lead
	dup := false
	s.update(func() {
		if id, ok := s.bySubmit[in.SubmissionID]; ok {
			lead, dup = s.leads[id], true
			return
		}
		s.nLead++
		lead = &Lead{ID: fmt.Sprintf("L-%03d", s.nLead), SubmissionID: in.SubmissionID, Profile: in.Profile,
			CreatedAt: a.now().UTC().Format(time.RFC3339), Name: in.Name, Email: in.Email, Phone: in.Phone,
			Company: in.Company, Message: in.Message, Plan: plan.Key, Amount: Money{plan.Amount, prof.Currency},
			Status: "processing", Steps: map[string]int{}}
		s.leads[lead.ID] = lead
		s.bySubmit[in.SubmissionID] = lead.ID
		s.order = append(s.order, lead.ID)
	})
	if dup {
		a.emit(s, lead, Event{Step: "form.submit", Service: "form", Status: "duplicate",
			Message: "Same submission_id received again — ignored, nothing is created twice",
			Data:    map[string]any{"submission_id": in.SubmissionID}})
		return lead, true
	}
	a.emit(s, lead, Event{Step: "form.submit", Service: "form", Status: "ok", Message: "Form submitted: " + lead.Name,
		Data: map[string]any{"submission_id": in.SubmissionID, "plan": plan.Key, "amount": lead.Amount, "profile": in.Profile}})
	s.update(func() { lead.Steps["form.submit"] = 1 })
	go a.process(s, lead)
	return lead, false
}

func (a *App) process(s *Session, lead *Lead) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t0 := a.now()
	st := a.stack(s, lead.Profile)
	prof := a.Profiles[lead.Profile]
	fail := func() {
		s.update(func() { lead.Status = "failed" })
		a.emit(s, lead, Event{Step: "pipeline", Service: "system", Status: "error", Message: "Pipeline stopped — the failed step can be retried safely (idempotency keys)"})
	}

	// 1. CRM: contact (dedup by email → phone)
	var contact *crm.Contact
	if err := a.step(ctx, s, lead, "crm.upsert_contact", "crm", func(ctx context.Context) (string, map[string]any, error) {
		c, err := st.CRM.UpsertContact(ctx, crm.UpsertContactInput{Name: lead.Name, Email: lead.Email, Phone: lead.Phone,
			Custom: map[string]any{"company": lead.Company}})
		if err != nil {
			return "", nil, err
		}
		contact = c
		verb := "Contact matched (existing)"
		if c.Created {
			verb = "Contact created"
		}
		return fmt.Sprintf("%s: %s", verb, c.ID), map[string]any{"contact": c}, nil
	}); err != nil {
		fail()
		return
	}
	s.update(func() { lead.ContactID = contact.ID })

	// 2. CRM: deal (idempotency key = form submission id)
	var deal *crm.Deal
	if err := a.step(ctx, s, lead, "crm.create_deal", "crm", func(ctx context.Context) (string, map[string]any, error) {
		d, err := st.CRM.CreateDeal(ctx, crm.CreateDealInput{Title: fmt.Sprintf("%s — %s (%s)", lead.ID, lead.Name, lead.Plan),
			ContactID: contact.ID, Amount: &crm.Money{Value: lead.Amount.Value, Currency: lead.Amount.Currency},
			Source: "website-form", IdempotencyKey: "deal-" + lead.SubmissionID})
		if err != nil {
			return "", nil, err
		}
		deal = d
		return fmt.Sprintf("Deal %s created, stage «%s»", d.ID, d.StageName), map[string]any{"deal": d, "idempotency_key": "deal-" + lead.SubmissionID}, nil
	}); err != nil {
		fail()
		return
	}
	s.update(func() { lead.DealID, lead.DealStage, lead.DealURL = deal.ID, deal.StageName, deal.URL })
	a.deliverCRMWebhook(ctx, s, lead, st)

	// 3. Google Sheet row (idempotent append)
	if err := a.step(ctx, s, lead, "sheets.append_rows", "sheets", func(ctx context.Context) (string, map[string]any, error) {
		if err := st.ensureHeader(ctx); err != nil {
			return "", nil, err
		}
		r, err := st.Sheets.AppendRows(ctx, sheets.AppendRowsInput{SheetID: st.SheetID, Tab: st.SheetTab, IdempotencyKey: "row-" + lead.SubmissionID,
			Rows: [][]any{{lead.ID, a.now().UTC().Format("2006-01-02 15:04"), lead.Name, lead.Email, lead.Phone, lead.Company,
				lead.Plan, formatMoney(lead.Amount), deal.ID, "New", "—"}}})
		if err != nil {
			return "", nil, err
		}
		return "Row appended: " + r.UpdatedRange, map[string]any{"result": r}, nil
	}); err != nil {
		fail()
		return
	}

	// 4. Notifications (Telegram for RU; Telegram + email for international)
	for _, ch := range prof.NotifyChans {
		ch := ch
		if err := a.step(ctx, s, lead, "notify."+ch, "notify", func(ctx context.Context) (string, map[string]any, error) {
			text := fmt.Sprintf("🆕 New lead %s\n%s · %s · %s\nPlan: %s — %s\nDeal: %s", lead.ID, lead.Name, nz(lead.Phone), nz(lead.Email),
				lead.Plan, formatMoney(lead.Amount), deal.ID)
			if lead.Profile == "ru" {
				text = fmt.Sprintf("🆕 Новая заявка %s\n%s · %s · %s\nТариф: %s — %s\nСделка: %s", lead.ID, lead.Name, nz(lead.Phone), nz(lead.Email),
					lead.Plan, formatMoney(lead.Amount), deal.ID)
			}
			r, err := st.Notify.Send(ctx, notify.SendInput{Channel: ch, To: prof.recipients[ch], Text: text, Subject: "New lead " + lead.ID, IdempotencyKey: "new-" + ch + "-" + lead.SubmissionID, Metadata: notifyMeta(ctx, lead)})
			if err != nil {
				return "", nil, err
			}
			return fmt.Sprintf("Sent to %s %s (%s)", ch, prof.recipients[ch], r.ID), map[string]any{"result": r, "text": text}, nil
		}); err != nil {
			// a notification failure must not lose the lead — it is already in CRM and the sheet
			a.emit(s, lead, Event{Step: "notify." + ch, Service: "notify", Status: "info", Message: "Notification skipped after retries; lead is safe in CRM + sheet"})
		}
	}

	// 5. Payment link (idempotency key → a retried request never creates a second payment)
	var pay *payments.Payment
	if err := a.step(ctx, s, lead, "payments.create_payment", "payments", func(ctx context.Context) (string, map[string]any, error) {
		p, err := st.Pay.CreatePayment(ctx, payments.CreatePaymentInput{
			Amount: payments.Money{Value: lead.Amount.Value, Currency: lead.Amount.Currency}, Description: fmt.Sprintf("Order %s (%s)", lead.ID, lead.Plan),
			IdempotencyKey: "pay-" + lead.SubmissionID, ReturnURL: a.PublicURL + "/?paid=" + lead.ID,
			Customer: customer(lead), Metadata: map[string]string{"lead_id": lead.ID, "session": s.ID, "deal_id": deal.ID, "profile": lead.Profile}})
		if err != nil {
			return "", nil, err
		}
		pay = p
		return fmt.Sprintf("Payment %s created (%s), link ready", p.ID, p.Status), map[string]any{"payment": p}, nil
	}); err != nil {
		fail()
		return
	}
	s.update(func() {
		lead.PaymentID, lead.PaymentStatus, lead.ConfirmationURL, lead.Status = pay.ID, pay.Status, pay.ConfirmationURL, "awaiting_payment"
	})
	a.emit(s, lead, Event{Step: "pipeline", Service: "system", Status: "ok",
		Message: fmt.Sprintf("Lead processed in %d ms — waiting for payment", a.now().Sub(t0).Milliseconds())})
}

func customer(l *Lead) *payments.Customer {
	if l.Email == "" && l.Phone == "" {
		return nil
	}
	return &payments.Customer{Email: l.Email, Phone: l.Phone}
}

func nz(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func formatMoney(m Money) string {
	major := fmt.Sprintf("%d", m.Value/100)
	if m.Value%100 != 0 {
		major = fmt.Sprintf("%d.%02d", m.Value/100, m.Value%100)
	}
	switch m.Currency {
	case "RUB":
		return major + " ₽"
	case "USD":
		return "$" + major
	}
	return major + " " + m.Currency
}

// deliverCRMWebhook posts the mock CRM's freshly emitted signed webhook to our own endpoint,
// exactly like a real CRM would call back (live CRMs call the public URL themselves).
func (a *App) deliverCRMWebhook(ctx context.Context, s *Session, lead *Lead, st *Stack) {
	if st.crmMock == nil {
		return
	}
	w, err := st.crmMock.LastWebhook()
	if err != nil {
		return
	}
	a.post(ctx, a.SelfURL+"/webhooks/crm/"+lead.Profile+"?s="+s.ID, w.Headers, []byte(w.Body))
}

func (a *App) post(ctx context.Context, url string, headers map[string]string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// ---------------------------------------------------------------------------------------------
// Payment side: the customer pays (mock checkout) → signed webhook → our endpoint → CRM / sheet / notify.

// MockPay simulates the customer finishing checkout and delivers the provider's signed webhook to us.
func (a *App) MockPay(ctx context.Context, s *Session, lead *Lead, outcome string) error {
	st := a.stack(s, lead.Profile)
	if st.payMock == nil {
		return fmt.Errorf("live payments: pay on the provider's checkout page")
	}
	w, err := st.payMock.MockComplete(ctx, lead.PaymentID, outcome)
	if err != nil {
		return err
	}
	s.update(func() { lead.lastWebhook = &storedWebhook{Headers: w.Headers, Body: w.Body} })
	a.emit(s, lead, Event{Step: "payments.checkout", Service: "payments", Status: "info", Provider: a.providerLabel(lead.Profile, "payments"),
		Message: fmt.Sprintf("Customer finished checkout: %s → provider sends signed webhook %s", outcome, w.EventType)})
	_, err = a.post(ctx, a.SelfURL+"/webhooks/payments/"+lead.Profile, w.Headers, []byte(w.Body))
	return err
}

// ReplayWebhook re-sends the last payment webhook byte-for-byte (providers do retry deliveries).
func (a *App) ReplayWebhook(ctx context.Context, s *Session, lead *Lead) error {
	var w *storedWebhook
	s.update(func() { w = lead.lastWebhook })
	if w == nil {
		return fmt.Errorf("no webhook yet — pay first")
	}
	a.emit(s, lead, Event{Step: "webhook.replay", Service: "webhook", Status: "info", Message: "Provider re-delivers the same webhook (network retry)"})
	_, err := a.post(ctx, a.SelfURL+"/webhooks/payments/"+lead.Profile, w.Headers, []byte(w.Body))
	return err
}

// ForgeWebhook sends a tampered webhook (amount changed, old signature kept) — must be rejected.
func (a *App) ForgeWebhook(ctx context.Context, s *Session, lead *Lead) error {
	var w *storedWebhook
	s.update(func() { w = lead.lastWebhook })
	body := fmt.Sprintf(`{"id":"evt_forged_%d","type":"payment.succeeded","created_at":%q,"payment":{"id":%q,"status":"succeeded","amount":{"value":1,"currency":%q},"metadata":{"lead_id":%q,"session":%q}}}`,
		a.now().UnixNano(), a.now().UTC().Format(time.RFC3339), lead.PaymentID, lead.Amount.Currency, lead.ID, s.ID)
	headers := map[string]string{"Content-Type": "application/json", payments.MockSignatureHeader: "t=1700000000,v1=deadbeef"}
	if w != nil {
		for k, v := range w.Headers {
			headers[k] = v // reuse a real signature from another body
		}
	}
	a.emit(s, lead, Event{Step: "webhook.forge", Service: "webhook", Status: "info", Message: "Attacker posts a fake “payment.succeeded” (amount 0.01, stolen signature)"})
	_, err := a.post(ctx, a.SelfURL+"/webhooks/payments/"+lead.Profile, headers, []byte(body))
	return err
}

// routeHint reads session/lead from an UNVERIFIED body only to pick the sandbox; verification follows.
func routeHint(body []byte) (sid, leadID string) {
	var v struct {
		Payment *struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"payment"`
		Refund *struct {
			PaymentID string `json:"payment_id"`
		} `json:"refund"`
	}
	if json.Unmarshal(body, &v) == nil && v.Payment != nil {
		return v.Payment.Metadata["session"], v.Payment.Metadata["lead_id"]
	}
	return "", ""
}

// HandlePaymentWebhook verifies + dedupes the webhook and advances the deal. Returns the HTTP status to answer.
func (a *App) HandlePaymentWebhook(ctx context.Context, profile string, headers map[string]string, body []byte) int {
	prof := a.Profiles[profile]
	if prof == nil {
		return http.StatusNotFound
	}
	var parser payments.Provider = prof.shared.Pay
	var s *Session
	var lead *Lead
	if parser == nil { // mock mode: route to the visitor's sandbox, whose mock verifies the signature
		sid, lid := routeHint(body)
		if s = a.Sessions.Peek(sid); s == nil {
			return http.StatusNotFound
		}
		parser = a.stack(s, profile).Pay
		lead = s.lead(lid)
	}
	t0 := a.now()
	ev, err := parser.ParseWebhook(ctx, headers, body)
	dur := a.now().Sub(t0).Milliseconds()
	label := a.providerLabel(profile, "webhook")
	if err != nil {
		info := classify(err)
		if s != nil {
			a.emit(s, lead, Event{Step: "webhook.payment", Service: "webhook", Status: "rejected", Duration: dur, Provider: label,
				Message: "Webhook rejected → HTTP 401: " + info.Code + " (" + info.Message + ")", Data: map[string]any{"error": info}})
		}
		if info.Code == "signature_invalid" {
			return http.StatusUnauthorized
		}
		return http.StatusBadRequest
	}
	if s == nil && ev.Payment != nil { // live: route by verified metadata
		s = a.Sessions.Peek(ev.Payment.Metadata["session"])
		if s != nil {
			lead = s.lead(ev.Payment.Metadata["lead_id"])
		}
	}
	if s == nil || lead == nil {
		return http.StatusOK // not ours (or session expired) — acknowledge so the provider stops retrying
	}
	if ev.Duplicate {
		a.emit(s, lead, Event{Step: "webhook.payment", Service: "webhook", Status: "duplicate", Duration: dur, Provider: label,
			Message: fmt.Sprintf("Duplicate webhook %s → HTTP 200, already processed, nothing changed", ev.ID), Data: map[string]any{"event_id": ev.ID}})
		return http.StatusOK
	}
	a.emit(s, lead, Event{Step: "webhook.payment", Service: "webhook", Status: "ok", Duration: dur, Provider: label,
		Message: fmt.Sprintf("Webhook %s verified (signature ✓, first delivery) → %s", ev.ID, ev.Type), Data: map[string]any{"event": ev}})
	go a.onPayment(s, lead, ev)
	return http.StatusOK
}

func (a *App) onPayment(s *Session, lead *Lead, ev *payments.WebhookEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	st := a.stack(s, lead.Profile)
	var stage, status, sheetStatus string
	switch ev.Type {
	case payments.EventPaymentSucceeded:
		stage, status, sheetStatus = "won", "paid", "Won"
	case payments.EventPaymentCanceled, payments.EventPaymentFailed:
		stage, status, sheetStatus = "lost", "canceled", "Lost"
	default:
		return
	}
	payStatus := ev.Type[strings.Index(ev.Type, ".")+1:]
	s.update(func() { lead.PaymentStatus, lead.Status = payStatus, status })

	var deal *crm.Deal
	if err := a.step(ctx, s, lead, "crm.update_deal", "crm", func(ctx context.Context) (string, map[string]any, error) {
		d, err := st.CRM.UpdateDeal(ctx, crm.UpdateDealInput{ID: lead.DealID, Stage: stage})
		if err != nil {
			return "", nil, err
		}
		deal = d
		return fmt.Sprintf("Deal %s → stage «%s»", d.ID, d.StageName), map[string]any{"deal": d}, nil
	}); err == nil {
		s.update(func() { lead.DealStage = deal.StageName })
		a.deliverCRMWebhook(ctx, s, lead, st)
	}
	_ = a.step(ctx, s, lead, "crm.add_note", "crm", func(ctx context.Context) (string, map[string]any, error) {
		n, err := st.CRM.AddNote(ctx, crm.AddNoteInput{Entity: "deal", ID: lead.DealID,
			Text: fmt.Sprintf("Payment %s: %s, %s (webhook %s)", lead.PaymentID, payStatus, formatMoney(lead.Amount), ev.ID)})
		if err != nil {
			return "", nil, err
		}
		return "Note added: " + n.ID, map[string]any{"note": n}, nil
	})
	_ = a.step(ctx, s, lead, "sheets.update_row", "sheets", func(ctx context.Context) (string, map[string]any, error) {
		found, err := st.Sheets.FindRows(ctx, sheets.FindRowsInput{SheetID: st.SheetID, Tab: st.SheetTab, Column: "Lead ID", Value: lead.ID})
		if err != nil {
			return "", nil, err
		}
		if len(found.Rows) == 0 {
			return "", nil, fmt.Errorf("row for %s not found", lead.ID)
		}
		row := found.Rows[0]
		vals := make([]any, len(SheetHeader))
		for i := range vals {
			if i < len(row.Values) {
				vals[i] = row.Values[i]
			}
		}
		vals[9], vals[10] = sheetStatus, payStatus+" · "+lead.PaymentID
		r, err := st.Sheets.UpdateRow(ctx, sheets.UpdateRowInput{SheetID: st.SheetID, Tab: st.SheetTab, RowIndex: row.Index, Values: vals})
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Row %d updated: %s", row.Index, r.UpdatedRange), map[string]any{"result": r}, nil
	})
	prof := a.Profiles[lead.Profile]
	ch := prof.NotifyChans[0]
	_ = a.step(ctx, s, lead, "notify."+ch, "notify", func(ctx context.Context) (string, map[string]any, error) {
		text := fmt.Sprintf("💰 %s paid: %s — %s. Deal %s → %s", lead.ID, lead.Name, formatMoney(lead.Amount), lead.DealID, stage)
		if lead.Profile == "ru" {
			text = fmt.Sprintf("💰 %s оплачена: %s — %s. Сделка %s → %s", lead.ID, lead.Name, formatMoney(lead.Amount), lead.DealID, stage)
		}
		if status != "paid" {
			text = fmt.Sprintf("❌ %s: payment %s. Deal %s → lost", lead.ID, payStatus, lead.DealID)
		}
		r, err := st.Notify.Send(ctx, notify.SendInput{Channel: ch, To: prof.recipients[ch], Text: text, Subject: "Payment " + lead.ID, IdempotencyKey: "pay-" + ch + "-" + ev.ID, Metadata: notifyMeta(ctx, lead)})
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Sent to %s %s (%s)", ch, prof.recipients[ch], r.ID), map[string]any{"result": r, "text": text}, nil
	})
	a.emit(s, lead, Event{Step: "pipeline", Service: "system", Status: "ok", Message: "Payment processed: CRM, sheet and team are up to date"})
}

// HandleCRMWebhook verifies a CRM webhook (mock: HMAC; for amoCRM the URL token is the secret).
func (a *App) HandleCRMWebhook(ctx context.Context, profile, sid string, headers map[string]string, body []byte) int {
	s := a.Sessions.Peek(sid)
	if s == nil || a.Profiles[profile] == nil {
		return http.StatusNotFound
	}
	st := a.stack(s, profile)
	ev, err := st.CRM.ParseWebhook(ctx, headers, body)
	label := a.providerLabel(profile, "crm")
	if err != nil {
		info := classify(err)
		a.emit(s, nil, Event{Step: "webhook.crm", Service: "webhook", Status: "rejected", Provider: label, Message: "CRM webhook rejected: " + info.Code})
		return http.StatusUnauthorized
	}
	st2 := "ok"
	msg := fmt.Sprintf("CRM webhook %s: %s %s (signature ✓)", ev.ID, ev.Type, ev.EntityID)
	if ev.Duplicate {
		st2, msg = "duplicate", fmt.Sprintf("CRM webhook %s already seen — skipped", ev.ID)
	}
	var lead *Lead
	s.update(func() {
		for _, l := range s.leads {
			if l.DealID == ev.EntityID && l.Profile == profile {
				lead = l
			}
		}
	})
	a.emit(s, lead, Event{Step: "webhook.crm", Service: "webhook", Status: st2, Provider: label, Message: msg, Data: map[string]any{"event": ev}})
	return http.StatusOK
}

func notifyMeta(ctx context.Context, lead *Lead) map[string]string {
	m := map[string]string{"lead_id": lead.ID, "event": "lead"}
	if c := chaosCode(ctx); c != "" {
		m[notify.MockFailKey] = c
	}
	return m
}
