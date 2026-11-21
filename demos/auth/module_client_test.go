package main

// Contract: the real accounts module client (provider http) against this service.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
)

func TestModuleHTTPClientAgainstService(t *testing.T) {
	app, srv := newTestApp(t, nil)
	tok := devSignup(t, srv, client(), "70")
	cl, err := accounts.New(accounts.Config{Provider: "http", AuthURL: srv.URL, PublicKey: app.signer.PublicKey(),
		Issuer: app.cfg.Issuer, ServiceKey: svcKey, Demo: "voice"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, err := cl.VerifyToken(ctx, accounts.VerifyTokenInput{Token: tok})
	if err != nil || id.UserID == "" {
		t.Fatalf("verify: %v", err)
	}
	if m := cl.Mode(ctx, accounts.ModeInput{Token: tok, Resource: "gigachat", HasKey: true}); !m.Live || m.ResetsAt == "" {
		t.Fatalf("mode: %+v", m)
	}
	if m := cl.Mode(ctx, accounts.ModeInput{Token: tok, Resource: "openai", HasKey: true}); m.Live {
		t.Fatalf("openai must not be live in RU: %+v", m)
	}
	calls := 0
	for i := 0; i < 3; i++ {
		_, m, err := accounts.LiveCall(ctx, cl, accounts.LiveCallInput{Token: tok, Resource: "gigachat", IdempotencyKey: "lc" + string(rune('a'+i))},
			func(context.Context) (string, error) { calls++; return "ok", nil })
		if err != nil || !m.Live {
			t.Fatalf("live call %d: %+v %v", i, m, err)
		}
	}
	_, m, _ := accounts.LiveCall(ctx, cl, accounts.LiveCallInput{Token: tok, Resource: "gigachat", IdempotencyKey: "lcd"},
		func(context.Context) (string, error) { calls++; return "ok", nil })
	if m.Live || m.Reason != "user_daily" || calls != 3 {
		t.Fatalf("4th call must go to mock: %+v calls=%d", m, calls)
	}
	// provider failure → refund → the unit is back
	app2, srv2 := newTestApp(t, nil)
	tok2 := devSignup(t, srv2, client(), "71")
	cl2, _ := accounts.New(accounts.Config{Provider: "http", AuthURL: srv2.URL, PublicKey: app2.signer.PublicKey(),
		Issuer: app2.cfg.Issuer, ServiceKey: svcKey, Demo: "voice"})
	_, _, err = accounts.LiveCall(ctx, cl2, accounts.LiveCallInput{Token: tok2, Resource: "gigachat", IdempotencyKey: "f1"},
		func(context.Context) (string, error) { return "", errors.New("provider down") })
	if err == nil {
		t.Fatal("fn error must be returned")
	}
	rem, err := cl2.Remaining(ctx, accounts.RemainingInput{Token: tok2})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rem.Resources {
		if r.Resource == "gigachat" && r.UserLeft != 3 {
			t.Fatalf("refund not applied: %+v", r)
		}
	}
	// token from a request (cookie) works the same
	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "sl_auth", Value: tok2})
	if m := cl2.ModeForRequest(ctx, req, "gigachat", true); !m.Live {
		t.Fatalf("mode for request: %+v", m)
	}
}
