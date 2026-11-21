package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	scraping "github.com/sedyukov/sedlab-modules/modules/scraping/go"
	scrapekit "github.com/sedyukov/sedlab-modules/modules/scraping/go/kit"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
)

var (
	errBadRef    = errors.New("bad_ref")
	errDuplicate = errors.New("duplicate")
	errTooMany   = errors.New("too_many")
	errRunsLimit = errors.New("runs_limit")
)

// LogLine is one request of a collection run, as shown in the "collection log".
type LogLine struct {
	Market   string `json:"market"`
	Key      string `json:"key"`
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Attempts int    `json:"attempts"`
	Ms       int64  `json:"ms"`
	Price    int64  `json:"price,omitempty"`
	Stock    *int64 `json:"stock,omitempty"`
	InStock  bool   `json:"in_stock"`
	Error    string `json:"error,omitempty"`
	Robots   bool   `json:"robots,omitempty"` // robots.txt request
}

// Run is the result of one collection ("Update now" or the daily schedule).
type Run struct {
	Day      int       `json:"day"`
	Date     string    `json:"date"`
	TookMs   int64     `json:"took_ms"`
	Fetched  int       `json:"fetched"`
	Failed   int       `json:"failed"`
	Retries  int       `json:"retries"`
	Rows     int       `json:"rows"`
	HostRate float64   `json:"host_rate"`
	Log      []LogLine `json:"log"`
}

// CollectorConfig: how polite the collector is. Live defaults are 1 request/s per site; the demo runs faster
// against its simulated storefronts so a visitor does not wait.
type CollectorConfig struct {
	HostRate  float64
	Retry     scrapekit.RetryPolicy
	UserAgent string
	Latency   bool // simulate storefront response times (off in tests)
}

func DefaultCollector() CollectorConfig {
	return CollectorConfig{HostRate: 12, Retry: scrapekit.RetryPolicy{MaxAttempts: 3, BaseDelay: 120 * time.Millisecond, MaxDelay: 600 * time.Millisecond},
		UserAgent: "sedlab-price-monitor/1.0 (+https://sedlab.dev)", Latency: true}
}

var (
	wbRules = map[string]scraping.Rule{
		"name":    {JSON: "data.products[0].name"},
		"price":   {JSON: "data.products[0].sizes[0].price.product", Type: "number"},
		"qty":     {JSON: "data.products[0].sizes[0].stocks[*].qty"},
		"rating":  {JSON: "data.products[0].reviewRating", Type: "number"},
		"reviews": {JSON: "data.products[0].feedbacks", Type: "number"},
	}
	ozonRules = map[string]scraping.Rule{
		"product": {JSONLD: true},
		"left":    {CSS: ".stock", Regex: `Осталось (\d+)`, Type: "number"},
		"rating":  {CSS: `script[type="application/ld+json"]`, Regex: `"ratingValue":"([0-9.]+)"`, Type: "number"},
		"reviews": {CSS: `script[type="application/ld+json"]`, Regex: `"reviewCount":"([0-9]+)"`, Type: "number"},
	}
)

// failureFor: a deterministic hiccup now and then so the log shows retries (429 on WB, 503 on Ozon).
func failureFor(c *Competitor, day int) (times, status int) {
	h := seedOf("fail", c.ID, fmt.Sprint(day)) % 23
	switch {
	case h == 0 && c.Market == MarketWB:
		return 1, 429
	case h == 1 && c.Market == MarketOzon:
		return 1, 503
	case h == 2:
		return 2, 503
	case h == 3 && day%5 == 0:
		return 3, 503 // gives up after 3 attempts: yesterday's values are kept, the error is logged
	}
	return 0, 0
}

// Collect moves the session one day forward and collects every tracked card through the scraping module.
func (s *Session) Collect(ctx context.Context, cfg CollectorConfig) (*Run, error) {
	if s.Runs >= MaxRuns || s.Day >= MaxDay {
		return nil, errRunsLimit
	}
	day := s.Day + 1
	comps := s.Competitors()

	pages := map[string]scraping.MockPage{
		"https://card.wb.ru/robots.txt":  {Body: wbRobots, Headers: map[string]string{"content-type": "text/plain"}},
		"https://www.ozon.ru/robots.txt": {Body: ozonRobots, Headers: map[string]string{"content-type": "text/plain"}},
	}
	for _, c := range comps {
		p := scraping.MockPage{}
		if cfg.Latency {
			p.LatencyMs = 20 + int(seedOf("lat", c.ID, fmt.Sprint(day))%60)
		}
		if c.Market == MarketWB {
			p.Body, p.Headers = wbCardJSON(c, day), map[string]string{"content-type": "application/json; charset=utf-8"}
		} else {
			p.Body, p.Headers = ozonPageHTML(c, day), map[string]string{"content-type": "text/html; charset=utf-8"}
		}
		p.FailTimes, p.FailStatus = failureFor(c, day)
		pages[c.FetchURL()] = p
	}
	mock := scraping.NewMock(false, pages)
	client := scraping.New(scraping.Config{Fetcher: mock, HostRate: cfg.HostRate, Retry: cfg.Retry, UserAgent: cfg.UserAgent})

	start := time.Now()
	lines := make([]LogLine, len(comps))
	obs := make([]Obs, len(comps))
	var wg sync.WaitGroup
	for _, market := range []string{MarketWB, MarketOzon} { // one worker per site: per-host limits apply
		wg.Add(1)
		go func(market string) {
			defer wg.Done()
			for i, c := range comps {
				if c.Market != market {
					continue
				}
				lines[i], obs[i] = fetchOne(ctx, client, c, s.ObsAt(c, day-1))
			}
		}(market)
	}
	wg.Wait()

	run := &Run{Day: day, Date: s.Date(day).Format("2006-01-02"), TookMs: time.Since(start).Milliseconds(), HostRate: cfg.HostRate}
	attempts := map[string]int{}
	for _, r := range mock.Requests() {
		attempts[strings.TrimPrefix(r, "GET ")]++
	}
	for _, host := range []string{"https://card.wb.ru/robots.txt", "https://www.ozon.ru/robots.txt"} {
		if attempts[host] > 0 {
			m := MarketWB
			if strings.Contains(host, "ozon") {
				m = MarketOzon
			}
			run.Log = append(run.Log, LogLine{Market: m, URL: host, Status: 200, Attempts: attempts[host], Robots: true, InStock: true})
		}
	}
	m := map[string]Obs{}
	for i, c := range comps {
		l := lines[i]
		l.Attempts = attempts[c.FetchURL()]
		run.Retries += max(0, l.Attempts-1)
		if l.Error != "" {
			run.Failed++
		} else {
			run.Fetched++
		}
		run.Log = append(run.Log, l)
		m[compKey(c)] = obs[i]
	}
	s.obs[day] = m
	s.Day = day
	s.Runs++
	n, err := s.appendSheet(ctx, day)
	if err != nil {
		return nil, err
	}
	run.Rows = n
	s.lastRun = run
	return run, nil
}

func fetchOne(ctx context.Context, client *scraping.Client, c *Competitor, prev Obs) (LogLine, Obs) {
	l := LogLine{Market: c.Market, Key: compKey(c), URL: c.FetchURL()}
	t := time.Now()
	page, err := client.Fetch(ctx, scraping.FetchInput{URL: c.FetchURL()})
	l.Ms = time.Since(t).Milliseconds()
	if err != nil {
		e := scrapekit.As(err)
		l.Status, l.Error = e.HTTPStatus, string(e.Code)
		prev.Failed = true
		return l, prev
	}
	l.Status = page.Status
	rules := wbRules
	if c.Market == MarketOzon {
		rules = ozonRules
	}
	r, err := scraping.ExtractPage(page, rules)
	if err != nil {
		l.Error = "extract"
		prev.Failed = true
		return l, prev
	}
	o := parseFields(c.Market, r.Fields)
	if o.Price == 0 {
		l.Error = "no_price"
		prev.Failed = true
		return l, prev
	}
	l.Price, l.Stock, l.InStock = o.Price, o.Stock, o.InStock
	return l, o
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// parseFields maps extracted fields to an observation (prices are kopecks).
func parseFields(market string, f map[string]any) Obs {
	var o Obs
	if v, ok := num(f["rating"]); ok && v > 0 {
		o.Rating = &v
	}
	if v, ok := num(f["reviews"]); ok {
		o.Reviews = int64(v)
	}
	if market == MarketWB {
		if v, ok := num(f["price"]); ok {
			o.Price = int64(v)
		}
		var total int64
		switch q := f["qty"].(type) {
		case []any:
			for _, x := range q {
				if v, ok := num(x); ok {
					total += int64(v)
				}
			}
		case float64:
			total = int64(q)
		}
		o.Stock, o.InStock = &total, total > 0
		return o
	}
	if p, ok := f["product"].(map[string]any); ok {
		if v, ok := p["price_minor"].(int64); ok {
			o.Price = v
		} else if v, ok := num(p["price_minor"]); ok {
			o.Price = int64(v)
		}
		o.InStock = p["availability"] != "OutOfStock"
	}
	if v, ok := num(f["left"]); ok {
		n := int64(v)
		o.Stock = &n
	} else if !o.InStock {
		z := int64(0)
		o.Stock = &z
	}
	return o
}

var sheetHeader = []any{"date", "product", "offer_id", "market", "competitor", "seller", "article", "price_rub", "in_stock", "stock", "our_price_rub", "diff_pct"}

// appendSheet writes the day's observations to the session's sheet (sheets-storage module, mock backend).
func (s *Session) appendSheet(ctx context.Context, day int) (int, error) {
	var rows [][]any
	if s.rows == 0 {
		rows = append(rows, sheetHeader)
	}
	date := s.Date(day).Format("2006-01-02")
	for _, c := range s.Competitors() {
		if c.Added && day < c.AddedDay {
			continue
		}
		o := s.ObsAt(c, day)
		p := s.product(c.ProductID)
		my := s.MyPrice(p.ID, day)
		var stock any
		if o.Stock != nil {
			stock = *o.Stock
		}
		diff := 0.0
		if my > 0 {
			diff = float64((o.Price-my)*1000/my) / 10
		}
		rows = append(rows, []any{date, p.Name(s.Lang), p.OfferID, strings.ToUpper(c.Market), c.Name(s.Lang), c.SellerName(s.Lang), c.ID,
			o.Price / 100, o.InStock, stock, my / 100, diff})
	}
	res, err := s.sheet.AppendRows(ctx, sheets.AppendRowsInput{SheetID: SheetID, Tab: SheetTab, Rows: rows,
		IdempotencyKey: fmt.Sprintf("%s:%d", s.ID, day)})
	if err != nil {
		return 0, err
	}
	s.rows += res.Appended
	n := res.Appended
	if len(rows) > 0 && rows[0][0] == "date" {
		n--
	}
	return n, nil
}

// SheetRows reads the session's sheet back through the module.
func (s *Session) SheetRows(ctx context.Context) ([][]string, error) {
	if s.rows == 0 {
		return [][]string{}, nil
	}
	r, err := s.sheet.ReadRows(ctx, sheets.ReadRowsInput{SheetID: SheetID, Tab: SheetTab})
	if err != nil {
		return nil, err
	}
	return r.Rows, nil
}
