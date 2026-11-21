package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	marketplaces "github.com/sedyukov/sedlab-modules/modules/marketplaces/go"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
)

const (
	StartDay       = HistoryLen // visitor starts with 30 days of "already collected" history
	MaxRuns        = 60         // collections per session (then "reset")
	MaxAdded       = 8          // visitor-added competitor cards per session
	MaxMessages    = 30
	SheetTab       = "prices"
	SheetID        = "price-monitoring"
	FloorPct       = 85 // never recommend below 85 % of the starting price
	UndercutPct    = 1  // recommended price = cheapest in-stock rival − 1 %
	RaiseThreshold = 10 // we are cheaper than everyone by > 10 % → suggest raising
)

// Product is the seller's own product (catalogue from the marketplaces module).
type Product struct {
	ID           string `json:"id"`
	OfferID      string `json:"offer_id"`
	ShortRU      string `json:"-"`
	ShortEN      string `json:"-"`
	InitialPrice int64  `json:"-"` // what the buyer pays (discount price when set), kopecks
	ListPrice    int64  `json:"-"` // crossed-out price
	HasDiscount  bool   `json:"-"`
	Stock        int64  `json:"-"`
}

func (p *Product) Name(lang string) string {
	if lang == "en" {
		return p.ShortEN
	}
	return p.ShortRU
}

// splitName: the module mock names products "Чай пуэр шу, 100 г (Shu puer tea, 100 g)".
func splitName(n string) (ru, en string) {
	if i := strings.LastIndex(n, " ("); i > 0 && strings.HasSuffix(n, ")") {
		return n[:i], n[i+2 : len(n)-1]
	}
	return n, n
}

// Obs is one observation of a competitor card on one day.
type Obs struct {
	Price   int64    `json:"price"` // kopecks
	InStock bool     `json:"in_stock"`
	Stock   *int64   `json:"stock"` // nil = the storefront does not show the exact remainder
	Rating  *float64 `json:"rating"`
	Reviews int64    `json:"reviews"`
	Failed  bool     `json:"failed,omitempty"` // fetch failed this day: values carried over from the day before
}

type pricePoint struct {
	Day   int
	Price int64
}

// Message is a report delivered through the notify module (mock: nothing leaves the server).
type Message struct {
	ID       string `json:"id"`
	Channel  string `json:"channel"`
	To       string `json:"to"`
	Text     string `json:"text"`
	Status   string `json:"status"`
	Provider string `json:"provider"`
	At       string `json:"at"`
}

// Session is one visitor's isolated sandbox.
type Session struct {
	ID       string
	Lang     string // language of the sheet rows (edition language)
	mu       sync.Mutex
	created  time.Time
	lastSeen time.Time
	base     time.Time // calendar date of day 0

	Day      int
	Runs     int
	mp       *marketplaces.Mock
	products []*Product
	prices   map[string][]pricePoint // our price changes
	extras   []*Competitor
	obs      map[int]map[string]Obs // days collected in this session: competitor key → observation
	sheet    *sheets.MockSheets
	rows     int
	notify   *notify.Client
	messages []Message
	lastRun  *Run
	drafts   map[string]Draft
	answered map[string]string
	own      []*Review // reviews pasted by the visitor (de-identified)
	nOwn     int
	consent  *Consent // prices-v1, given when pasting own texts
}

// Draft is an AI reply draft for a review.
type Draft struct {
	Text     string `json:"text"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func compKey(c *Competitor) string { return c.Market + ":" + c.ID + ":" + c.ProductID }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewSession builds a fresh sandbox: own marketplaces mock (catalogue, reviews), sheet and notify journal.
func NewSession(id string, now time.Time, lang string) (*Session, error) {
	s := &Session{ID: id, Lang: lang, created: now, lastSeen: now, Day: StartDay,
		base:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -StartDay),
		mp:     marketplaces.NewMock(),
		prices: map[string][]pricePoint{}, obs: map[int]map[string]Obs{},
		sheet: sheets.NewMockSheets(), notify: notify.NewMockClient(),
		drafts: map[string]Draft{}, answered: map[string]string{}}
	page, err := s.mp.ListProducts(context.Background(), marketplaces.ListProductsInput{Limit: 100})
	if err != nil {
		return nil, err
	}
	for _, it := range page.Items {
		ru, en := splitName(it.Name)
		p := &Product{ID: it.ID, OfferID: it.OfferID, ShortRU: ru, ShortEN: en, ListPrice: it.Price.Value,
			InitialPrice: it.Price.Value, Stock: it.Stock}
		if it.DiscountPrice != nil {
			p.InitialPrice, p.HasDiscount = it.DiscountPrice.Value, true
		}
		s.products = append(s.products, p)
	}
	if _, err := s.appendSheet(context.Background(), s.Day); err != nil { // today's rows are already in the sheet
		return nil, err
	}
	return s, nil
}

func (s *Session) Date(day int) time.Time { return s.base.AddDate(0, 0, day) }

func (s *Session) product(id string) *Product {
	for _, p := range s.products {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Competitors: built-in cards + the visitor's, in display order.
func (s *Session) Competitors() []*Competitor {
	out := append([]*Competitor{}, builtinCompetitors...)
	return append(out, s.extras...)
}

func (s *Session) compsOf(productID string) []*Competitor {
	var out []*Competitor
	for _, c := range s.Competitors() {
		if c.ProductID == productID {
			out = append(out, c)
		}
	}
	return out
}

// MyPrice: our price on a day (initial price, then changes applied through the marketplaces module).
func (s *Session) MyPrice(productID string, day int) int64 {
	p := s.product(productID)
	price := p.InitialPrice
	for _, pt := range s.prices[productID] {
		if pt.Day <= day {
			price = pt.Price
		}
	}
	return price
}

// truth: what the storefront shows on a day (used for the pre-loaded history; collected days come from scraping).
func truth(c *Competitor, d int) Obs {
	sr := SeriesOf(c)
	st := sr.Stock[d]
	o := Obs{Price: sr.Price[d], InStock: st > 0, Rating: c.Rating, Reviews: sr.Reviews[d]}
	if c.Market == MarketWB || st <= ozonStockShown {
		o.Stock = &st
	}
	if c.Rating == nil {
		o.Reviews = 0
	}
	return o
}

// ObsAt: collected observation when this session collected that day, else the simulated history.
func (s *Session) ObsAt(c *Competitor, d int) Obs {
	if m, ok := s.obs[d]; ok {
		if o, ok := m[compKey(c)]; ok {
			return o
		}
	}
	if c.Added && d < c.AddedDay {
		return Obs{} // not tracked yet
	}
	return truth(c, d)
}

// Recommendation for one product on the current day.
type Recommendation struct {
	Kind     string `json:"kind"` // lower | raise | ok | floor | none
	Price    int64  `json:"price"`
	Cheapest string `json:"cheapest"` // competitor key
}

func (s *Session) recommend(p *Product, day int) Recommendation {
	my := s.MyPrice(p.ID, day)
	var best *Competitor
	var bestPrice int64
	for _, c := range s.compsOf(p.ID) {
		o := s.ObsAt(c, day)
		if !o.InStock || o.Price == 0 {
			continue
		}
		if best == nil || o.Price < bestPrice {
			best, bestPrice = c, o.Price
		}
	}
	if best == nil {
		return Recommendation{Kind: "none"}
	}
	target := bestPrice * (100 - UndercutPct) / 100 / (10 * rub) * (10 * rub) // round down to tens of rubles
	floor := (p.InitialPrice*FloorPct/100 + 10*rub - 1) / (10 * rub) * (10 * rub)
	switch {
	case my > bestPrice && max(target, floor) < my:
		return Recommendation{Kind: "lower", Price: max(target, floor), Cheapest: compKey(best)}
	case my > bestPrice:
		return Recommendation{Kind: "floor", Price: my, Cheapest: compKey(best)} // already at the floor price
	case my*100 < bestPrice*(100-RaiseThreshold):
		return Recommendation{Kind: "raise", Price: target, Cheapest: compKey(best)}
	}
	return Recommendation{Kind: "ok", Price: my, Cheapest: compKey(best)}
}

// AddCompetitor parses a link/article and starts tracking it for a product.
func (s *Session) AddCompetitor(productID, ref string) (*Competitor, error) {
	p := s.product(productID)
	if p == nil {
		return nil, fmt.Errorf("unknown product")
	}
	market, id, ok := ParseRef(ref)
	if !ok {
		return nil, errBadRef
	}
	for _, c := range s.Competitors() {
		if c.Market == market && c.ID == id && c.ProductID == productID {
			return nil, errDuplicate
		}
	}
	if len(s.extras) >= MaxAdded {
		return nil, errTooMany
	}
	c := NewAddedCompetitor(market, id, p)
	c.AddedDay = s.Day
	s.extras = append(s.extras, c)
	return c, nil
}

func (s *Session) RemoveCompetitor(key string) bool {
	for i, c := range s.extras {
		if compKey(c) == key {
			s.extras = append(s.extras[:i], s.extras[i+1:]...)
			return true
		}
	}
	return false
}

// ApplyPrice sets our price through the marketplaces module (mock) and records it for today.
func (s *Session) ApplyPrice(ctx context.Context, productID string, price int64) (*marketplaces.BatchResult, error) {
	p := s.product(productID)
	if p == nil {
		return nil, fmt.Errorf("unknown product")
	}
	in := marketplaces.PriceUpdate{ID: p.ID, Price: marketplaces.Money{Value: p.ListPrice, Currency: "RUB"}}
	if p.HasDiscount || price < p.ListPrice {
		d := marketplaces.Money{Value: price, Currency: "RUB"}
		in.DiscountPrice = &d
		if price > p.ListPrice {
			in.Price.Value = price
		}
	} else {
		in.Price.Value = price
	}
	res, err := s.mp.UpdatePrices(ctx, marketplaces.UpdatePricesInput{Items: []marketplaces.PriceUpdate{in}})
	if err != nil {
		return nil, err
	}
	if res.Failed == 0 {
		pts := s.prices[p.ID]
		if n := len(pts); n > 0 && pts[n-1].Day == s.Day {
			pts[n-1].Price = price
		} else {
			pts = append(pts, pricePoint{s.Day, price})
		}
		s.prices[p.ID] = pts
	}
	return res, nil
}

// ---- session store ----

type Sessions struct {
	mu   sync.Mutex
	m    map[string]*Session
	max  int
	now  func() time.Time
	Lang string
}

func NewSessions(max int) *Sessions {
	return &Sessions{m: map[string]*Session{}, max: max, now: time.Now}
}

// Get returns the session (creating it when unknown), evicting the least recently seen one over the limit.
func (ss *Sessions) Get(id string) (*Session, bool, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := ss.now()
	if s, ok := ss.m[id]; ok && id != "" {
		s.lastSeen = now
		return s, false, nil
	}
	if len(ss.m) >= ss.max {
		var oldest *Session
		for _, s := range ss.m {
			if oldest == nil || s.lastSeen.Before(oldest.lastSeen) {
				oldest = s
			}
		}
		delete(ss.m, oldest.ID)
	}
	s, err := NewSession(newID(), now, ss.Lang)
	if err != nil {
		return nil, false, err
	}
	ss.m[s.ID] = s
	return s, true, nil
}

func (ss *Sessions) Drop(id string) {
	ss.mu.Lock()
	delete(ss.m, id)
	ss.mu.Unlock()
}

// Purge drops sessions idle longer than idle (0 = all); returns how many.
func (ss *Sessions) Purge(idle time.Duration) int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	n := 0
	cut := ss.now().Add(-idle)
	for id, s := range ss.m {
		if idle == 0 || s.lastSeen.Before(cut) {
			delete(ss.m, id)
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

func sortedKeys[M ~map[string]V, V any](m M) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
