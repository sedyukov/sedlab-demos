package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
)

// Change is one notable move of a competitor between two days.
type Change struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"` // down | up | out | back
	From    int64  `json:"from"`
	To      int64  `json:"to"`
	Pct     int64  `json:"pct"`
	Product string `json:"product"`
}

// Report is the daily summary (shown on the page, sent through notify, rows already in the sheet).
type Report struct {
	Date      string   `json:"date"`
	Checked   int      `json:"checked"`
	Failed    int      `json:"failed"`
	Changes   []Change `json:"changes"`
	Expensive []string `json:"expensive"` // product ids where we are above the cheapest rival
	Text      string   `json:"text"`
}

var monthsRU = []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

func fmtDate(t time.Time, lang string) string {
	if lang == "en" {
		return t.Format("January 2, 2006")
	}
	return fmt.Sprintf("%d %s %d", t.Day(), monthsRU[t.Month()-1], t.Year())
}

func marketName(m, lang string) string {
	if m == MarketWB {
		if lang == "en" {
			return "Wildberries"
		}
		return "WB"
	}
	return "Ozon"
}

// BuildReport compares the current day with the day before.
func (s *Session) BuildReport(lang string) *Report {
	d := s.Day
	r := &Report{Date: s.Date(d).Format("2006-01-02")}
	byKey := map[string]*Competitor{}
	for _, c := range s.Competitors() {
		if c.Added && d < c.AddedDay {
			continue
		}
		byKey[compKey(c)] = c
		r.Checked++
		now := s.ObsAt(c, d)
		if now.Failed {
			r.Failed++
			continue
		}
		if c.Added && d == c.AddedDay {
			continue
		}
		prev := s.ObsAt(c, d-1)
		ch := Change{Key: compKey(c), From: prev.Price, To: now.Price, Product: c.ProductID}
		switch {
		case prev.InStock && !now.InStock:
			ch.Kind = "out"
		case !prev.InStock && now.InStock:
			ch.Kind = "back"
		case now.Price < prev.Price:
			ch.Kind = "down"
		case now.Price > prev.Price:
			ch.Kind = "up"
		default:
			continue
		}
		if prev.Price > 0 {
			ch.Pct = (now.Price - prev.Price) * 100 / prev.Price
		}
		r.Changes = append(r.Changes, ch)
	}
	for _, p := range s.products {
		if s.recommend(p, d).Kind == "lower" {
			r.Expensive = append(r.Expensive, p.ID)
		}
	}
	r.Text = s.reportText(r, byKey, lang)
	return r
}

func (s *Session) reportText(r *Report, byKey map[string]*Competitor, lang string) string {
	en := lang == "en"
	cur := " ₽"
	var b strings.Builder
	t := func(ru, e string) string {
		if en {
			return e
		}
		return ru
	}
	fmt.Fprintf(&b, "%s · %s\n", t("Мониторинг цен", "Price monitoring"), fmtDate(s.Date(s.Day), lang))
	fmt.Fprintf(&b, t("Проверено карточек конкурентов: %d (WB и Ozon)", "Competitor cards checked: %d (Wildberries and Ozon)"), r.Checked)
	if r.Failed > 0 {
		fmt.Fprintf(&b, t(", не ответили: %d (оставили вчерашние данные)", ", not available: %d (kept yesterday's data)"), r.Failed)
	}
	b.WriteString("\n")
	groups := []struct{ kind, ru, en string }{
		{"down", "Снизили цену", "Price drops"}, {"up", "Подняли цену", "Price increases"},
		{"out", "Закончились", "Out of stock"}, {"back", "Снова в продаже", "Back in stock"},
	}
	any := false
	for _, g := range groups {
		var lines []string
		for _, ch := range r.Changes {
			if ch.Kind != g.kind {
				continue
			}
			c := byKey[ch.Key]
			who := fmt.Sprintf("%s (%s, %s)", c.Name(lang), c.SellerName(lang), marketName(c.Market, lang))
			switch g.kind {
			case "down", "up":
				lines = append(lines, fmt.Sprintf("• %s: %s → %s%s (%+d%%)", who, fmtRub(ch.From, lang), fmtRub(ch.To, lang), cur, ch.Pct))
			default:
				lines = append(lines, "• "+who)
			}
		}
		if len(lines) > 0 {
			any = true
			fmt.Fprintf(&b, "\n%s (%d):\n%s\n", t(g.ru, g.en), len(lines), strings.Join(lines, "\n"))
		}
	}
	if !any {
		b.WriteString(t("\nЗа сутки у конкурентов без изменений.\n", "\nNo competitor changes in the last 24 hours.\n"))
	}
	if len(r.Expensive) > 0 {
		fmt.Fprintf(&b, "\n%s (%d):\n", t("Вы дороже рынка", "You are above the market"), len(r.Expensive))
		for _, id := range r.Expensive {
			p := s.product(id)
			rec := s.recommend(p, s.Day)
			c := byKey[rec.Cheapest]
			o := s.ObsAt(c, s.Day)
			fmt.Fprintf(&b, t("• %s: ваша цена %s%s, дешевле всех %s (%s) %s%s. Рекомендуем %s%s\n",
				"• %s: yours %s%s, cheapest is %s (%s) at %s%s. Suggested: %s%s\n"),
				p.Name(lang), fmtRub(s.MyPrice(id, s.Day), lang), cur, c.SellerName(lang), marketName(c.Market, lang), fmtRub(o.Price, lang), cur,
				fmtRub(rec.Price, lang), cur)
		}
	} else {
		b.WriteString(t("\nПо всем товарам ваша цена в рынке.\n", "\nAll your prices are in line with the market.\n"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- notify ----

// Channels per edition: RU sends to Max and Telegram, International to Telegram and email (all mock in the demo).
func channelsFor(edition string) []string {
	if edition == EditionIntl {
		return []string{"telegram", "email"}
	}
	return []string{"max", "telegram"}
}

// recipient: demo chat ids (mock transport, nothing is delivered) and the name shown on the page.
func recipient(channel, lang string) (to, shown string) {
	switch channel {
	case "email":
		return "manager@example.com", "manager@example.com"
	case "max":
		return "100200300", "Менеджер (Max)"
	}
	if lang == "en" {
		return "-1001000000001", "Manager (Telegram)"
	}
	return "-1001000000001", "Менеджер (Telegram)"
}

// SendReport delivers the report through the notify module (mock journal: nothing leaves the server).
func (s *Session) SendReport(ctx context.Context, channel, lang string) (*Message, error) {
	r := s.BuildReport(lang)
	to, shown := recipient(channel, lang)
	in := notify.SendInput{Channel: channel, To: to, Text: r.Text,
		IdempotencyKey: fmt.Sprintf("report:%s:%d:%s:%d", s.ID, s.Day, channel, len(s.messages))}
	if channel == "email" {
		in.Subject = strings.SplitN(r.Text, "\n", 2)[0]
	}
	d, err := s.notify.Send(ctx, in)
	if err != nil {
		return nil, err
	}
	m := Message{ID: d.ID, Channel: channel, To: shown, Text: r.Text, Status: d.Status, Provider: d.Provider, At: d.CreatedAt}
	s.messages = append(s.messages, m)
	if len(s.messages) > MaxMessages {
		s.messages = s.messages[len(s.messages)-MaxMessages:]
	}
	return &m, nil
}
