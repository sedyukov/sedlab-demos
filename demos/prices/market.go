package main

// The demo's simulated storefronts. In the public demo nothing is fetched from wildberries.ru / ozon.ru:
// the scraping module gets these pages through its mock transport, so the real pipeline (robots.txt,
// per-host limits, retries, extraction) runs end to end on plausible, deterministic data.

import (
	"fmt"
	marketplaces "github.com/sedyukov/sedlab-modules/modules/marketplaces/go"
	"hash/fnv"
	"math/rand"
	"strings"
	"sync"
)

const (
	MarketWB   = "wb"
	MarketOzon = "ozon"
	MaxDay     = 400 // simulated days available (30 of history + up to 370 collections)
	HistoryLen = 30  // days shown on charts
)

// Competitor is one tracked competitor card on a marketplace.
type Competitor struct {
	ID        string   `json:"id"`         // WB nmID / Ozon SKU
	Market    string   `json:"market"`     // wb | ozon
	ProductID string   `json:"product_id"` // our product it competes with
	NameRU    string   `json:"-"`
	NameEN    string   `json:"-"`
	Seller    string   `json:"seller"`
	Base      int64    `json:"-"` // kopecks
	BaseStock int64    `json:"-"`
	Rating    *float64 `json:"rating"`
	Reviews   int64    `json:"-"`
	Added     bool     `json:"added"` // added by the visitor
	AddedDay  int      `json:"-"`
}

// sellerEN: Latin names of the fictitious sellers for the International edition.
var sellerEN = map[string]string{
	"ЧайныйДом": "ChainyDom", "Чайная лавка Восток": "Vostok Tea Shop", "ЧайМаркет": "ChaiMarket", "ПосудаПлюс": "PosudaPlus",
	"Гончарная мастерская": "Pottery Workshop", "КухняДом": "KukhnyaDom", "Керамика Урала": "Ural Ceramics", "Лесной воск": "Forest Wax",
	"Дом ароматов": "House of Scents", "Турист": "Turist", "Актив Плюс": "Active Plus", "ТермоМир": "ThermoMir", "Уютный дом": "Cozy Home",
	"Северная пряжа": "Northern Yarn", "Маркет Сити": "Market City", "ТоварыДляВас": "Goods4You", "Склад24": "Sklad24",
	"Лучшая цена": "Best Price", "Домашний уют": "Home Comfort",
}

func (c *Competitor) SellerName(lang string) string {
	if lang == "en" {
		if v, ok := sellerEN[c.Seller]; ok {
			return v
		}
	}
	return c.Seller
}

func (c *Competitor) Name(lang string) string {
	if lang == "en" && c.NameEN != "" {
		return c.NameEN
	}
	return c.NameRU
}

// URL is the public storefront link of the card.
func (c *Competitor) URL() string {
	if c.Market == MarketWB {
		return "https://www.wildberries.ru/catalog/" + c.ID + "/detail.aspx"
	}
	return "https://www.ozon.ru/product/" + slug(c.NameEN) + "-" + c.ID + "/"
}

// FetchURL is what the collector requests: WB public card API, Ozon product page.
func (c *Competitor) FetchURL() string {
	if c.Market == MarketWB {
		return "https://card.wb.ru/cards/v2/detail?appType=1&curr=rub&dest=-1257786&nm=" + c.ID
	}
	return c.URL()
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func rt(v float64) *float64 { return &v }

const rub = 100 // kopecks

const seriesSalt = "series6" // picked for a lively first week of the demo

// Catalog: 5 products of the demo seller (ids/prices from the marketplaces module mock), 4 competitors each.
// WB cards 200000001..200000005 reuse the module mock's competitor ids; names, sellers and prices are fictitious.
var builtinCompetitors = []*Competitor{
	{ID: "200000001", Market: MarketWB, ProductID: "prod_1", NameRU: "Пуэр шу «Старый город», 100 г", NameEN: "Shu puer Old Town, 100 g", Seller: "ЧайныйДом", Base: 790 * rub, BaseStock: 140, Rating: rt(4.8), Reviews: 1284},
	{ID: "147381025", Market: MarketWB, ProductID: "prod_1", NameRU: "Чай пуэр шу прессованный, 100 г", NameEN: "Pressed shu puer tea, 100 g", Seller: "Чайная лавка Восток", Base: 650 * rub, BaseStock: 60, Rating: rt(4.7), Reviews: 532},
	{ID: "1203457781", Market: MarketOzon, ProductID: "prod_1", NameRU: "Шу пуэр «Мэнхай», 100 г", NameEN: "Menghai shu puer, 100 g", Seller: "ЧайМаркет", Base: 720 * rub, BaseStock: 35, Rating: rt(4.9), Reviews: 211},
	{ID: "1188004512", Market: MarketOzon, ProductID: "prod_1", NameRU: "Пуэр шу листовой, 100 г", NameEN: "Loose leaf shu puer, 100 g", Seller: "Green Leaf", Base: 590 * rub, BaseStock: 18, Rating: rt(4.5), Reviews: 98},

	{ID: "200000002", Market: MarketWB, ProductID: "prod_2", NameRU: "Кружка 350 мл, керамика, белая", NameEN: "Mug 350 ml, white ceramic", Seller: "ПосудаПлюс", Base: 520 * rub, BaseStock: 220, Rating: rt(4.6), Reviews: 312},
	{ID: "162994310", Market: MarketWB, ProductID: "prod_2", NameRU: "Кружка керамическая матовая, 350 мл", NameEN: "Matte ceramic mug, 350 ml", Seller: "Гончарная мастерская", Base: 540 * rub, BaseStock: 25, Rating: rt(4.8), Reviews: 77},
	{ID: "1094451207", Market: MarketOzon, ProductID: "prod_2", NameRU: "Кружка из керамики 350 мл, однотонная", NameEN: "Plain ceramic mug 350 ml", Seller: "КухняДом", Base: 510 * rub, BaseStock: 400, Rating: rt(4.6), Reviews: 640},
	{ID: "1265530018", Market: MarketOzon, ProductID: "prod_2", NameRU: "Кружка ручной работы, 350 мл", NameEN: "Handmade mug, 350 ml", Seller: "Керамика Урала", Base: 690 * rub, BaseStock: 8, Rating: rt(4.9), Reviews: 41},

	{ID: "200000003", Market: MarketWB, ProductID: "prod_3", NameRU: "Свеча ароматическая соевая, лаванда", NameEN: "Scented soy candle, lavender", Seller: "Aroma Studio", Base: 1290 * rub, BaseStock: 30, Rating: rt(4.9), Reviews: 87},
	{ID: "171230554", Market: MarketWB, ProductID: "prod_3", NameRU: "Соевая свеча в стекле «Лаванда», 180 г", NameEN: "Soy candle in glass Lavender, 180 g", Seller: "Лесной воск", Base: 1250 * rub, BaseStock: 45, Rating: rt(4.7), Reviews: 150},
	{ID: "1302298874", Market: MarketOzon, ProductID: "prod_3", NameRU: "Свеча соевая с деревянным фитилём, 200 г", NameEN: "Soy candle with wooden wick, 200 g", Seller: "Candle Bar", Base: 1290 * rub, BaseStock: 12, Rating: rt(4.8), Reviews: 64},
	{ID: "1157720431", Market: MarketOzon, ProductID: "prod_3", NameRU: "Аромасвеча «Лаванда и шалфей», 200 г", NameEN: "Scented candle Lavender and sage, 200 g", Seller: "Дом ароматов", Base: 1240 * rub, BaseStock: 70, Rating: rt(4.6), Reviews: 233},

	{ID: "200000004", Market: MarketWB, ProductID: "prod_4", NameRU: "Термобутылка из нержавеющей стали, 750 мл", NameEN: "Stainless steel thermo bottle, 750 ml", Seller: "SportLine", Base: 1390 * rub, BaseStock: 300, Rating: rt(4.5), Reviews: 2045},
	{ID: "154407993", Market: MarketWB, ProductID: "prod_4", NameRU: "Термос-бутылка 750 мл, двойные стенки", NameEN: "Double-wall thermos bottle, 750 ml", Seller: "Турист", Base: 1190 * rub, BaseStock: 120, Rating: rt(4.4), Reviews: 860},
	{ID: "1077345620", Market: MarketOzon, ProductID: "prod_4", NameRU: "Термобутылка спортивная, 750 мл", NameEN: "Sports thermo bottle, 750 ml", Seller: "Актив Плюс", Base: 1250 * rub, BaseStock: 210, Rating: rt(4.6), Reviews: 1320},
	{ID: "1219980057", Market: MarketOzon, ProductID: "prod_4", NameRU: "Бутылка-термос 0,75 л, нержавейка", NameEN: "Thermos bottle 0.75 l, stainless", Seller: "ТермоМир", Base: 1490 * rub, BaseStock: 55, Rating: rt(4.7), Reviews: 410},

	{ID: "200000005", Market: MarketWB, ProductID: "prod_5", NameRU: "Носки из шерсти мериноса, 3 пары", NameEN: "Merino wool socks, 3 pairs", Seller: "WoolMart", Base: 730 * rub, BaseStock: 90},
	{ID: "139985126", Market: MarketWB, ProductID: "prod_5", NameRU: "Носки шерстяные тёплые, набор 3 пары", NameEN: "Warm wool socks, set of 3 pairs", Seller: "Уютный дом", Base: 760 * rub, BaseStock: 500, Rating: rt(4.6), Reviews: 1720},
	{ID: "1146630295", Market: MarketOzon, ProductID: "prod_5", NameRU: "Носки зимние из шерсти, 3 пары", NameEN: "Winter wool socks, 3 pairs", Seller: "Северная пряжа", Base: 740 * rub, BaseStock: 150, Rating: rt(4.7), Reviews: 388},
	{ID: "1290014470", Market: MarketOzon, ProductID: "prod_5", NameRU: "Носки с шерстью альпаки, 3 пары", NameEN: "Alpaca wool socks, 3 pairs", Seller: "Alpaca Store", Base: 890 * rub, BaseStock: 20, Rating: rt(4.8), Reviews: 95},
}

// Series is the simulated truth of one competitor card: what its storefront shows on each day.
type Series struct {
	Price   []int64 // kopecks
	Stock   []int64
	Reviews []int64
}

var (
	seriesMu    sync.Mutex
	seriesCache = map[string]*Series{}
)

func seedOf(parts ...string) int64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// SeriesOf returns the deterministic daily series of a competitor (cached; same for every visitor).
// Prices move in steps: occasional promotions (−5..−15 % for 2–4 days) and lasting re-pricing (±3..8 %),
// kept within 80–120 % of the base price. Stock sells down and gets restocked, sometimes after a gap.
func SeriesOf(c *Competitor) *Series {
	key := c.Market + ":" + c.ID + ":" + c.ProductID
	seriesMu.Lock()
	defer seriesMu.Unlock()
	if s, ok := seriesCache[key]; ok {
		return s
	}
	r := rand.New(rand.NewSource(seedOf(seriesSalt, key)))
	s := &Series{Price: make([]int64, MaxDay+1), Stock: make([]int64, MaxDay+1), Reviews: make([]int64, MaxDay+1)}
	level, promo, promoLeft := c.Base, int64(0), 0
	stock, reviews := c.BaseStock, c.Reviews
	sell := max(1, c.BaseStock/12)
	for d := 0; d <= MaxDay; d++ {
		if d > 0 {
			switch {
			case promoLeft > 0:
				promoLeft--
				if promoLeft == 0 {
					promo = 0
				}
			case r.Intn(100) < 9:
				promo, promoLeft = int64(5+r.Intn(11)), 2+r.Intn(3)
			case r.Intn(100) < 7:
				pct := int64(3 + r.Intn(6))
				if r.Intn(2) == 0 {
					pct = -pct
				}
				level = level * (100 + pct) / 100
				level = min(max(level, c.Base*80/100), c.Base*120/100)
			}
			if stock > 0 {
				stock -= min(stock, int64(r.Intn(int(sell)+1)))
			}
			if stock < c.BaseStock/8 && r.Intn(100) < 30 {
				stock = c.BaseStock * int64(80+r.Intn(41)) / 100
			}
			if c.Rating != nil {
				reviews += int64(r.Intn(4))
			}
		}
		p := level * (100 - promo) / 100
		s.Price[d] = (p + 5*rub) / (10 * rub) * (10 * rub) // whole tens of rubles
		s.Stock[d] = stock
		s.Reviews[d] = reviews
	}
	seriesCache[key] = s
	if len(seriesCache) > 5000 { // visitor-added cards: keep the cache bounded
		for k := range seriesCache {
			delete(seriesCache, k)
			break
		}
	}
	return s
}

// Storefront pages for a day: what the collector fetches through the scraping module's mock transport.

func wbCardJSON(c *Competitor, d int) string {
	s := SeriesOf(c)
	stock := s.Stock[d]
	var stocks []string
	if stock > 0 {
		a := stock * 2 / 3
		stocks = append(stocks, fmt.Sprintf(`{"wh":507,"qty":%d}`, a))
		if stock-a > 0 {
			stocks = append(stocks, fmt.Sprintf(`{"wh":117986,"qty":%d}`, stock-a))
		}
	}
	rating := "0"
	if c.Rating != nil {
		rating = fmt.Sprintf("%.1f", *c.Rating)
	}
	return fmt.Sprintf(`{"state":0,"data":{"products":[{"id":%s,"name":%q,"supplier":%q,"reviewRating":%s,"feedbacks":%d,`+
		`"sizes":[{"name":"","price":{"basic":%d,"product":%d},"stocks":[%s]}]}]}}`,
		c.ID, c.NameRU, c.Seller, rating, s.Reviews[d], s.Price[d]*130/100/100*100, s.Price[d], strings.Join(stocks, ","))
}

// Ozon shows an exact remainder only when few are left ("Осталось N шт"); otherwise just "in stock".
const ozonStockShown = 50

func ozonPageHTML(c *Competitor, d int) string {
	s := SeriesOf(c)
	avail, left := "https://schema.org/InStock", ""
	switch st := s.Stock[d]; {
	case st == 0:
		avail = "https://schema.org/OutOfStock"
		left = `<div class="stock out">Нет в наличии</div>`
	case st <= ozonStockShown:
		left = fmt.Sprintf(`<div class="stock">Осталось %d шт</div>`, st)
	default:
		left = `<div class="stock">В наличии</div>`
	}
	rating := ""
	if c.Rating != nil {
		rating = fmt.Sprintf(`,"aggregateRating":{"@type":"AggregateRating","ratingValue":"%.1f","reviewCount":"%d"}`, *c.Rating, s.Reviews[d])
	}
	ld := fmt.Sprintf(`{"@context":"https://schema.org","@type":"Product","name":%q,"sku":"%s","brand":{"@type":"Brand","name":%q}%s,`+
		`"offers":{"@type":"Offer","price":"%d","priceCurrency":"RUB","availability":"%s","seller":{"@type":"Organization","name":%q}}}`,
		c.NameRU, c.ID, c.Seller, rating, s.Price[d]/100, avail, c.Seller)
	return `<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>` + c.NameRU + ` купить на OZON</title>` +
		`<script type="application/ld+json">` + ld + `</script></head><body><h1 class="title">` + c.NameRU + `</h1>` +
		fmt.Sprintf(`<div class="price"><span class="current">%s ₽</span></div>`, fmtRub(s.Price[d], "ru")) +
		`<div class="seller">` + c.Seller + `</div>` + left + `</body></html>`
}

const ozonRobots = "User-agent: *\nDisallow: /my/\nDisallow: /cart\nDisallow: /search/\n"
const wbRobots = "User-agent: *\nAllow: /cards/\n"

// fmtRub formats kopecks as whole rubles with a thin group separator: 12 990.
func fmtRub(k int64, lang string) string {
	neg := k < 0
	if neg {
		k = -k
	}
	s := fmt.Sprintf("%d", k/100)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	sep := " "
	if lang == "en" {
		sep = ","
	}
	r := strings.Join(out, sep)
	if neg {
		r = "−" + r
	}
	return r
}

// ---- visitor-added competitors ----

// ParseRef: a WB/Ozon link, a WB article or wb:<id> / ozon:<sku> → (market, id), via the marketplaces module.
func ParseRef(ref string) (market, id string, ok bool) {
	r, err := marketplaces.ParseProductRef(ref)
	if err != nil {
		return "", "", false
	}
	if r.Marketplace == marketplaces.MarketplaceOzon {
		return MarketOzon, r.ID, true
	}
	return MarketWB, r.ID, true
}

var addedSellers = []string{"Маркет Сити", "ТоварыДляВас", "Склад24", "Лучшая цена", "Домашний уют"}

// NewAddedCompetitor simulates a card the visitor pasted: in the demo its storefront data is modelled
// (deterministic from the article), in production the same collector fetches the real public page.
func NewAddedCompetitor(market, id string, p *Product) *Competitor {
	r := rand.New(rand.NewSource(seedOf("added", market, id)))
	i := r.Intn(len(addedSellers))
	pct := int64(85 + r.Intn(31))
	c := &Competitor{ID: id, Market: market, ProductID: p.ID, Seller: addedSellers[i],
		NameRU: p.ShortRU + ", аналог", NameEN: p.ShortEN + ", similar",
		Base: p.InitialPrice * pct / 100, BaseStock: int64(10 + r.Intn(200)), Rating: rt(float64(40+r.Intn(10)) / 10),
		Reviews: int64(r.Intn(900)), Added: true}
	return c
}
