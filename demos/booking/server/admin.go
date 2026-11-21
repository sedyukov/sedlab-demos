package main

import (
	"net/http"
	"strings"
	"time"
)

// GET /api/admin/overview?date=YYYY-MM-DD → day schedule (masters × bookings) + KPIs
func (a *App) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	day := a.now().In(a.cfg.TZ)
	if d := r.URL.Query().Get("date"); d != "" {
		if t, err := time.ParseInLocation("2006-01-02", d, a.cfg.TZ); err == nil {
			day = t
		}
	}
	from, to := DayRange(day)
	bs, err := a.store.BookingsBetween(from, to)
	if err != nil {
		a.fail(w, err)
		return
	}
	ms, err := a.store.Masters(false)
	if err != nil {
		a.fail(w, err)
		return
	}
	views := []map[string]any{}
	for _, b := range bs {
		if b.Status == "cancelled" || b.Status == "expired" {
			continue
		}
		views = append(views, a.bookingView(b))
	}
	// KPIs over the next 7 days and revenue (prepayments) over the last 30 days
	now := a.now().Unix()
	var week, rubPaid, starsPaid, pending int64
	upcoming, _ := a.store.queryBookings(`status='confirmed' AND start_at>=? AND start_at<?`, now, now+7*86400)
	week = int64(len(upcoming))
	paid, _ := a.store.queryBookings(`paid_at>=? AND refunded=0 AND status IN ('confirmed','completed')`, now-30*86400)
	for _, b := range paid {
		switch b.PayCurrency {
		case "RUB":
			rubPaid += b.PayAmount
		case "XTR":
			starsPaid += b.PayAmount
		}
	}
	pend, _ := a.store.queryBookings(`status='pending' AND hold_until>?`, now)
	pending = int64(len(pend))
	writeJSON(w, 200, map[string]any{
		"date": day.Format("2006-01-02"), "tz": a.cfg.TZ.String(), "masters": ms, "bookings": views,
		"kpi": map[string]any{"week_bookings": week, "prepaid_rub_30d": rubPaid, "prepaid_stars_30d": starsPaid, "pending_holds": pending},
	})
}

// GET /api/admin/bookings?from=YYYY-MM-DD&days=N&status=
func (a *App) handleAdminBookings(w http.ResponseWriter, r *http.Request) {
	start := a.now().In(a.cfg.TZ).AddDate(0, 0, -7)
	if d := r.URL.Query().Get("from"); d != "" {
		if t, err := time.ParseInLocation("2006-01-02", d, a.cfg.TZ); err == nil {
			start = t
		}
	}
	days := qInt(r, "days")
	if days <= 0 || days > 92 {
		days = 30
	}
	from, _ := DayRange(start)
	to := from + days*86400
	bs, err := a.store.BookingsBetween(from, to)
	if err != nil {
		a.fail(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	out := []map[string]any{}
	for _, b := range bs {
		if b.Status == "expired" || (status != "" && b.Status != status) {
			continue
		}
		out = append(out, a.bookingView(b))
	}
	writeJSON(w, 200, map[string]any{"bookings": out})
}

func (a *App) handleAdminCancel(w http.ResponseWriter, r *http.Request) {
	b, err := a.store.Booking(pathID(r))
	if err != nil {
		a.fail(w, err)
		return
	}
	var in struct {
		Refund bool `json:"refund"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if b.Status != "confirmed" && b.Status != "pending" {
		writeErr(w, 409, "conflict", "booking is "+b.Status)
		return
	}
	nb, err := a.cancelBooking(r.Context(), b, in.Refund, "owner")
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, a.bookingView(nb))
}

func (a *App) handleAdminComplete(w http.ResponseWriter, r *http.Request) {
	ok, err := a.store.SetStatus(pathID(r), []string{"confirmed"}, "completed")
	if err != nil {
		a.fail(w, err)
		return
	}
	if !ok {
		writeErr(w, 409, "conflict", "only confirmed bookings can be completed")
		return
	}
	b, _ := a.store.Booking(pathID(r))
	writeJSON(w, 200, a.bookingView(b))
}

func (a *App) handleAdminServices(w http.ResponseWriter, r *http.Request) {
	s, err := a.store.Services(false)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"services": s})
}

func (a *App) handleAdminSaveService(w http.ResponseWriter, r *http.Request) {
	var v Service
	if !readJSON(w, r, &v) {
		return
	}
	v.NameRU, v.NameEN = strings.TrimSpace(v.NameRU), strings.TrimSpace(v.NameEN)
	if v.NameEN == "" {
		v.NameEN = v.NameRU
	}
	if v.NameRU == "" {
		v.NameRU = v.NameEN
	}
	if v.NameRU == "" || len(v.NameRU) > 120 || len(v.NameEN) > 120 || v.DurationMin < 15 || v.DurationMin > 480 || v.DurationMin%15 != 0 ||
		v.PriceRub < 100 || v.PriceRub > 100_000_000 || v.PriceStars < 1 || v.PriceStars > 100_000 {
		writeErr(w, 400, "invalid_argument", "check name, duration (15–480, step 15) and prices")
		return
	}
	if err := a.store.SaveService(&v); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (a *App) handleAdminMasters(w http.ResponseWriter, r *http.Request) {
	m, err := a.store.Masters(false)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"masters": m})
}

func (a *App) handleAdminSaveMaster(w http.ResponseWriter, r *http.Request) {
	var m Master
	if !readJSON(w, r, &m) {
		return
	}
	m.NameRU, m.NameEN = strings.TrimSpace(m.NameRU), strings.TrimSpace(m.NameEN)
	if m.NameEN == "" {
		m.NameEN = m.NameRU
	}
	if m.NameRU == "" {
		m.NameRU = m.NameEN
	}
	if m.NameRU == "" || len(m.NameRU) > 80 || len(m.NameEN) > 80 || len(m.RoleRU) > 80 || len(m.RoleEN) > 80 || len(m.Hours) > 7 || len(m.ServiceIDs) > 100 {
		writeErr(w, 400, "invalid_argument", "check name and fields")
		return
	}
	if !strings.HasPrefix(m.Color, "#") || len(m.Color) != 7 {
		m.Color = "#7c5cff"
	}
	for _, h := range m.Hours {
		if h.Weekday < 0 || h.Weekday > 6 || h.StartMin < 0 || h.EndMin > 24*60 || h.StartMin >= h.EndMin {
			writeErr(w, 400, "invalid_argument", "bad working hours")
			return
		}
	}
	if err := a.store.SaveMaster(&m); err != nil {
		a.fail(w, err)
		return
	}
	nm, _ := a.store.Master(m.ID)
	writeJSON(w, 200, nm)
}

// POST /api/admin/reset — demo only: restore sample data.
func (a *App) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	if !sessionFrom(r).Emulated || !a.cfg.Emulator {
		writeErr(w, 403, "permission_denied", "reset is available in the demo only")
		return
	}
	if err := a.ResetDemo(); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
