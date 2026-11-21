package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

const svcKey = "voice-service-key-0123456789abcdef"

func newTestApp(t *testing.T, extra map[string]string) (*App, *httptest.Server) {
	t.Helper()
	seed, _, _ := GenerateKeypair()
	envs := map[string]string{
		"AUTH_DEV": "1", "DATA_DIR": t.TempDir(), "AUTH_SIGNING_KEY": seed, "AUTH_SVC_VOICE": svcKey,
		"AUTH_SVC_ADMIN": "admin-service-key-0123456789abcdef", "AUTH_MAX_BOT_USERNAME": "sedlab_login_bot",
		"MAX_PROVIDER": "mock", "NOTIFY_MAX_PROVIDER": "mock", "AUTH_OWNER_MAX_USER_ID": "777",
	}
	for k, v := range extra {
		envs[k] = v
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Routes())
	t.Cleanup(srv.Close)
	app.cfg.PublicURL = srv.URL
	return app, srv
}

func client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func cookieVal(c *http.Client, base, name string) string {
	u, _ := url.Parse(base)
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

// devSignup: dev login → consent → signed in. Returns the session token.
func devSignup(t *testing.T, srv *httptest.Server, c *http.Client, id string) string {
	t.Helper()
	r, err := c.Get(srv.URL + "/dev/login?provider=max&id=" + id)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if !strings.HasSuffix(r.Request.URL.Path, "/consent") && cookieVal(c, srv.URL, cookieAuth) == "" {
		t.Fatalf("expected consent page, got %s", r.Request.URL)
	}
	if cookieVal(c, srv.URL, cookieAuth) == "" {
		r, err = c.PostForm(srv.URL+"/consent", url.Values{"agree_pd": {"1"}, "agree_terms": {"1"}})
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
	}
	tok := cookieVal(c, srv.URL, cookieAuth)
	if tok == "" {
		t.Fatal("no session cookie after consent")
	}
	return tok
}

func internalCall(t *testing.T, srv *httptest.Server, key, op string, body map[string]any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+"/internal/v1/"+op, strings.NewReader(string(b)))
	req.Header.Set("X-Service-Key", key)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(r.Body).Decode(&out)
	return r.StatusCode, out
}

func TestTokenRoundTripAndTamper(t *testing.T) {
	seed, _, _ := GenerateKeypair()
	s, _ := NewSigner(seed, "auth.demo.sedlab.dev")
	tok := s.Issue("U1", "S1", "max", 1000, 2000)
	c, err := s.Verify("Bearer "+tok, 1500)
	if err != nil || c.Sub != "U1" || c.Sid != "S1" || c.Prov != "max" {
		t.Fatalf("verify: %v %+v", err, c)
	}
	if _, err := s.Verify(tok, 2000+clockSkew+1); err != errTokenExpired {
		t.Fatalf("want expired, got %v", err)
	}
	parts := strings.Split(tok, ".")
	forged := b64.EncodeToString([]byte(`{"iss":"auth.demo.sedlab.dev","sub":"ADMIN","sid":"S1","prov":"max","iat":1000,"exp":2000,"v":1}`))
	if _, err := s.Verify(parts[0]+"."+forged+"."+parts[2], 1500); err != errTokenSignature {
		t.Fatalf("forged payload accepted: %v", err)
	}
	none := b64.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	if _, err := s.Verify(none+"."+parts[1]+".", 1500); err == nil {
		t.Fatal("alg none accepted")
	}
	other, _ := NewSigner(func() string { k, _, _ := GenerateKeypair(); return k }(), "auth.demo.sedlab.dev")
	if _, err := other.Verify(tok, 1500); err != errTokenSignature {
		t.Fatalf("other key accepted: %v", err)
	}
	if strings.Contains(string(mustB64(parts[1])), "name") {
		t.Fatal("token must not carry personal data")
	}
}

func mustB64(s string) []byte { b, _ := b64.DecodeString(s); return b }

// Store contract (limits, idempotency, refund, rollover, concurrency, delete keeps global counters) is the
// module's storetest — see store_module_test.go.

func TestMigrateIdempotent(t *testing.T) {
	app, _ := newTestApp(t, nil)
	if err := app.store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSignupConsumeLogoutFlow(t *testing.T) {
	_, srv := newTestApp(t, nil)
	c := client()
	tok := devSignup(t, srv, c, "42")

	r, _ := c.Get(srv.URL + "/api/v1/me")
	var me map[string]any
	_ = json.NewDecoder(r.Body).Decode(&me)
	r.Body.Close()
	if me["authenticated"] != true || me["name"] != "Dev 42" {
		t.Fatalf("me: %v", me)
	}

	if code, _ := internalCall(t, srv, "wrong-key-wrong-key-wrong-key", "check", map[string]any{"token": tok, "resource": "gigachat"}); code != 401 {
		t.Fatalf("bad key: %d", code)
	}
	if code, _ := internalCall(t, srv, svcKey, "check", map[string]any{"token": tok, "demo": "booking", "resource": "gigachat"}); code != 403 {
		t.Fatalf("other demo: %d", code)
	}
	for i := 0; i < 3; i++ {
		code, d := internalCall(t, srv, svcKey, "consume", map[string]any{"token": tok, "resource": "gigachat", "idempotency_key": fmt.Sprint("r", i)})
		if code != 200 || d["allowed"] != true {
			t.Fatalf("consume %d: %d %v", i, code, d)
		}
	}
	_, d := internalCall(t, srv, svcKey, "consume", map[string]any{"token": tok, "resource": "gigachat", "idempotency_key": "r3"})
	if d["allowed"] != false || d["reason"] != "user_daily" {
		t.Fatalf("limit: %v", d)
	}
	_, d = internalCall(t, srv, svcKey, "check", map[string]any{"token": tok, "resource": "yandex-stt", "units": 61})
	if d["code"] != "invalid_argument" {
		t.Fatalf("audio over 60s must be rejected: %v", d)
	}
	_, d = internalCall(t, srv, svcKey, "remaining", map[string]any{"token": tok})
	if d["authenticated"] != true {
		t.Fatalf("remaining: %v", d)
	}

	r, _ = c.PostForm(srv.URL+"/logout", nil)
	r.Body.Close()
	_, d = internalCall(t, srv, svcKey, "consume", map[string]any{"token": tok, "resource": "yandex-llm", "idempotency_key": "after-logout"})
	if d["reason"] != "unauthenticated" {
		t.Fatalf("revoked session still live: %v", d)
	}
}

func TestAccountDeleteAndExport(t *testing.T) {
	app, srv := newTestApp(t, nil)
	c := client()
	tok := devSignup(t, srv, c, "43")
	r, _ := c.Get(srv.URL + "/account/export")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(b), `"provider_uid":"43"`) {
		t.Fatalf("export: %s", b)
	}
	r, _ = c.PostForm(srv.URL+"/account/delete", url.Values{"confirm": {"1"}})
	r.Body.Close()
	c2, _ := app.signer.Verify(tok, app.now())
	if _, err := app.store.GetUser(context.Background(), c2.Sub); err != errNotFound {
		t.Fatal("account not deleted")
	}
	if _, d := internalCall(t, srv, svcKey, "check", map[string]any{"token": tok, "resource": "gigachat"}); d["reason"] != "unauthenticated" {
		t.Fatalf("deleted account still live: %v", d)
	}
}

func TestMaxDeepLinkLogin(t *testing.T) {
	app, srv := newTestApp(t, nil)
	c := client()
	r, _ := c.Get(srv.URL + "/login?return=" + url.QueryEscape("https://voice.demo.sedlab.dev/x"))
	r.Body.Close()
	r, _ = c.Post(srv.URL+"/api/login/start", "application/json", strings.NewReader(`{"provider":"max"}`))
	var st map[string]any
	_ = json.NewDecoder(r.Body).Decode(&st)
	r.Body.Close()
	nonce, _ := st["nonce"].(string)
	if nonce == "" || !strings.HasPrefix(st["link"].(string), "https://max.ru/sedlab_login_bot?start=login_") {
		t.Fatalf("start: %v", st)
	}
	poll := func(cl *http.Client) map[string]any {
		r, _ := cl.Get(srv.URL + "/api/login/poll?nonce=" + nonce)
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		r.Body.Close()
		return p
	}
	if p := poll(c); p["status"] != "pending" {
		t.Fatalf("poll before confirm: %v", p)
	}
	hook := func(body string) {
		r, _ := http.Post(srv.URL+"/max/webhook", "application/json", strings.NewReader(body))
		r.Body.Close()
	}
	ts := time.Now().UnixMilli()
	hook(fmt.Sprintf(`{"update_type":"bot_started","timestamp":%d,"chat_id":5,"user":{"user_id":4242,"first_name":"Anna"},"payload":"login_%s"}`, ts, nonce))
	// another browser can't redeem the nonce
	if p := poll(client()); p["status"] == "ok" || p["status"] == "consent" {
		t.Fatalf("foreign browser redeemed: %v", p)
	}
	hook(fmt.Sprintf(`{"update_type":"message_callback","timestamp":%d,"callback":{"callback_id":"cb1","payload":"login:%s","user":{"user_id":4242,"first_name":"Anna","username":"anna"}}}`, ts+1, nonce))
	p := poll(c)
	if p["status"] != "consent" {
		t.Fatalf("poll after confirm: %v", p)
	}
	r, _ = c.PostForm(srv.URL+"/consent", url.Values{"agree_pd": {"1"}, "agree_terms": {"1"}})
	r.Body.Close()
	tok := cookieVal(c, srv.URL, cookieAuth)
	cl, err := app.signer.Verify(tok, app.now())
	if err != nil || cl.Prov != "max" {
		t.Fatalf("token: %v", err)
	}
	u, _ := app.store.GetUser(context.Background(), cl.Sub)
	if u.DisplayName != "Anna" || u.Username != "anna" {
		t.Fatalf("user: %+v", u)
	}
	lead, err := app.store.LeadByUser(context.Background(), u.ID)
	if err != nil || lead.FirstDemo != "voice" {
		t.Fatalf("lead: %+v %v", lead, err)
	}
	app.notifyLeads(context.Background())
	if l, _ := app.store.LeadByUser(context.Background(), u.ID); l.NotifiedAt == 0 {
		t.Fatal("lead not notified")
	}
	// replayed nonce is dead
	if p := poll(c); p["status"] == "consent" || p["status"] == "ok" {
		t.Fatalf("nonce reused: %v", p)
	}
}

func TestYandexOAuthFlow(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "good" || r.Form.Get("client_secret") != "ysecret" {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"access_token":"AT"}`)
		case "/info":
			if r.Header.Get("Authorization") != "OAuth AT" {
				w.WriteHeader(401)
				return
			}
			io.WriteString(w, `{"id":"9001","login":"ivan","first_name":"Иван","default_email":"x@y"}`)
		}
	}))
	defer fake.Close()
	app, srv := newTestApp(t, map[string]string{"ACCOUNTS_YANDEX_CLIENT_ID": "yid", "ACCOUNTS_YANDEX_CLIENT_SECRET": "ysecret",
		"ACCOUNTS_YANDEX_AUTH_URL": fake.URL + "/authorize", "ACCOUNTS_YANDEX_TOKEN_URL": fake.URL + "/token", "ACCOUNTS_YANDEX_INFO_URL": fake.URL + "/info"})
	c := client()
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if strings.HasPrefix(req.URL.String(), fake.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	r, _ := c.Get(srv.URL + "/login/oauth/yandex/start")
	r.Body.Close()
	loc, _ := url.Parse(r.Header.Get("Location"))
	if loc.Query().Get("scope") != "login:info" || loc.Query().Get("state") == "" {
		t.Fatalf("authorize url: %s", loc)
	}
	// wrong state → rejected
	r, _ = c.Get(srv.URL + "/login/oauth/yandex/callback?code=good&state=evil")
	r.Body.Close()
	if strings.HasSuffix(r.Request.URL.Path, "/consent") {
		t.Fatal("state not checked")
	}
	r, _ = c.Get(srv.URL + "/login/oauth/yandex/start")
	r.Body.Close()
	loc, _ = url.Parse(r.Header.Get("Location"))
	r, _ = c.Get(srv.URL + "/login/oauth/yandex/callback?code=good&state=" + loc.Query().Get("state"))
	r.Body.Close()
	if !strings.HasSuffix(r.Request.URL.Path, "/consent") {
		t.Fatalf("expected consent, at %s", r.Request.URL)
	}
	r, _ = c.PostForm(srv.URL+"/consent", url.Values{"agree_pd": {"1"}, "agree_terms": {"1"}})
	r.Body.Close()
	cl, err := app.signer.Verify(cookieVal(c, srv.URL, cookieAuth), app.now())
	if err != nil || cl.Prov != "yandex" {
		t.Fatalf("yandex login: %v", err)
	}
	data, _ := app.store.ExportUser(context.Background(), cl.Sub)
	if b, _ := json.Marshal(data); strings.Contains(string(b), "x@y") {
		t.Fatal("email must not be stored")
	}
}

func TestProvidersConfigOnlyRussian(t *testing.T) {
	app, _ := newTestApp(t, map[string]string{"AUTH_LOGIN_PROVIDERS_RU": "telegram,google,max", "AUTH_LOGIN_PROVIDERS_EN": "google"})
	ru := app.providers.For("ru")
	if len(ru) != 1 || ru[0].ID != "max" {
		t.Fatalf("ru methods: %+v", ru)
	}
	if len(app.providers.For("en")) != 0 || app.providers.Enabled("telegram") || app.providers.Enabled("google") {
		t.Fatal("foreign provider enabled")
	}
	if app.providers.Enabled("yandex") {
		t.Fatal("yandex without credentials must be off")
	}
}

func TestSafeReturn(t *testing.T) {
	app, _ := newTestApp(t, nil)
	app.cfg.Dev = false
	for in, want := range map[string]string{
		"https://voice.demo.sedlab.dev/a?b=1":    "https://voice.demo.sedlab.dev/a?b=1",
		"https://evil.com/":                      "/account",
		"https://voice.demo.sedlab.dev.evil.com": "/account",
		"//evil.com/x":                           "/account",
		"javascript:alert(1)":                    "/account",
		"http://voice.demo.sedlab.dev/":          "/account",
		"/account":                               "/account",
		"https://user@voice.demo.sedlab.dev/":    "/account",
	} {
		if got := app.safeReturn(in); got != want {
			t.Errorf("safeReturn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInternalIPAllowlist(t *testing.T) {
	_, srv := newTestApp(t, map[string]string{"AUTH_INTERNAL_ALLOW_IPS": "167.71.38.231"})
	if code, _ := internalCall(t, srv, svcKey, "check", map[string]any{"resource": "gigachat"}); code != 401 {
		t.Fatalf("non-allowlisted IP got %d", code)
	}
}

func TestConsentRequiresBothBoxesAndCancelStoresNothing(t *testing.T) {
	app, srv := newTestApp(t, nil)
	c := client()
	r, _ := c.Get(srv.URL + "/dev/login?provider=max&id=50")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	for _, want := range []string{`name="agree_pd"`, `name="agree_terms"`, `name="contact_ok"`, "/consent/demo", "/terms", "/consent/contact"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("consent page lacks %s", want)
		}
	}
	if regexp.MustCompile(`<input[^>]*\bchecked\b`).Match(b) {
		t.Fatal("no checkbox may be pre-ticked")
	}
	r, _ = c.PostForm(srv.URL+"/consent", url.Values{"agree_pd": {"1"}}) // terms missing
	r.Body.Close()
	if cookieVal(c, srv.URL, cookieAuth) != "" {
		t.Fatal("account created without terms acceptance")
	}
	r, _ = c.PostForm(srv.URL+"/consent", url.Values{"cancel": {"1"}})
	r.Body.Close()
	if _, err := app.store.FindIdentity(context.Background(), "max", "50"); err != errNotFound {
		t.Fatal("cancel must not create an account")
	}
	// with the optional contact box
	r, _ = c.Get(srv.URL + "/dev/login?provider=max&id=51")
	r.Body.Close()
	r, _ = c.PostForm(srv.URL+"/consent", url.Values{"agree_pd": {"1"}, "agree_terms": {"1"}, "contact_ok": {"1"}})
	r.Body.Close()
	id, _ := app.store.FindIdentity(context.Background(), "max", "51")
	u, _ := app.store.GetUser(context.Background(), id)
	if u.ConsentVersion != "demo-v2" || u.ConsentAt == 0 || u.TermsVersion != "terms-v2" || u.TermsAt == 0 || !u.ContactOK || u.ContactConsentAt == 0 {
		t.Fatalf("consents not stored: %+v", u)
	}
	r, _ = c.PostForm(srv.URL+"/account/contact", url.Values{}) // withdraw
	r.Body.Close()
	if u, _ = app.store.GetUser(context.Background(), id); u.ContactOK {
		t.Fatal("contact consent not withdrawn")
	}
}

func TestLegalPagesFilledFromEnvOnly(t *testing.T) {
	_, srv := newTestApp(t, map[string]string{"AUTH_PD_EMAIL": "pd@example.test", "AUTH_OPERATOR_OGRNIP": "<b>1</b>"})
	for _, p := range []string{"/privacy", "/privacy?lang=en", "/terms", "/terms?lang=en", "/consent/demo", "/consent/demo?lang=en", "/consent/contact", "/consent/contact?lang=en"} {
		r, err := http.Get(srv.URL + p)
		if err != nil || r.StatusCode != 200 {
			t.Fatalf("%s: %v %v", p, err, r.StatusCode)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if strings.Contains(string(b), "<b>1</b>") {
			t.Fatalf("%s: placeholder value not escaped", p)
		}
		if regexp.MustCompile(`(?:^|[^\d])\d{12,15}(?:[^\d]|$)`).Match(b) {
			t.Fatalf("%s: INN/OGRNIP-like number hardcoded (must come from env)", p)
		}
	}
	r, _ := http.Get(srv.URL + "/privacy")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(b), "pd@example.test") {
		t.Fatal("PD_EMAIL not substituted")
	}
	if !strings.Contains(string(b), "{{OPERATOR_INN}}") {
		t.Fatal("unset placeholder must stay visible")
	}
}

func TestRefundOnlyByOwningDemo(t *testing.T) {
	_, srv := newTestApp(t, map[string]string{"AUTH_SVC_BOOKING": "booking-service-key-0123456789abcd"})
	tok := devSignup(t, srv, client(), "60")
	_, d := internalCall(t, srv, svcKey, "consume", map[string]any{"token": tok, "resource": "gigachat", "idempotency_key": "z1"})
	cid, _ := d["consumption_id"].(string)
	if cid == "" {
		t.Fatalf("consume: %v", d)
	}
	if code, _ := internalCall(t, srv, "booking-service-key-0123456789abcd", "refund", map[string]any{"consumption_id": cid}); code != 404 {
		t.Fatalf("other demo refunded: %d", code)
	}
	if code, _ := internalCall(t, srv, svcKey, "refund", map[string]any{"consumption_id": cid}); code != 200 {
		t.Fatalf("own refund: %d", code)
	}
}

func TestIntlEditionNotServed(t *testing.T) {
	app, _ := newTestApp(t, nil)
	app.cfg.Dev = false
	if app.allowedOrigin("https://voice-en.demo.sedlab.dev") || !app.allowedOrigin("https://voice.demo.sedlab.dev") {
		t.Fatal("CORS: INTL edition must be refused, RU edition allowed")
	}
	if _, ok := app.limits.For("voice", "openai"); ok {
		t.Fatal("RU voice must not get live OpenAI")
	}
}

func TestStartsWithoutMaxTokenAndHidesMax(t *testing.T) {
	app, srv := newTestApp(t, map[string]string{"MAX_PROVIDER": "max", "MAX_BOT_TOKEN": "", "NOTIFY_MAX_PROVIDER": "max", "NOTIFY_MAX_BOT_TOKEN": ""})
	if app.providers.Enabled("max") {
		t.Fatal("max must be off without a token")
	}
	r, _ := http.Get(srv.URL + "/login")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if strings.Contains(string(b), `id="max-login"`) {
		t.Fatal("login page shows Max without a token")
	}
}
