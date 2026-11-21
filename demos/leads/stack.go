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
	Key         string   `json:"key"`  // ru | ru_b24 | intl
	Lang        string   `json:"lang"` // ru | en: texts of notifications and labels
	Currency    string   `json:"currency"`
	Plans       []Plan   `json:"plans"`
	CRMName     string   `json:"crm"`      // amoCRM | HubSpot
	PayName     string   `json:"payments"` // YooKassa | Stripe
	SheetsName  string   `json:"sheets"`
	NotifyChans []string `json:"notify"` // telegram, email
	Modes       Modes    `json:"modes"`

	env        string // env prefix: RU_ / INTL_
	shared     *Stack // live providers shared by all sessions (nil fields → per-session mock)
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
	CRM      crm.Provider
	Pay      payments.Provider
	Sheets   sheets.Sheets
	Notify   *notify.Client
	crmMock  *crm.Mock
	payMock  *payments.Mock
	SheetID  string
	SheetTab string
	once     sync.Once
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
	intl := &Profile{Key: "intl", Lang: "en", Currency: "USD", CRMName: "HubSpot", PayName: "Stripe", SheetsName: "Google Sheets",
		NotifyChans: []string{"telegram", "email"}, env: "INTL_", Plans: intlPlans()}
	var ps map[string]*Profile
	switch edition {
	case EditionRU:
		ps = map[string]*Profile{
			"ru": {Key: "ru", Lang: "ru", Currency: "RUB", CRMName: "amoCRM", PayName: "ЮKassa", SheetsName: "Яндекс Диск",
				NotifyChans: []string{"telegram", "max"}, env: "RU_", Plans: ruPlans()},
			"ru_b24": {Key: "ru_b24", Lang: "ru", Currency: "RUB", CRMName: "Битрикс24", PayName: "ЮKassa", SheetsName: "Яндекс Диск",
				NotifyChans: []string{"telegram", "max"}, env: "RU_B24_", Plans: ruPlans()},
		}
	case EditionIntl:
		ps = map[string]*Profile{"intl": intl}
	case EditionCombined:
		ps = map[string]*Profile{
			"ru": {Key: "ru", Lang: "ru", Currency: "RUB", CRMName: "amoCRM", PayName: "YooKassa", SheetsName: "Google Sheets",
				NotifyChans: []string{"telegram"}, env: "RU_", Plans: ruPlans()},
			"intl": intl,
		}
	default:
		return nil, fmt.Errorf("EDITION must be ru, intl or empty, got %q", edition)
	}
	for _, p := range ps {
		p.shared = &Stack{}
		p.sheetTab = "Leads"
		p.sheetID = "demo-sheet"
		if getenv == nil {
			getenv = os.LookupEnv
		}
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
		}
		p.Modes = Modes{CRM: mode(p.shared.CRM != nil), Payments: mode(p.shared.Pay != nil),
			Sheets: mode(p.shared.Sheets != nil), Notify: mode(p.shared.Notify != nil)}
	}
	return ps, nil
}

// NewStack returns providers for one session: live ones are shared, the rest are fresh mocks,
// so every visitor gets an isolated sandbox (own CRM, sheet, payments, inbox).
func (p *Profile) NewStack(checkoutURL string) *Stack {
	s := &Stack{CRM: p.shared.CRM, Pay: p.shared.Pay, Sheets: p.shared.Sheets, Notify: p.shared.Notify, SheetID: p.sheetID, SheetTab: p.sheetTab}
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

// SheetHeader is row 1 of the leads sheet.
var SheetHeader = []any{"Lead ID", "Date (UTC)", "Name", "Email", "Phone", "Company", "Plan", "Amount", "CRM deal", "Status", "Payment"}

// ensureHeader writes the header once per mock sheet (live sheets are expected to have it).
func (s *Stack) ensureHeader(ctx context.Context) error {
	var err error
	s.once.Do(func() {
		if _, isMock := s.Sheets.(*sheets.MockSheets); !isMock {
			return
		}
		_, err = s.Sheets.AppendRows(ctx, sheets.AppendRowsInput{SheetID: s.SheetID, Tab: s.SheetTab, Rows: [][]any{SheetHeader}})
	})
	return err
}

func (p *Profile) Plan(key string) (Plan, bool) {
	for _, pl := range p.Plans {
		if pl.Key == key {
			return pl, true
		}
	}
	return Plan{}, false
}
