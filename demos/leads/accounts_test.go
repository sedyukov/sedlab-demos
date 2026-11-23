package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	crm "github.com/sedyukov/sedlab-modules/modules/crm/go"
	payments "github.com/sedyukov/sedlab-modules/modules/payments/go"
)

// RU edition with a "live" CRM: signed-in visitors get live leads within the daily limit, then the sandbox.
func TestAccountsGatesLiveLeads(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	prof := h.app.Profiles["ru"]
	liveCRM := crm.NewMock() // stands in for a real amoCRM
	prof.shared.CRM = liveCRM
	prof.Modes.CRM = "live"
	acc, err := accounts.New(accounts.Config{Provider: "mock", Demo: "leads", MockAllowLive: true,
		MockLimits: &accounts.LimitsConfig{Resources: map[string]accounts.ResourceLimit{
			"amocrm": {Kind: accounts.KindFree, Unit: accounts.UnitRun, PerUserDay: 2}}, Demos: map[string][]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	h.app.Acc = acc
	tok, err := acc.Mock().IssueToken(accounts.IssueTokenInput{UserID: "u1", SID: "s1", Prov: "max"})
	if err != nil {
		t.Fatal(err)
	}
	s := sid(30)
	submit := func(sub, token string) *Lead {
		b, _ := json.Marshal(form(s, "ru", sub))
		req, _ := http.NewRequest("POST", h.srv.URL+"/api/leads", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "sl_auth", Value: token})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		return h.waitFor(s, out["lead_id"].(string), func(l *Lead) bool { return l.Status == "awaiting_payment" })
	}

	anon := submit("a0", "")
	if anon.Live {
		t.Fatal("signed-out visitor must stay in the sandbox")
	}
	l1, l2, l3 := submit("a1", tok.Token), submit("a2", tok.Token), submit("a3", tok.Token)
	if !l1.Live || !l2.Live || l3.Live {
		t.Fatalf("live flags: %v %v %v (limit 2/day)", l1.Live, l2.Live, l3.Live)
	}
	if _, err := liveCRM.GetDeal(context.Background(), "deal_2"); err != nil {
		t.Fatalf("live CRM must hold 2 deals: %v", err)
	}
	if _, err := liveCRM.GetDeal(context.Background(), "deal_3"); err == nil {
		t.Fatal("the 3rd lead must not reach the live CRM")
	}
	var msgs []string
	for _, e := range h.app.Hub.Events(s) {
		if e.Step == "accounts.live" {
			msgs = append(msgs, e.Message)
		}
	}
	all := strings.Join(msgs, " | ")
	for _, want := range []string{"войдите", "Живой режим", "Живой лимит на сегодня исчерпан"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in %q", want, all)
		}
	}
	// live lead pays through its own stack; the sandbox mock signature still routes correctly
	h.post("/api/leads/"+l1.ID+"/pay", map[string]any{"session": s})
	h.waitFor(s, l1.ID, func(l *Lead) bool {
		return l.Status == "paid" && l.DealStage == "Успешно" || l.DealStage == "Won"
	})

	// banner endpoint
	req, _ := http.NewRequest("GET", h.srv.URL+"/api/mode?profile=ru", nil)
	req.AddCookie(&http.Cookie{Name: "sl_auth", Value: tok.Token})
	resp, _ := http.DefaultClient.Do(req)
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if m["gated"] != true || m["live"] != false || !strings.Contains(m["message"].(string), "исчерпан") {
		t.Fatalf("mode: %v", m)
	}
}

// Without live keys nothing is gated: the RU edition runs in the sandbox and never calls auth for limits.
func TestAccountsNoLiveKeysNoGate(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	acc, _ := accounts.New(accounts.Config{Provider: "mock", Demo: "leads"})
	h.app.Acc = acc
	if h.app.Gated(h.app.Profiles["ru"]) {
		t.Fatal("no live CRM → not gated")
	}
	l := h.submitAndWait(sid(31), "ru", "x")
	if l.Live {
		t.Fatal("must be sandbox")
	}
}

// A test-mode payment shop can be open to everyone (PAYMENTS_PUBLIC=1): the sandbox stack pays through it,
// the page shows payments as real, the CRM stays simulated and nothing is gated.
func TestPublicTestPayments(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	acc, _ := accounts.New(accounts.Config{Provider: "mock", Demo: "leads"})
	h.app.Acc = acc
	p := h.app.Profiles["ru"]
	live := payments.NewMock(payments.MockConfig{}) // stands in for Robokassa test mode
	p.shared.Pay, p.payPublic, p.PayName = live, true, "Robokassa"
	if h.app.Gated(p) {
		t.Fatal("payments alone must not gate")
	}
	s := sid(33)
	l := h.submitAndWait(s, "ru", "pp-1")
	if l.Live || h.app.stackFor(h.app.Sessions.Peek(s), l).Pay != live {
		t.Fatal("sandbox stack must pay through the public test shop")
	}
	if m := p.PublicModes(); m.Payments != "live" || m.CRM != "mock" {
		t.Fatalf("modes %+v", m)
	}
	p.payPublic = false
	if m := p.PublicModes(); m.Payments != "mock" {
		t.Fatal("without PAYMENTS_PUBLIC the sandbox stays simulated")
	}
}

// Robokassa test shop open to everyone: the signed link goes to the visitor, ResultURL is answered with
// exactly "OK<InvId>" (text/plain, duplicates too), the deal is won, the CRM stays simulated.
func TestRobokassaPublicResultURL(t *testing.T) {
	h := newEditionHarness(t, nil, EditionRU)
	acc, _ := accounts.New(accounts.Config{Provider: "mock", Demo: "leads"})
	h.app.Acc = acc
	rk, err := payments.NewRobokassa(payments.RobokassaConfig{MerchantLogin: "demo", Password1: "p1", Password2: "p2", Hash: "sha256", Test: true})
	if err != nil {
		t.Fatal(err)
	}
	p := h.app.Profiles["ru"]
	p.shared.Pay, p.payPublic, p.PayName = rk, true, "Robokassa"
	p.Modes.Payments = "live"
	s := sid(34)
	l := h.submitAndWait(s, "ru", "rk-1")
	u, err := url.Parse(l.ConfirmationURL)
	if err != nil || !strings.Contains(u.Host, "robokassa") || u.Query().Get("IsTest") != "1" || l.PaymentID == "" {
		t.Fatalf("payment link %q id %q", l.ConfirmationURL, l.PaymentID)
	}
	q := u.Query()
	shp := map[string]string{}
	form := url.Values{"OutSum": {q.Get("OutSum")}, "InvId": {q.Get("InvId")}}
	for k, v := range q {
		if strings.HasPrefix(k, "Shp_") {
			shp[k] = v[0]
			form.Set(k, v[0])
		}
	}
	form.Set("SignatureValue", payments.RobokassaSignature(sha256.New, payments.RobokassaResultBase(q.Get("OutSum"), q.Get("InvId"), "p2", shp)))
	for i := 0; i < 2; i++ { // second delivery is a duplicate: same reply
		resp, err := http.Post(h.srv.URL+"/webhooks/payments/ru", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(b) != "OK"+q.Get("InvId") || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
			t.Fatalf("delivery %d: %d %q %q", i, resp.StatusCode, b, resp.Header.Get("Content-Type"))
		}
	}
	h.waitFor(s, l.ID, func(l *Lead) bool { return l.DealStage == "Won" && l.Status == "paid" })
	form.Set("SignatureValue", strings.Repeat("0", 64))
	resp, _ := http.Post(h.srv.URL+"/webhooks/payments/ru", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if resp.StatusCode != 401 {
		t.Fatalf("forged ResultURL: %d", resp.StatusCode)
	}
}
