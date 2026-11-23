package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	notify "github.com/sedyukov/sedlab-modules/modules/notify/go"
	notifykit "github.com/sedyukov/sedlab-modules/modules/notify/go/kit"
	sheets "github.com/sedyukov/sedlab-modules/modules/sheets-storage/go"
)

// "Put the key in, redeploy": a provider switches on by its key alone, an explicit *_PROVIDER wins.
func TestAutoProvidersByKey(t *testing.T) {
	env := map[string]string{
		"RU_B24_CRM_BITRIX24_WEBHOOK_URL":      "https://example.bitrix24.ru/rest/1/abc123/",
		"RU_PAYMENTS_ROBOKASSA_MERCHANT_LOGIN": "demo", "RU_PAYMENTS_ROBOKASSA_PASSWORD1": "p1", "RU_PAYMENTS_ROBOKASSA_PASSWORD2": "p2",
		"RU_PAYMENTS_ROBOKASSA_TEST": "true", "RU_PAYMENTS_PUBLIC": "1",
		"RU_NOTIFY_MAX_BOT_TOKEN": "tok", "RU_NOTIFY_MAX_TO": "-1001", "RU_NOTIFY_PUBLIC_URL": "https://max.ru/join/demo",
	}
	ps, err := LoadProfiles(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, EditionRU)
	if err != nil {
		t.Fatal(err)
	}
	ru, b24 := ps["ru"], ps["ru_b24"]
	if b24.Modes.CRM != "live" || ru.Modes.CRM != "mock" {
		t.Fatalf("crm modes ru=%s b24=%s", ru.Modes.CRM, b24.Modes.CRM)
	}
	if ru.Modes.Payments != "live" || ru.PayName != "Robokassa" || !ru.payPublic || b24.Modes.Payments != "mock" {
		t.Fatalf("payments: %+v %q", ru.Modes, ru.PayName)
	}
	if ru.Modes.Notify != "live" || len(ru.NotifyLive) != 1 || ru.NotifyLive[0] != "max" || ru.recipients["max"] != "-1001" || ru.Links["notify"] == "" {
		t.Fatalf("notify: %+v live=%v links=%v", ru.Modes, ru.NotifyLive, ru.Links)
	}
	env["RU_PAYMENTS_PROVIDER"] = "mock" // explicit beats the key
	ps, _ = LoadProfiles(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, EditionRU)
	if ps["ru"].Modes.Payments != "mock" {
		t.Fatal("explicit provider must win")
	}
}

// A public live sheet and channel are shared by every visitor: rows carry the alias and no typed text,
// keys are unique across visitors, and each visitor's panels show only their own rows and messages.
func TestSharedSheetAndChannelArePrivatePerVisitor(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	acc, _ := accounts.New(accounts.Config{Provider: "mock", Demo: "leads"})
	h.app.Acc = acc
	p := h.app.Profiles["ru"]
	nt, _ := notify.FromEnv(notifykit.Env{Prefix: "NOTIFY_", Lookup: func(string) (string, bool) { return "", false }})
	p.shared.Sheets, p.shPublic, p.sheetID = sheets.NewMockSheets(), true, "demo-shared"
	p.shared.Notify, p.ntPublic = nt, true
	a, b := sid(41), sid(42)
	la := h.submitAndWait(a, "ru", "sh-a")
	lb := h.submitAndWait(b, "ru", "sh-b")
	if la.ID != lb.ID {
		t.Fatalf("both visitors start at L-001: %s %s", la.ID, lb.ID)
	}
	for _, s := range []string{a, b} {
		resp, err := http.Get(h.srv.URL + "/api/state?s=" + s + "&profile=ru")
		if err != nil {
			t.Fatal(err)
		}
		var st struct {
			Sheet    [][]string `json:"sheet"`
			Messages []Message  `json:"messages"`
			Modes    Modes      `json:"modes"`
		}
		json.NewDecoder(resp.Body).Decode(&st)
		resp.Body.Close()
		if len(st.Sheet) != 2 || !strings.HasPrefix(st.Sheet[1][0], "L-001·") || st.Sheet[1][2] != "Клиент №1" || st.Sheet[1][5] != "-" {
			t.Fatalf("%s sheet: %v", s, st.Sheet)
		}
		if len(st.Messages) != 2 || st.Modes.Sheets != "live" || st.Modes.Notify != "live" {
			t.Fatalf("%s messages %d modes %+v", s, len(st.Messages), st.Modes)
		}
	}
	all, _ := p.shared.Sheets.ReadRows(context.Background(), sheets.ReadRowsInput{SheetID: "demo-shared", Tab: p.sheetTab})
	if len(all.Rows) != 3 || all.Rows[1][0] == all.Rows[2][0] {
		t.Fatalf("shared sheet: %v", all.Rows)
	}
	for _, r := range all.Rows[1:] {
		if strings.Contains(strings.Join(r, " "), "Ivan") || strings.Contains(strings.Join(r, " "), "@") {
			t.Fatalf("typed data in a shared sheet: %v", r)
		}
	}
	// payment updates the visitor's own row (found by the unique key)
	h.post("/api/leads/"+la.ID+"/pay", map[string]any{"session": a})
	h.waitEvent(a, "sheets.update_row", "ok", 1)
}
