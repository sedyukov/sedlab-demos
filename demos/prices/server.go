package main

import (
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	ai "github.com/sedyukov/sedlab-modules/modules/ai/go"
	mkit "github.com/sedyukov/sedlab-modules/modules/marketplaces/go/kit"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
)

//go:embed web
var webFS embed.FS

const (
	EditionRU   = "ru"
	EditionIntl = "intl"
	cookieName  = "prices_sid"
)

var errNotFound = errors.New("not_found")

// App holds the shared configuration.
type App struct {
	Edition   string // ru | intl | "" (dev: both languages)
	OtherURL  string
	Sessions  *Sessions
	LLM       *ai.Client
	LLMName   string // shown in the UI: GigaChat, YandexGPT, OpenAI, Anthropic
	Collector CollectorConfig
	Schedule  string // daily collection time shown in the UI, e.g. 09:00
	Log       *slog.Logger
	// RU edition: sign-in and daily live limits (service accounts). nil = always mock.
	Acc           *accounts.Client
	AuthWidgetURL string
	LLMResource   string // accounts resource of the live LLM: yandex-llm
}

// Guard: per-IP limits for a public demo.
type Guard struct {
	API     *mkit.KeyedLimiter
	Refresh *mkit.KeyedLimiter
	Global  *mkit.Limiter // all collections on the server
}

func DefaultGuard() *Guard {
	return &Guard{
		API:     mkit.NewKeyedLimiter(4, 40),
		Refresh: mkit.NewKeyedLimiter(1.0/2, 6), // 6 in a row, then one per 2 s
		Global:  mkit.NewLimiter(5, 20),
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" { // behind Caddy: the last entry is the real peer
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
	s.mux.HandleFunc("GET /api/config", s.config)
	s.mux.HandleFunc("GET /api/state", s.with(s.state))
	s.mux.HandleFunc("POST /api/refresh", s.with(s.refresh))
	s.mux.HandleFunc("POST /api/reset", s.reset)
	s.mux.HandleFunc("POST /api/products/{id}/apply", s.with(s.apply))
	s.mux.HandleFunc("POST /api/competitors", s.with(s.addCompetitor))
	s.mux.HandleFunc("POST /api/competitors/remove", s.with(s.removeCompetitor))
	s.mux.HandleFunc("POST /api/report/send", s.with(s.sendReport))
	s.mux.HandleFunc("POST /api/reviews", s.with(s.addReview))
	s.mux.HandleFunc("POST /api/reviews/{id}/draft", s.with(s.draft))
	s.mux.HandleFunc("POST /api/reviews/{id}/answer", s.with(s.answer))
	s.mux.HandleFunc("GET /api/export.csv", s.with(s.exportCSV))
	s.mux.HandleFunc("GET /api/export.xlsx", s.with(s.exportXLSX))
	s.mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	}))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://auth.demo.sedlab.dev; connect-src 'self' https://auth.demo.sedlab.dev; img-src 'self' data: https://auth.demo.sedlab.dev; style-src 'self' 'unsafe-inline'; frame-ancestors 'self' https://*.sedlab.dev")
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]string{"code": errCode, "message": msg}})
}

type handler func(w http.ResponseWriter, r *http.Request, sess *Session, lang string)

// with: rate limit + the visitor's session (cookie) + language; the session is locked for the handler.
func (s *Server) with(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Guard.API.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "5")
			apiErr(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		id := ""
		if c, err := r.Cookie(cookieName); err == nil {
			id = c.Value
		}
		sess, created, err := s.App.Sessions.Get(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "internal", "session")
			return
		}
		if created {
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: sess.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
				Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", MaxAge: 7 * 24 * 3600})
		}
		sess.mu.Lock()
		defer sess.mu.Unlock()
		h(w, r, sess, s.lang(r))
	}
}

func (s *Server) lang(r *http.Request) string {
	switch s.App.Edition {
	case EditionRU:
		return "ru"
	case EditionIntl:
		return "en"
	}
	if r.URL.Query().Get("lang") == "en" {
		return "en"
	}
	return "ru"
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	sheetsName := "Яндекс Диск (XLSX)"
	if s.App.Edition == EditionIntl {
		sheetsName = "Google Sheets"
	}
	var widget map[string]string
	if s.App.Acc != nil && s.App.AuthWidgetURL != "" {
		widget = map[string]string{"url": s.App.AuthWidgetURL, "demo": s.App.Acc.Demo(), "resource": s.App.LLMResource}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"auth_widget": widget,
		"edition":     s.App.Edition, "other_edition_url": s.App.OtherURL,
		"channels": channelsFor(s.App.Edition), "sheets": sheetsName, "llm": s.App.LLMName,
		"llm_mode": llmMode(s.App.LLM), "schedule": s.App.Schedule, "max_runs": MaxRuns, "max_added": MaxAdded,
		"host_rate": s.App.Collector.HostRate,
	})
}

func llmMode(c *ai.Client) string {
	if c == nil || c.LLM == nil || c.LLM.Name() == "mock" {
		return "mock"
	}
	return "live"
}

// ---- state ----

type compView struct {
	Key       string   `json:"key"`
	ID        string   `json:"id"`
	Market    string   `json:"market"`
	Name      string   `json:"name"`
	Seller    string   `json:"seller"`
	URL       string   `json:"url"`
	Added     bool     `json:"added"`
	Price     int64    `json:"price"`
	PrevPrice int64    `json:"prev_price"`
	InStock   bool     `json:"in_stock"`
	Stock     *int64   `json:"stock"`
	Rating    *float64 `json:"rating"`
	Reviews   int64    `json:"reviews"`
	Failed    bool     `json:"failed"`
	Prices    []*int64 `json:"prices"` // history window, null = not tracked / out of stock
	Stocks    []*int64 `json:"stocks"`
}

type productView struct {
	ID       string         `json:"id"`
	OfferID  string         `json:"offer_id"`
	Name     string         `json:"name"`
	MyPrice  int64          `json:"my_price"`
	MyPrices []int64        `json:"my_prices"`
	Stock    int64          `json:"stock"`
	MinPrice int64          `json:"min_price"`
	DiffPct  float64        `json:"diff_pct"`
	Rec      Recommendation `json:"rec"`
	Comps    []compView     `json:"comps"`
}

func (s *Server) buildState(ctx context.Context, sess *Session, lang string) map[string]any {
	d := sess.Day
	from := d - HistoryLen + 1
	dates := []string{}
	for i := from; i <= d; i++ {
		dates = append(dates, sess.Date(i).Format("2006-01-02"))
	}
	var products []productView
	changes := 0
	for _, p := range sess.products {
		pv := productView{ID: p.ID, OfferID: p.OfferID, Name: p.Name(lang), MyPrice: sess.MyPrice(p.ID, d), Stock: p.Stock,
			Rec: sess.recommend(p, d)}
		for i := from; i <= d; i++ {
			pv.MyPrices = append(pv.MyPrices, sess.MyPrice(p.ID, i))
		}
		for _, c := range sess.compsOf(p.ID) {
			o, prev := sess.ObsAt(c, d), sess.ObsAt(c, d-1)
			cv := compView{Key: compKey(c), ID: c.ID, Market: c.Market, Name: c.Name(lang), Seller: c.SellerName(lang), URL: c.URL(), Added: c.Added,
				Price: o.Price, PrevPrice: prev.Price, InStock: o.InStock, Stock: o.Stock, Rating: o.Rating, Reviews: o.Reviews, Failed: o.Failed}
			if c.Added && d == c.AddedDay {
				cv.PrevPrice = o.Price
			}
			if cv.PrevPrice != cv.Price && !o.Failed {
				changes++
			}
			for i := from; i <= d; i++ {
				if c.Added && i < c.AddedDay {
					cv.Prices, cv.Stocks = append(cv.Prices, nil), append(cv.Stocks, nil)
					continue
				}
				oi := sess.ObsAt(c, i)
				pr := oi.Price
				cv.Prices = append(cv.Prices, &pr)
				cv.Stocks = append(cv.Stocks, oi.Stock)
			}
			if o.InStock && (pv.MinPrice == 0 || o.Price < pv.MinPrice) {
				pv.MinPrice = o.Price
			}
			pv.Comps = append(pv.Comps, cv)
		}
		if pv.MinPrice > 0 {
			pv.DiffPct = float64((pv.MyPrice-pv.MinPrice)*1000/pv.MinPrice) / 10
		}
		products = append(products, pv)
	}
	reviews, _ := sess.Reviews(ctx, lang)
	unanswered := 0
	for _, r := range reviews {
		if !r.Answered {
			unanswered++
		}
	}
	rows, _ := sess.SheetRows(ctx)
	total := len(rows)
	if len(rows) > 21 {
		rows = append([][]string{rows[0]}, rows[len(rows)-20:]...)
	}
	rep := sess.BuildReport(lang)
	return map[string]any{
		"day": d, "date": sess.Date(d).Format("2006-01-02"), "runs": sess.Runs, "dates": dates,
		"products": products, "changes": changes, "expensive": len(rep.Expensive), "competitors": len(sess.Competitors()),
		"report": rep, "messages": sess.messages, "last_run": sess.lastRun,
		"reviews": reviews, "unanswered": unanswered, "consent": sess.consent,
		"sheet": map[string]any{"rows": rows, "total": max(0, total-1)},
	}
}

func (s *Server) state(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	writeJSON(w, http.StatusOK, s.buildState(r.Context(), sess, lang))
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	ip := clientIP(r)
	if !s.Guard.Refresh.Allow(ip) || !s.Guard.Global.Allow() {
		w.Header().Set("Retry-After", "3")
		apiErr(w, http.StatusTooManyRequests, "rate_limited", "too many collections, try again in a few seconds")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	run, err := sess.Collect(ctx, s.App.Collector)
	if errors.Is(err, errRunsLimit) {
		apiErr(w, http.StatusConflict, "runs_limit", "demo limit reached: reset the sandbox")
		return
	}
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "collection failed")
		return
	}
	st := s.buildState(r.Context(), sess, lang)
	st["run"] = run
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.App.Sessions.Drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(v)
}

func (s *Server) apply(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	p := sess.product(r.PathValue("id"))
	if p == nil {
		apiErr(w, http.StatusNotFound, "not_found", "unknown product")
		return
	}
	var in struct {
		Price int64 `json:"price"` // kopecks; 0 = the recommendation
	}
	_ = readJSON(r, &in)
	if in.Price == 0 {
		rec := sess.recommend(p, sess.Day)
		if rec.Kind != "lower" && rec.Kind != "raise" {
			apiErr(w, http.StatusConflict, "nothing_to_apply", "price is already in line with the market")
			return
		}
		in.Price = rec.Price
	}
	if in.Price < p.InitialPrice/2 || in.Price > p.InitialPrice*2 {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "price out of the allowed range")
		return
	}
	res, err := sess.ApplyPrice(r.Context(), p.ID, in.Price)
	if err != nil {
		apiErr(w, http.StatusBadGateway, "marketplace_error", "price update failed")
		return
	}
	st := s.buildState(r.Context(), sess, lang)
	st["applied"] = map[string]any{"product_id": p.ID, "price": in.Price, "result": res}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) addCompetitor(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	var in struct {
		ProductID string `json:"product_id"`
		Ref       string `json:"ref"`
	}
	if err := readJSON(r, &in); err != nil || utf8.RuneCountInString(in.Ref) > 300 {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "bad request")
		return
	}
	c, err := sess.AddCompetitor(in.ProductID, in.Ref)
	switch {
	case errors.Is(err, errBadRef):
		apiErr(w, http.StatusBadRequest, "bad_ref", "not a Wildberries/Ozon product link or article")
		return
	case errors.Is(err, errDuplicate):
		apiErr(w, http.StatusConflict, "duplicate", "already tracked")
		return
	case errors.Is(err, errTooMany):
		apiErr(w, http.StatusConflict, "too_many", "demo limit of added cards reached")
		return
	case err != nil:
		apiErr(w, http.StatusNotFound, "not_found", "unknown product")
		return
	}
	st := s.buildState(r.Context(), sess, lang)
	st["added"] = compKey(c)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) removeCompetitor(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	var in struct {
		Key string `json:"key"`
	}
	if err := readJSON(r, &in); err != nil || !sess.RemoveCompetitor(in.Key) {
		apiErr(w, http.StatusNotFound, "not_found", "only cards you added can be removed")
		return
	}
	writeJSON(w, http.StatusOK, s.buildState(r.Context(), sess, lang))
}

func (s *Server) sendReport(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	var in struct {
		Channel string `json:"channel"`
	}
	_ = readJSON(r, &in)
	ok := false
	for _, c := range channelsFor(s.App.Edition) {
		ok = ok || c == in.Channel
	}
	if !ok {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "channel is not available in this edition")
		return
	}
	m, err := sess.SendReport(r.Context(), in.Channel, lang)
	if err != nil {
		apiErr(w, http.StatusBadGateway, "notify_error", "delivery failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": m, "messages": sess.messages})
}

// addReview: the visitor pastes a review (RU edition: prices-v1 consent checkbox is required).
func (s *Server) addReview(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	var in struct {
		ProductID string `json:"product_id"`
		Rating    int    `json:"rating"`
		Text      string `json:"text"`
		Consent   string `json:"consent"`
	}
	if err := readJSON(r, &in); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "bad request")
		return
	}
	_, err := sess.AddOwnReview(in.ProductID, in.Rating, in.Text, in.Consent, time.Now())
	switch {
	case errors.Is(err, errConsent):
		apiErr(w, http.StatusBadRequest, "consent_required", "the prices-v1 consent is required")
		return
	case errors.Is(err, errOwnLimit):
		apiErr(w, http.StatusConflict, "too_many", "demo limit of pasted reviews reached")
		return
	case err != nil:
		apiErr(w, http.StatusBadRequest, "invalid_argument", "product, rating 1..5 and text up to 1000 characters are required")
		return
	}
	s.App.Log.Info("consent", "version", ConsentPrices, "session", sess.ID[:8]) // never the text
	writeJSON(w, http.StatusOK, s.buildState(r.Context(), sess, lang))
}

func (s *Server) draft(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	mock := ai.NewMock()
	var mode *accounts.Mode
	gen := mock.ReplyToReview
	if s.App.Acc != nil && llmMode(s.App.LLM) == "live" {
		// live model only through accounts: signed-in visitor, daily limit, refund when the provider fails
		gen = func(ctx context.Context, in ai.ReviewReplyInput) (*ai.ReviewReply, error) {
			res, m, err := accounts.LiveCall(ctx, s.App.Acc, accounts.LiveCallInput{Token: s.App.Acc.TokenFromRequest(r.Header),
				Resource: s.App.LLMResource, Units: 1, IdempotencyKey: "prices-draft-" + newID()},
				func(ctx context.Context) (*ai.ReviewReply, error) { return s.App.LLM.ReplyToReview(ctx, in) })
			mode = &m
			if err != nil || !m.Live {
				return mock.ReplyToReview(ctx, in)
			}
			return res, nil
		}
	}
	d, err := sess.DraftReply(ctx, gen, r.PathValue("id"))
	if errors.Is(err, errNotFound) {
		apiErr(w, http.StatusNotFound, "not_found", "unknown review")
		return
	}
	if err != nil {
		apiErr(w, http.StatusBadGateway, "ai_error", "the model did not answer, try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": d, "mode": mode})
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	var in struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &in); err != nil || strings.TrimSpace(in.Text) == "" || utf8.RuneCountInString(in.Text) > 1000 {
		apiErr(w, http.StatusBadRequest, "invalid_argument", "reply text is required (up to 1000 characters)")
		return
	}
	if err := sess.Answer(r.Context(), r.PathValue("id"), strings.TrimSpace(in.Text)); err != nil {
		e := mkit.As(err)
		if e != nil && e.Code == mkit.NotFound {
			apiErr(w, http.StatusNotFound, "not_found", "unknown review")
			return
		}
		apiErr(w, http.StatusConflict, "answer_failed", "could not publish the reply")
		return
	}
	writeJSON(w, http.StatusOK, s.buildState(r.Context(), sess, lang))
}

// exportCSV: the session's sheet as CSV (what the daily job writes to Google Sheets / Yandex Disk).
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	rows, err := sess.SheetRows(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "sheet")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="prices-`+sess.Date(sess.Day).Format("2006-01-02")+`.csv"`)
	w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM: Excel opens UTF-8 correctly
	cw := csv.NewWriter(w)
	cw.WriteAll(rows)
}

// exportXLSX: the same sheet as an .xlsx workbook (sheets-storage EncodeXLSX), numbers stay numbers.
func (s *Server) exportXLSX(w http.ResponseWriter, r *http.Request, sess *Session, lang string) {
	rows, err := sess.SheetRows(r.Context())
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "sheet")
		return
	}
	out := make([][]any, 0, len(rows))
	for i, row := range rows {
		cells := make([]any, len(row))
		for j, v := range row {
			cells[j] = v
			if i > 0 {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					cells[j] = f
				}
			}
		}
		out = append(out, cells)
	}
	b, err := sheets.EncodeXLSX([]sheets.XLSXSheet{{Name: SheetTab, Rows: out}})
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "xlsx")
		return
	}
	w.Header().Set("Content-Type", sheets.XLSXContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="prices-`+sess.Date(sess.Day).Format("2006-01-02")+`.xlsx"`)
	w.Write(b)
}
