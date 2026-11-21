package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ai "github.com/sedyukov/sedlab-modules/modules/ai/go"
	marketplaces "github.com/sedyukov/sedlab-modules/modules/marketplaces/go"
	scrapekit "github.com/sedyukov/sedlab-modules/modules/scraping/go/kit"
)

func fastCollector() CollectorConfig {
	c := DefaultCollector()
	c.HostRate = -1
	c.Latency = false
	c.Retry = scrapekit.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
	return c
}

func newSess(t *testing.T) *Session {
	t.Helper()
	s, err := NewSession("test", time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC), "ru")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The scraping pipeline (fetch → extract) must reproduce exactly what the storefronts show.
func TestCollectMatchesStorefront(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	retries, failed := 0, 0
	for i := 0; i < 25; i++ {
		run, err := s.Collect(ctx, fastCollector())
		if err != nil {
			t.Fatal(err)
		}
		retries += run.Retries
		failed += run.Failed
		for _, c := range s.Competitors() {
			got, want := s.ObsAt(c, run.Day), truth(c, run.Day)
			if got.Failed {
				continue
			}
			if got.Price != want.Price || got.InStock != want.InStock {
				t.Fatalf("day %d %s: got %+v want %+v", run.Day, compKey(c), got, want)
			}
			if (got.Stock == nil) != (want.Stock == nil) || (got.Stock != nil && *got.Stock != *want.Stock) {
				t.Fatalf("day %d %s stock: got %v want %v", run.Day, compKey(c), got.Stock, want.Stock)
			}
			if c.Rating != nil && (got.Rating == nil || *got.Rating != *c.Rating || got.Reviews != want.Reviews) {
				t.Fatalf("day %d %s rating/reviews: got %v/%d", run.Day, compKey(c), got.Rating, got.Reviews)
			}
		}
		if run.Fetched+run.Failed != len(s.Competitors()) {
			t.Fatalf("run counts %+v", run)
		}
	}
	if retries == 0 {
		t.Fatal("expected some retries over 25 days (429/503 hiccups)")
	}
	if failed == 0 {
		t.Fatal("expected at least one give-up (kept yesterday's data)")
	}
	if s.Day != StartDay+25 || s.Runs != 25 {
		t.Fatalf("day %d runs %d", s.Day, s.Runs)
	}
}

func TestFailedFetchKeepsYesterday(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	for i := 0; i < 60 && s.Runs < MaxRuns; i++ {
		run, err := s.Collect(ctx, fastCollector())
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range run.Log {
			if l.Error == "" || l.Robots {
				continue
			}
			var c *Competitor
			for _, x := range s.Competitors() {
				if compKey(x) == l.Key {
					c = x
				}
			}
			o, prev := s.ObsAt(c, run.Day), s.ObsAt(c, run.Day-1)
			if !o.Failed || o.Price != prev.Price || l.Attempts != 3 {
				t.Fatalf("failed fetch: %+v obs %+v prev %+v", l, o, prev)
			}
			return
		}
	}
	t.Fatal("no give-up within 60 days")
}

func TestRunsLimit(t *testing.T) {
	s := newSess(t)
	s.Runs = MaxRuns
	if _, err := s.Collect(context.Background(), fastCollector()); err != errRunsLimit {
		t.Fatalf("want runs limit, got %v", err)
	}
}

func TestSeriesPlausible(t *testing.T) {
	for _, c := range builtinCompetitors {
		sr := SeriesOf(c)
		changes := 0
		for d := 1; d <= MaxDay; d++ {
			p := sr.Price[d]
			if p < c.Base*65/100 || p > c.Base*125/100 || p%(10*rub) != 0 {
				t.Fatalf("%s day %d price %d out of range (base %d)", c.ID, d, p, c.Base)
			}
			if p != sr.Price[d-1] {
				changes++
			}
			if sr.Stock[d] < 0 {
				t.Fatal("negative stock")
			}
		}
		if changes < 20 || changes > 250 {
			t.Fatalf("%s: %d price changes in %d days", c.ID, changes, MaxDay)
		}
	}
	// deterministic
	a, b := SeriesOf(builtinCompetitors[0]), SeriesOf(builtinCompetitors[0])
	if a != b {
		t.Fatal("series not cached")
	}
}

func TestParseRef(t *testing.T) {
	cases := []struct{ in, market, id string }{
		{"https://www.wildberries.ru/catalog/146972802/detail.aspx?targetUrl=GP", MarketWB, "146972802"},
		{"wildberries.ru/catalog/12345678/detail.aspx", MarketWB, "12345678"},
		{"146972802", MarketWB, "146972802"},
		{"https://www.ozon.ru/product/termobutylka-750-ml-1077345620/?asb=1", MarketOzon, "1077345620"},
		{"ozon:1077345620", MarketOzon, "1077345620"},
	}
	for _, c := range cases {
		m, id, ok := ParseRef(c.in)
		if !ok || m != c.market || id != c.id {
			t.Fatalf("%q → %s %s %v", c.in, m, id, ok)
		}
	}
	for _, bad := range []string{"", "abc", "https://example.com/catalog/123456/", "https://www.ozon.ru/category/x-123456/", "https://ozon.ru/t/AbCdEf"} {
		if _, _, ok := ParseRef(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestRecommendAndApply(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	var p *Product
	var rec Recommendation
	for i := 0; i < 40; i++ {
		for _, x := range s.products {
			if r := s.recommend(x, s.Day); r.Kind == "lower" {
				p, rec = x, r
			}
		}
		if p != nil {
			break
		}
		s.Collect(ctx, fastCollector())
	}
	if p == nil {
		t.Fatal("no product above the market in 40 days")
	}
	floor := p.InitialPrice * FloorPct / 100
	if rec.Price < floor || rec.Price >= s.MyPrice(p.ID, s.Day) {
		t.Fatalf("rec %+v floor %d my %d", rec, floor, s.MyPrice(p.ID, s.Day))
	}
	res, err := s.ApplyPrice(ctx, p.ID, rec.Price)
	if err != nil || res.Failed != 0 {
		t.Fatalf("apply: %v %+v", err, res)
	}
	if s.MyPrice(p.ID, s.Day) != rec.Price || s.MyPrice(p.ID, s.Day-1) == rec.Price && s.Day > StartDay {
		t.Fatal("price history not recorded for today only")
	}
	// the marketplace (module mock) has the new price
	got, _ := s.mp.GetPrices(ctx, marketplaces.GetPricesInput{IDs: []string{p.ID}})
	if eff := got[0].Price.Value; got[0].DiscountPrice != nil {
		eff = got[0].DiscountPrice.Value
		if eff != rec.Price {
			t.Fatalf("marketplace price %d want %d", eff, rec.Price)
		}
	} else if eff != rec.Price {
		t.Fatalf("marketplace price %d want %d", eff, rec.Price)
	}
}

func TestAddCompetitor(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	c, err := s.AddCompetitor("prod_2", "https://www.wildberries.ru/catalog/98765432/detail.aspx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCompetitor("prod_2", "98765432"); err != errDuplicate {
		t.Fatalf("dup: %v", err)
	}
	if _, err := s.AddCompetitor("prod_2", "hello"); err != errBadRef {
		t.Fatalf("bad: %v", err)
	}
	if o := s.ObsAt(c, s.Day-1); o.Price != 0 {
		t.Fatal("added card must have no history before it was added")
	}
	run, err := s.Collect(ctx, fastCollector())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range run.Log {
		found = found || l.Key == compKey(c)
	}
	if !found || s.ObsAt(c, s.Day).Price == 0 {
		t.Fatal("added card not collected")
	}
	for i := 0; i < MaxAdded; i++ {
		s.AddCompetitor("prod_1", "1100000"+string(rune('0'+i))+"00")
	}
	if _, err := s.AddCompetitor("prod_1", "55555555"); err != errTooMany {
		t.Fatalf("limit: %v", err)
	}
	if !s.RemoveCompetitor(compKey(c)) || s.RemoveCompetitor("wb:200000001:prod_1") {
		t.Fatal("remove: only added cards")
	}
}

func TestReportAndNotify(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		s.Collect(ctx, fastCollector())
	}
	for _, lang := range []string{"ru", "en"} {
		r := s.BuildReport(lang)
		if r.Checked != len(builtinCompetitors) || r.Text == "" {
			t.Fatalf("report %+v", r)
		}
		if strings.Contains(r.Text, "—") {
			t.Fatal("em dash in report text")
		}
	}
	for _, ch := range channelsFor(EditionRU) {
		m, err := s.SendReport(ctx, ch, "ru")
		if err != nil || m.Status == "" || m.Provider != "mock" {
			t.Fatalf("%s: %v %+v", ch, err, m)
		}
	}
	if len(s.messages) != 2 {
		t.Fatal("messages not journaled")
	}
}

func TestReviewDraftAndAnswer(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	rs, _ := s.Reviews(ctx, "ru")
	if len(rs) == 0 || rs[0].Answered {
		t.Fatal("unanswered reviews must come first")
	}
	for _, r := range rs {
		if r.Answered {
			continue
		}
		d, err := s.DraftReply(ctx, ai.NewMock().ReplyToReview, r.ID)
		if err != nil || d.Text == "" || strings.HasPrefix(d.Text, "mock:") || strings.Contains(d.Text, "—") {
			t.Fatalf("draft %s: %v %+v", r.ID, err, d)
		}
		if isCyrillic(r.Text) != isCyrillic(d.Text) {
			t.Fatalf("draft language differs from review: %q → %q", r.Text, d.Text)
		}
	}
	if err := s.Answer(ctx, rs[0].ID, "Спасибо!"); err != nil {
		t.Fatal(err)
	}
	rs2, _ := s.Reviews(ctx, "ru")
	for _, r := range rs2 {
		if r.ID == rs[0].ID && (!r.Answered || r.AnswerText != "Спасибо!") {
			t.Fatal("answer not stored in the marketplace")
		}
	}
	if _, err := s.DraftReply(ctx, nil, "nope"); err != errNotFound {
		t.Fatal("unknown review")
	}
}

// ---- HTTP ----

type client struct {
	t   *testing.T
	srv *httptest.Server
	jar []*http.Cookie
}

func (c *client) do(method, path, body string) (int, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if ck := resp.Cookies(); len(ck) > 0 {
		c.jar = ck
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func newTestServer(t *testing.T, edition string) *client {
	ss := NewSessions(50)
	if edition == EditionIntl {
		ss.Lang = "en"
	}
	app := &App{Edition: edition, Sessions: ss, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), LLM: ai.NewMock(), LLMName: "GigaChat", Collector: fastCollector(), Schedule: "09:00"}
	g := DefaultGuard()
	srv := httptest.NewServer(NewServer(app, g))
	t.Cleanup(srv.Close)
	return &client{t: t, srv: srv}
}

func TestHTTPFlow(t *testing.T) {
	c := newTestServer(t, EditionRU)
	code, cfg := c.do("GET", "/api/config", "")
	if code != 200 || cfg["edition"] != "ru" {
		t.Fatalf("config %d %v", code, cfg)
	}
	code, st := c.do("GET", "/api/state", "")
	if code != 200 || len(st["products"].([]any)) != 5 || len(st["dates"].([]any)) != HistoryLen {
		t.Fatalf("state %d", code)
	}
	day0 := st["day"].(float64)
	code, st = c.do("POST", "/api/refresh", "")
	if code != 200 || st["day"].(float64) != day0+1 || st["run"] == nil {
		t.Fatalf("refresh %d %v", code, st["error"])
	}
	// same session: state keeps the new day
	_, st = c.do("GET", "/api/state", "")
	if st["day"].(float64) != day0+1 {
		t.Fatal("session not kept")
	}
	code, _ = c.do("POST", "/api/competitors", `{"product_id":"prod_1","ref":"https://www.ozon.ru/product/chay-puer-1111222233/"}`)
	if code != 200 {
		t.Fatalf("add %d", code)
	}
	code, _ = c.do("POST", "/api/competitors", `{"product_id":"prod_1","ref":"nope"}`)
	if code != 400 {
		t.Fatalf("bad ref %d", code)
	}
	code, m := c.do("POST", "/api/report/send", `{"channel":"max"}`)
	if code != 200 || m["message"] == nil {
		t.Fatalf("send %d", code)
	}
	code, _ = c.do("POST", "/api/report/send", `{"channel":"email"}`)
	if code != 400 {
		t.Fatalf("email in RU edition: %d", code)
	}
	code, m = c.do("POST", "/api/reviews/rev_2/draft", "")
	if code != 200 {
		t.Fatalf("draft %d", code)
	}
	code, st = c.do("POST", "/api/reviews/rev_2/answer", `{"text":"Здравствуйте! Разберёмся."}`)
	if code != 200 || st["unanswered"].(float64) != 3 {
		t.Fatalf("answer %d %v", code, st["unanswered"])
	}
	req, _ := http.NewRequest("GET", c.srv.URL+"/api/export.csv", nil)
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	resp, _ := http.DefaultClient.Do(req)
	buf := new(strings.Builder)
	b := make([]byte, 64<<10)
	n, _ := resp.Body.Read(b)
	buf.Write(b[:n])
	resp.Body.Close()
	if !strings.HasPrefix(buf.String(), "\xef\xbb\xbfdate,product") || strings.Count(buf.String(), "\n") < 2*len(builtinCompetitors) {
		t.Fatalf("csv: %q", buf.String()[:min(200, buf.Len())])
	}
	code, _ = c.do("POST", "/api/reset", "")
	if code != 200 {
		t.Fatal("reset")
	}
	c.jar = nil
	_, st = c.do("GET", "/api/state", "")
	if st["day"].(float64) != StartDay {
		t.Fatal("reset must give a fresh sandbox")
	}
}

func TestHTTPApplyAndIsolation(t *testing.T) {
	a := newTestServer(t, EditionIntl)
	b := &client{t: t, srv: a.srv}
	for i := 0; i < 40; i++ {
		_, st := a.do("GET", "/api/state", "")
		for _, p := range st["products"].([]any) {
			pm := p.(map[string]any)
			if pm["rec"].(map[string]any)["kind"] == "lower" {
				code, st2 := a.do("POST", "/api/products/"+pm["id"].(string)+"/apply", "")
				if code != 200 || st2["applied"] == nil {
					t.Fatalf("apply %d", code)
				}
				_, sb := b.do("GET", "/api/state", "")
				if sb["day"].(float64) != StartDay {
					t.Fatal("sessions are not isolated")
				}
				return
			}
		}
		a.do("POST", "/api/refresh", "")
	}
	t.Fatal("nothing to apply in 40 days")
}

func TestRefreshRateLimit(t *testing.T) {
	c := newTestServer(t, EditionRU)
	limited := false
	for i := 0; i < 12; i++ {
		if code, _ := c.do("POST", "/api/refresh", ""); code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("refresh is not rate limited")
	}
}

func TestSessionsLRU(t *testing.T) {
	ss := NewSessions(3)
	var ids []string
	for i := 0; i < 4; i++ {
		s, _, _ := ss.Get("")
		ids = append(ids, s.ID)
	}
	if ss.Len() != 3 {
		t.Fatal("LRU limit")
	}
	if n := ss.Purge(0); n != 3 {
		t.Fatal("purge all")
	}
}

func TestStaticAndHealth(t *testing.T) {
	c := newTestServer(t, "")
	for _, p := range []string{"/health", "/", "/app.js", "/style.css"} {
		resp, err := http.Get(c.srv.URL + p)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", p, err, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// legal-ru: buyer names never reach the page or the model; pasted texts need prices-v1 and are de-identified.
func TestReviewsDeidentified(t *testing.T) {
	s := newSess(t)
	ctx := context.Background()
	rs, _ := s.Reviews(ctx, "ru")
	raw, _ := s.mp.ListReviews(ctx, marketplaces.ListReviewsInput{Limit: 100})
	b, _ := json.Marshal(rs)
	for _, r := range raw {
		if r.Author != "" && strings.Contains(string(b), r.Author) {
			t.Fatalf("author name %q leaked", r.Author)
		}
	}
	if _, err := s.AddOwnReview("prod_2", 2, "Отзыв", "", time.Now()); err != errConsent {
		t.Fatalf("no consent: %v", err)
	}
	r, err := s.AddOwnReview("prod_2", 2, "Иван Петров, звоните +7 913 123-45-67: кружка с трещиной", ConsentPrices, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Text, "913") || strings.Contains(r.Text, "Петров") || r.Masked < 2 {
		t.Fatalf("not de-identified: %q (%d)", r.Text, r.Masked)
	}
	if s.consent == nil || s.consent.Version != ConsentPrices || s.consent.At == "" {
		t.Fatal("consent not recorded")
	}
	var got ai.ReviewReplyInput
	gen := func(ctx context.Context, in ai.ReviewReplyInput) (*ai.ReviewReply, error) {
		got = in
		return ai.NewMock().ReplyToReview(ctx, in)
	}
	if _, err := s.DraftReply(ctx, gen, r.ID); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Text, "913") || strings.Contains(got.Text, "Петров") {
		t.Fatal("personal data sent to the model")
	}
	if err := s.Answer(ctx, r.ID, "Здравствуйте! Заменим."); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxOwnReviews; i++ {
		s.AddOwnReview("prod_1", 5, "ok", ConsentPrices, time.Now())
	}
	if _, err := s.AddOwnReview("prod_1", 5, "ok", ConsentPrices, time.Now()); err != errOwnLimit {
		t.Fatalf("limit: %v", err)
	}
}

func TestHTTPOwnReviewNeedsConsent(t *testing.T) {
	c := newTestServer(t, EditionRU)
	if code, m := c.do("POST", "/api/reviews", `{"product_id":"prod_1","rating":4,"text":"Хороший чай"}`); code != 400 || m["error"].(map[string]any)["code"] != "consent_required" {
		t.Fatalf("no consent: %d", code)
	}
	code, st := c.do("POST", "/api/reviews", `{"product_id":"prod_1","rating":4,"text":"Хороший чай","consent":"prices-v1"}`)
	if code != 200 || st["consent"] == nil || st["unanswered"].(float64) != 5 {
		t.Fatalf("own review: %d %v", code, st["unanswered"])
	}
}
