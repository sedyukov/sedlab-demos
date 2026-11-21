package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) handleCatalog(w http.ResponseWriter, r *http.Request) {
	svcs, err := a.store.Services(true)
	if err != nil {
		a.fail(w, err)
		return
	}
	ms, err := a.store.Masters(true)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"services": svcs, "masters": ms, "deposit_percent": a.cfg.DepositPercent, "pay_methods": a.cfg.PayMethods})
}

// candidates returns the masters that can do the service (one if masterID != 0).
func (a *App) candidates(serviceID, masterID int64) ([]Master, Service, error) {
	svc, err := a.store.Service(serviceID)
	if err != nil || !svc.Active {
		return nil, svc, ErrNotFound
	}
	ms, err := a.store.Masters(true)
	if err != nil {
		return nil, svc, err
	}
	var out []Master
	for _, m := range ms {
		if masterID != 0 && m.ID != masterID {
			continue
		}
		for _, sid := range m.ServiceIDs {
			if sid == serviceID {
				out = append(out, m)
			}
		}
	}
	return out, svc, nil
}

type slotOut struct {
	Slot
	MasterID int64 `json:"master_id"`
}

func (a *App) daySlots(day time.Time, masters []Master, svc Service) ([]slotOut, error) {
	from, to := DayRange(day)
	now := a.now()
	notBefore := now.Add(time.Duration(a.cfg.LeadMinutes) * time.Minute).Unix()
	seen := map[int64]bool{}
	out := []slotOut{}
	for _, m := range masters {
		busy, err := a.store.BusyIntervals(m.ID, from, to, now.Unix())
		if err != nil {
			return nil, err
		}
		for _, s := range FreeSlots(SlotParams{Day: day, Hours: m.Hours, DurationMin: svc.DurationMin, StepMin: 30, Busy: busy, NotBefore: notBefore}) {
			if !seen[s.Start] { // "any master": first master free at this time
				seen[s.Start] = true
				out = append(out, slotOut{s, m.ID})
			}
		}
	}
	sortSlots(out)
	return out, nil
}

func sortSlots(s []slotOut) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Start < s[j-1].Start; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func qInt(r *http.Request, k string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(k), 10, 64)
	return v
}

// GET /api/days?service=&master= → next 14 days with free-slot counts
func (a *App) handleDays(w http.ResponseWriter, r *http.Request) {
	masters, svc, err := a.candidates(qInt(r, "service"), qInt(r, "master"))
	if err != nil {
		a.fail(w, err)
		return
	}
	today := a.now().In(a.cfg.TZ)
	type day struct {
		Date string `json:"date"`
		Free int    `json:"free"`
	}
	out := []day{}
	for i := 0; i < 14; i++ {
		d := today.AddDate(0, 0, i)
		s, err := a.daySlots(d, masters, svc)
		if err != nil {
			a.fail(w, err)
			return
		}
		out = append(out, day{d.Format("2006-01-02"), len(s)})
	}
	writeJSON(w, 200, map[string]any{"days": out})
}

// GET /api/slots?service=&master=&date=YYYY-MM-DD
func (a *App) handleSlots(w http.ResponseWriter, r *http.Request) {
	masters, svc, err := a.candidates(qInt(r, "service"), qInt(r, "master"))
	if err != nil {
		a.fail(w, err)
		return
	}
	day, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("date"), a.cfg.TZ)
	if err != nil {
		writeErr(w, 400, "invalid_argument", "date must be YYYY-MM-DD")
		return
	}
	slots, err := a.daySlots(day, masters, svc)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"slots": slots})
}

// POST /api/bookings {service_id, master_id, start, name, phone} → pending booking holding the slot
func (a *App) handleCreateBooking(w http.ResponseWriter, r *http.Request) {
	s := sessionFrom(r)
	var in struct {
		ServiceID int64  `json:"service_id"`
		MasterID  int64  `json:"master_id"`
		Start     int64  `json:"start"`
		Name      string `json:"name"`
		Phone     string `json:"phone"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name, in.Phone = strings.TrimSpace(in.Name), strings.TrimSpace(in.Phone)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 60 || utf8.RuneCountInString(in.Phone) > 30 {
		writeErr(w, 400, "invalid_argument", "name is required (≤60 chars)")
		return
	}
	masters, svc, err := a.candidates(in.ServiceID, in.MasterID)
	if err != nil || len(masters) == 0 {
		writeErr(w, 404, "not_found", "service/master not found")
		return
	}
	// the start must be one of the computed free slots (working hours, grid, lead time)
	day := time.Unix(in.Start, 0).In(a.cfg.TZ)
	slots, err := a.daySlots(day, masters, svc)
	if err != nil {
		a.fail(w, err)
		return
	}
	ok := false
	for _, sl := range slots {
		if sl.Start == in.Start && sl.MasterID == in.MasterID {
			ok = true
		}
	}
	if !ok {
		writeErr(w, 409, "conflict", "slot is not available")
		return
	}
	// demo hygiene: one unpaid hold per user
	if pend, _ := a.store.queryBookings(`user_id=? AND status='pending' AND hold_until>?`, s.UserID, a.now().Unix()); len(pend) >= 3 {
		writeErr(w, 429, "rate_limited", "too many unpaid bookings: pay or wait 15 minutes")
		return
	}
	if !a.cfg.PDAllowed {
		in.Phone = "" // host outside RU: never store personal data (152-FZ localisation)
	}
	now := a.now().Unix()
	b := &Booking{UserID: s.UserID, ClientName: in.Name, ClientPhone: in.Phone, ServiceID: svc.ID, MasterID: in.MasterID,
		StartAt: in.Start, EndAt: in.Start + int64(svc.DurationMin)*60, HoldUntil: now + int64(a.cfg.HoldMinutes)*60,
		Lang: a.langFor(s.UserID)}
	if err := a.store.CreatePending(b, now); err != nil {
		if errors.Is(err, ErrSlotTaken) {
			writeErr(w, 409, "conflict", "slot was just taken")
			return
		}
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, a.bookingView(*b))
}

func (a *App) ownBooking(w http.ResponseWriter, r *http.Request) (Booking, bool) {
	b, err := a.store.Booking(pathID(r))
	s := sessionFrom(r)
	if err != nil || (b.UserID != s.UserID && s.Role != "owner") {
		writeErr(w, 404, "not_found", "booking not found")
		return b, false
	}
	return b, true
}

func (a *App) handleGetBooking(w http.ResponseWriter, r *http.Request) {
	if b, ok := a.ownBooking(w, r); ok {
		writeJSON(w, 200, a.bookingView(b))
	}
}

func (a *App) handleMyBookings(w http.ResponseWriter, r *http.Request) {
	bs, err := a.store.UserBookings(sessionFrom(r).UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(bs))
	for _, b := range bs {
		out = append(out, a.bookingView(b))
	}
	writeJSON(w, 200, map[string]any{"bookings": out})
}

func (a *App) handleLang(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Lang string `json:"lang"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// Client cancel: free cancellation (with prepayment refund) until 24h before the visit.
func (a *App) handleClientCancel(w http.ResponseWriter, r *http.Request) {
	b, ok := a.ownBooking(w, r)
	if !ok {
		return
	}
	refund := b.StartAt-a.now().Unix() >= 24*3600
	if b.Status == "pending" {
		refund = false
	} else if b.Status != "confirmed" {
		writeErr(w, 409, "conflict", "booking can't be cancelled")
		return
	}
	nb, err := a.cancelBooking(r.Context(), b, refund, "client")
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, a.bookingView(nb))
}

// bookingView adds display fields for the UI.
func (a *App) bookingView(b Booking) map[string]any {
	svc, _ := a.store.Service(b.ServiceID)
	m, _ := a.store.Master(b.MasterID)
	return map[string]any{
		"id": b.ID, "status": b.Status, "start_at": b.StartAt, "end_at": b.EndAt,
		"service_id": b.ServiceID, "master_id": b.MasterID,
		"service":     map[string]string{"ru": svc.NameRU, "en": svc.NameEN},
		"master":      map[string]string{"ru": m.NameRU, "en": m.NameEN, "color": m.Color},
		"client_name": b.ClientName, "client_phone": b.ClientPhone, "user_id": b.UserID,
		"pay_method": b.PayMethod, "pay_currency": b.PayCurrency, "pay_amount": b.PayAmount, "paid_at": b.PaidAt,
		"refunded": b.Refunded, "hold_until": b.HoldUntil,
		"price_rub": svc.PriceRub, "price_stars": svc.PriceStars, "price_usd": svc.PriceUSD,
		"card_currency": a.cardCurrency(), "card_price": a.cardPrice(svc), "deposit_card": a.deposit(a.cardPrice(svc), 100),
		"deposit_stars": a.deposit(svc.PriceStars, 1),
		"cancel_refundable": b.Status == "confirmed" && b.StartAt-a.now().Unix() >= 24*3600,
	}
}

// deposit = DepositPercent of the price, rounded up to `unit` (whole rubles / whole stars).
func (a *App) deposit(price, unit int64) int64 {
	d := price * a.cfg.DepositPercent / 100
	if rem := d % unit; rem != 0 {
		d += unit - rem
	}
	if d <= 0 {
		d = unit
	}
	return d
}
