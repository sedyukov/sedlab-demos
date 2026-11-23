package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	crm "github.com/sedyukov/sedlab-modules/modules/crm/go"
	crmkit "github.com/sedyukov/sedlab-modules/modules/crm/go/kit"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	notifykit "github.com/sedyukov/sedlab-modules/modules/notify/go/kit"
	payments "github.com/sedyukov/sedlab-modules/modules/payments/go"
	paykit "github.com/sedyukov/sedlab-modules/modules/payments/go/kit"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
	sheetskit "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go/kit"
)

// Plan is a price-list entry; amounts are server-side, the form only sends the plan key.
type Plan struct {
	Key    string `json:"key"`
	Amount int64  `json:"amount"` // minor units
}

// Profile is one integration configuration the demo can switch between.
type Profile struct {
	Key         string   `json:"key"`      // ru | ru_b24 | intl
	Resource    string   `json:"resource"` // accounts resource that gates live mode (CRM of the stack)
	Lang        string   `json:"lang"`     // ru | en: texts of notifications and labels
	Currency    string   `json:"currency"`
	Plans       []Plan   `json:"plans"`
	CRMName     string   `json:"crm"`      // amoCRM | HubSpot
	PayName     string   `json:"payments"` // YooKassa | Stripe
	SheetsName  string   `json:"sheets"`
	NotifyChans []string `json:"notify"` // telegram, email
	Modes       Modes    `json:"modes"`

	env        string            // env prefix: RU_ / INTL_
	shared     *Stack            // live providers shared by all sessions (nil fields → per-session mock)
	payPublic  bool              // PAYMENTS_PUBLIC=1: a test-mode shop open to every visitor (no sign-in); CRM stays gated
	shPublic   bool              // SHEETS_PUBLIC=1: the demo table is shared by everyone (rows carry aliases, never names)
	ntPublic   bool              // NOTIFY_PUBLIC=1: the demo channel is shared by everyone (messages carry aliases)
	Links      map[string]string `json:"links,omitempty"`       // sheets / notify: public read-only link (proof that it is real)
	NotifyLive []string          `json:"notify_live,omitempty"` // channels with a real provider
	sheetID    string
	sheetTab   string
	recipients map[string]string // notify channel → recipient (demo: fake team chat / inbox)
}

// Modes says which services run live (real sandbox keys) vs in-process mock.
type Modes struct {
	CRM      string `json:"crm"`
	Payments string `json:"payments"`
	Sheets   string `json:"sheets"`
	Notify   string `json:"notify"`
}

// Stack is the set of providers one session uses for one profile.
type Stack struct {
	CRM                       crm.Provider
	Pay                       payments.Provider
	Sheets                    sheets.Sheets
	Notify                    *notify.Client
	sharedSheet, sharedNotify bool // live providers shared by every visitor: the page filters to the visitor's own
	crmMock                   *crm.Mock
	payMock                   *payments.Mock
	SheetID                   string
	SheetTab                  string
	Header                    []any // row 1 in the profile's language; column 0 is the lookup key
	hdrMu                     sync.Mutex
	hdrOK                     bool // header verified/written (retried on failure, unlike sync.Once)
}

func mode(live bool) string {
	if live {
		return "live"
	}
	return "mock"
}

// Editions: one code base, the edition is picked by EDITION (owner's decision 2026-11-21).
//   - ru:   Russian only, Russian integrations (amoCRM or Bitrix24, YooKassa, Yandex Disk table, Telegram + Max), hosted in RF.
//   - intl: English only, international integrations (HubSpot, Stripe, Google Sheets, Telegram + email).
//   - "":   legacy combined page (both stacks, RU/EN switch) until the RU host is up.
const (
	EditionRU       = "ru"
	EditionIntl     = "intl"
	EditionCombined = ""
)

// ProfileOrder is the display order of the stack switch.
var ProfileOrder = []string{"ru", "ru_b24", "intl"}

func ruPlans() []Plan   { return []Plan{{"start", 500000}, {"business", 1500000}, {"pro", 4500000}} }
func intlPlans() []Plan { return []Plan{{"start", 9900}, {"business", 29900}, {"pro", 89900}} }

// LoadProfiles builds the profiles of an edition. Live providers are enabled per service by env:
// RU_CRM_PROVIDER=amocrm (+ RU_CRM_AMOCRM_*), RU_B24_CRM_PROVIDER=bitrix24, INTL_PAYMENTS_PROVIDER=stripe,
// INTL_SHEETS_PROVIDER=google_sheets (+ INTL_SHEETS_SHEET_ID), ... Everything defaults to the modules' mock providers.
func LoadProfiles(getenv func(string) (string, bool), edition string) (map[string]*Profile, error) {
	intl := &Profile{Key: "intl", Resource: "hubspot", Lang: "en", Currency: "USD", CRMName: "HubSpot", PayName: "Stripe", SheetsName: "Google Sheets",
		NotifyChans: []string{"telegram", "email"}, env: "INTL_", Plans: intlPlans()}
	var ps map[string]*Profile
	switch edition {
	case EditionRU:
		ps = map[string]*Profile{
			"ru": {Key: "ru", Resource: "amocrm", Lang: "ru", Currency: "RUB", CRMName: "amoCRM", PayName: "ЮKassa", SheetsName: "Яндекс Диск",
				NotifyChans: []string{"telegram", "max"}, env: "RU_", Plans: ruPlans()},
			"ru_b24": {Key: "ru_b24", Resource: "bitrix24", Lang: "ru", Currency: "RUB", CRMName: "Битрикс24", PayName: "ЮKassa", SheetsName: "Яндекс Диск",
				NotifyChans: []string{"telegram", "max"}, env: "RU_B24_", Plans: ruPlans()},
		}
	case EditionIntl:
		ps = map[string]*Profile{"intl": intl}
	case EditionCombined:
		ps = map[string]*Profile{
			"ru": {Key: "ru", Resource: "amocrm", Lang: "ru", Currency: "RUB", CRMName: "amoCRM", PayName: "YooKassa", SheetsName: "Google Sheets",
				NotifyChans: []string{"telegram"}, env: "RU_", Plans: ruPlans()},
			"intl": intl,
		}
	default:
		return nil, fmt.Errorf("EDITION must be ru, intl or empty, got %q", edition)
	}
	for _, p := range ps {
		p.shared = &Stack{}
		p.sheetTab = tr(p.Lang, "Leads", "Заявки")
		p.sheetID = "demo-sheet"
		if getenv == nil {
			getenv = os.LookupEnv
		}
		getenv := autoProviders(p.env, getenv)
		get := func(k string) string { v, _ := getenv(p.env + k); return strings.TrimSpace(v) }

		if v := strings.ToLower(get("CRM_PROVIDER")); v != "" && v != "mock" {
			c, err := crm.FromEnv(crmkit.Env{Prefix: p.env + "CRM_", Lookup: getenv})
			if err != nil {
				return nil, fmt.Errorf("%s crm: %w", p.Key, err)
			}
			p.shared.CRM = c
		}
		if v := strings.ToLower(get("PAYMENTS_PROVIDER")); v != "" && v != "mock" {
			c, err := payments.FromEnv(paykit.Env{Prefix: p.env + "PAYMENTS_", Lookup: getenv})
			if err != nil {
				return nil, fmt.Errorf("%s payments: %w", p.Key, err)
			}
			p.shared.Pay = c
			if n, ok := payNames[v]; ok { // the page names the provider that is really connected
				p.PayName = n
			}
			p.payPublic = isTrue(get("PAYMENTS_PUBLIC"))
		}
		if v := strings.ToLower(get("SHEETS_PROVIDER")); v != "" && v != "mock" {
			c, err := sheets.SheetsFromEnv(sheetskit.Env{Prefix: p.env + "SHEETS_", Lookup: getenv})
			if err != nil {
				return nil, fmt.Errorf("%s sheets: %w", p.Key, err)
			}
			if p.sheetID = get("SHEETS_SHEET_ID"); p.sheetID == "" {
				return nil, fmt.Errorf("%s sheets: %sSHEETS_SHEET_ID is required for a live sheet", p.Key, p.env)
			}
			if t := get("SHEETS_TAB"); t != "" {
				p.sheetTab = t
			}
			p.shared.Sheets = c
			p.shPublic = isTrue(get("SHEETS_PUBLIC"))
			if u := get("SHEETS_PUBLIC_URL"); u != "" {
				p.link("sheets", u)
			}
		}
		// demo sandbox recipients (distinct ids per channel: they are mapped back to labels on the page)
		p.recipients = map[string]string{"telegram": "100200300", "max": "100200301", "email": "manager@example.com"}
		live := false
		for _, k := range []string{"NOTIFY_TELEGRAM_PROVIDER", "NOTIFY_MAX_PROVIDER", "NOTIFY_EMAIL_PROVIDER"} {
			if v := strings.ToLower(get(k)); v != "" && v != "mock" {
				live = true
			}
		}
		if live {
			c, err := notify.FromEnv(notifykit.Env{Prefix: p.env + "NOTIFY_", Lookup: getenv})
			if err != nil {
				return nil, fmt.Errorf("%s notify: %w", p.Key, err)
			}
			if v := get("NOTIFY_TELEGRAM_TO"); v != "" {
				p.recipients["telegram"] = v
			}
			if v := get("NOTIFY_MAX_TO"); v != "" {
				p.recipients["max"] = v
			}
			if v := get("NOTIFY_EMAIL_TO"); v != "" {
				p.recipients["email"] = v
			}
			p.shared.Notify = c
			p.ntPublic = isTrue(get("NOTIFY_PUBLIC"))
			if u := get("NOTIFY_PUBLIC_URL"); u != "" {
				p.link("notify", u)
			}
			for _, ch := range p.NotifyChans {
				if v := strings.ToLower(get("NOTIFY_" + strings.ToUpper(ch) + "_PROVIDER")); v != "" && v != "mock" {
					p.NotifyLive = append(p.NotifyLive, ch)
				}
			}
		}
		p.Modes = Modes{CRM: mode(p.shared.CRM != nil), Payments: mode(p.shared.Pay != nil),
			Sheets: mode(p.shared.Sheets != nil), Notify: mode(p.shared.Notify != nil)}
	}
	return ps, nil
}

// NewStack returns providers for one session: live ones are shared, the rest are fresh mocks,
// so every visitor gets an isolated sandbox (own CRM, sheet, payments, inbox).
// forceMock: the sandbox stack of a visitor when live mode is gated by accounts (RU edition).
func (p *Profile) NewStack(checkoutURL string, forceMock bool) *Stack {
	s := &Stack{CRM: p.shared.CRM, Pay: p.shared.Pay, Sheets: p.shared.Sheets, Notify: p.shared.Notify, SheetID: p.sheetID, SheetTab: p.sheetTab}
	if forceMock {
		s = &Stack{SheetID: "demo-sheet", SheetTab: tr(p.Lang, "Leads", "Заявки")}
		if p.payPublic { // test-mode payments need no sign-in: nothing real is charged, nothing personal is stored
			s.Pay = p.shared.Pay
		}
		if p.shPublic && p.shared.Sheets != nil {
			s.Sheets, s.SheetID, s.SheetTab = p.shared.Sheets, p.sheetID, p.sheetTab
		}
		if p.ntPublic && p.shared.Notify != nil {
			s.Notify = p.shared.Notify
		}
	}
	s.sharedSheet = s.Sheets != nil && s.Sheets == p.shared.Sheets
	s.sharedNotify = s.Notify != nil && s.Notify == p.shared.Notify
	s.Header = sheetHeader(p.Lang)
	if s.CRM == nil {
		s.crmMock = crm.NewMock()
		s.CRM = s.crmMock
	}
	if s.Pay == nil {
		s.payMock = payments.NewMock(payments.MockConfig{CheckoutURL: checkoutURL})
		s.Pay = s.payMock
	}
	if s.Sheets == nil {
		s.Sheets = sheets.NewMockSheets()
	}
	if s.Notify == nil {
		s.Notify, _ = notify.FromEnv(notifykit.Env{Prefix: "NOTIFY_", Lookup: func(string) (string, bool) { return "", false }}) // all-mock client
	}
	return s
}

// SheetHeader is row 1 of the leads sheet (English); sheetHeader picks the profile's language.
var SheetHeader = []any{"Lead ID", "Date (UTC)", "Name", "Email", "Phone", "Company", "Plan", "Amount", "CRM deal", "Status", "Payment"}

var sheetHeaderRU = []any{"Заявка", "Дата (UTC)", "Имя", "Email", "Телефон", "Компания", "Тариф", "Сумма", "Сделка в CRM", "Статус", "Оплата"}

func sheetHeader(lang string) []any {
	if lang == "ru" {
		return sheetHeaderRU
	}
	return SheetHeader
}

// ensureHeader writes the header row if the sheet/file has none yet (mock, Google Sheets, Yandex Disk table):
// the payment step finds the lead's row by the "Lead ID" column. Checked once per stack.
func (s *Stack) ensureHeader(ctx context.Context) error {
	s.hdrMu.Lock()
	defer s.hdrMu.Unlock()
	if s.hdrOK {
		return nil
	}
	rows, err := s.Sheets.ReadRows(ctx, sheets.ReadRowsInput{SheetID: s.SheetID, Tab: s.SheetTab, Range: "1:1"})
	switch {
	case err == nil && len(rows.Rows) > 0 && len(rows.Rows[0]) > 0 && rows.Rows[0][0] != "":
		// header present
	case err != nil && sheetskit.CodeOf(err) != sheetskit.NotFound:
		return err
	default:
		if _, err := s.Sheets.AppendRows(ctx, sheets.AppendRowsInput{SheetID: s.SheetID, Tab: s.SheetTab, Rows: [][]any{s.Header}}); err != nil {
			return err
		}
	}
	s.hdrOK = true
	return nil
}

func (p *Profile) Plan(key string) (Plan, bool) {
	for _, pl := range p.Plans {
		if pl.Key == key {
			return pl, true
		}
	}
	return Plan{}, false
}

// payNames: display names of live payment providers (RU edition: Robokassa first, ЮKassa later).
var payNames = map[string]string{"yookassa": "ЮKassa", "robokassa": "Robokassa", "stripe": "Stripe"}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// PublicModes: what every visitor's (sandbox) stack really uses in the RU edition.
func (p *Profile) PublicModes() Modes {
	m := Modes{CRM: "mock", Payments: "mock", Sheets: "mock", Notify: "mock"}
	if p.payPublic && p.shared.Pay != nil {
		m.Payments = "live"
	}
	if p.shPublic && p.shared.Sheets != nil {
		m.Sheets = "live"
	}
	if p.ntPublic && p.shared.Notify != nil {
		m.Notify = "live"
	}
	return m
}

func (p *Profile) link(k, u string) {
	if p.Links == nil {
		p.Links = map[string]string{}
	}
	p.Links[k] = u
}

// autoRules: a provider switches on by itself when its key is present ("put the key in, redeploy").
// An explicit *_PROVIDER always wins; the first matching rule per service is used.
var autoRules = []struct{ provider, key, value string }{
	{"CRM_PROVIDER", "CRM_BITRIX24_WEBHOOK_URL", "bitrix24"},
	{"CRM_PROVIDER", "CRM_AMOCRM_TOKEN", "amocrm"},
	{"PAYMENTS_PROVIDER", "PAYMENTS_ROBOKASSA_MERCHANT_LOGIN", "robokassa"},
	{"PAYMENTS_PROVIDER", "PAYMENTS_YOOKASSA_SHOP_ID", "yookassa"},
	{"SHEETS_PROVIDER", "SHEETS_YADISK_TOKEN", "yandex_disk_table"},
	{"NOTIFY_TELEGRAM_PROVIDER", "NOTIFY_TELEGRAM_BOT_TOKEN", "telegram"},
	{"NOTIFY_MAX_PROVIDER", "NOTIFY_MAX_BOT_TOKEN", "max"},
}

func autoProviders(prefix string, getenv func(string) (string, bool)) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if v, ok := getenv(k); ok && strings.TrimSpace(v) != "" {
			return v, ok
		}
		for _, r := range autoRules {
			if k == prefix+r.provider {
				if v, _ := getenv(prefix + r.key); strings.TrimSpace(v) != "" {
					return r.value, true
				}
			}
		}
		return getenv(k)
	}
}
