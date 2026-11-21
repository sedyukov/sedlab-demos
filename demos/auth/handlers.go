package main

import (
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	maxcore "github.com/sedyukov/sedlab-modules/modules/max-core/go"
)

func (a *App) jwks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, 200, a.signer.JWKS())
}

// ---- login page -------------------------------------------------------------------------------------------

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	ret := a.safeReturn(r.URL.Query().Get("return"))
	if u, _ := a.authUser(r.Context(), tokenFromRequest(r)); u != nil {
		http.Redirect(w, r, ret, http.StatusFound)
		return
	}
	// remember return + demo for the widget callback (same-origin, short-lived)
	a.setCookie(w, "sl_ret", url.QueryEscape(ret)+"|"+url.QueryEscape(demoFromReturn(ret, a.cfg.ReturnSuffix)), pendingTTL, false)
	a.render(w, r, "login.html", map[string]any{
		"Return": ret, "Methods": a.providers.For(langOf(r)), "Dev": a.cfg.Dev, "Error": r.URL.Query().Get("error"),
	})
}

func demoFromReturn(ret, suffix string) string {
	u, err := url.Parse(ret)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if strings.HasSuffix(h, suffix) {
		return strings.TrimSuffix(h, suffix)
	}
	return ""
}

func (a *App) retCookie(r *http.Request) (ret, demo string) {
	c, err := r.Cookie("sl_ret")
	if err != nil {
		return "/account", ""
	}
	rr, dd, _ := strings.Cut(c.Value, "|")
	ret, _ = url.QueryUnescape(rr)
	demo, _ = url.QueryUnescape(dd)
	return a.safeReturn(ret), demo
}

// ---- bot deep-link login (Max) ---------------------------------------------------------

func browserHash(v string) string {
	h := sha256.Sum256([]byte("sl_login:" + v))
	return b64.EncodeToString(h[:16])
}

func (a *App) loginStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
	if in.Provider == "" {
		in.Provider = "max"
	}
	if in.Provider != "max" || !a.providers.Enabled("max") {
		moduleErr(w, 400, "invalid_argument", "bot login: provider must be max (enabled)")
		return
	}
	if a.cfg.MaxBotUser == "" && a.cfg.MaxBotLink == "https://max.ru/" {
		moduleErr(w, 400, "config", "max bot not configured")
		return
	}
	base := a.cfg.MaxBotLink + "?start="
	bind := randToken(18)
	a.setCookie(w, cookieLogin, bind, nonceTTL, false)
	n, err := a.store.CreateNonce(r.Context(), in.Provider, browserHash(bind), a.now()+nonceTTL)
	if err != nil {
		moduleErr(w, 500, "unavailable", "storage")
		return
	}
	writeJSON(w, 200, map[string]any{"nonce": n, "link": base + "login_" + n, "expires_in": nonceTTL})
}

func (a *App) loginPoll(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieLogin)
	if err != nil {
		moduleErr(w, 401, "unauthenticated", "login not started in this browser")
		return
	}
	x, err := a.store.UseNonce(r.Context(), r.URL.Query().Get("nonce"), browserHash(c.Value), a.now())
	if err != nil {
		moduleErr(w, 404, "not_found", "login expired")
		return
	}
	if x.Status != "used" {
		writeJSON(w, 200, map[string]any{"status": "pending"})
		return
	}
	a.setCookie(w, cookieLogin, "", -1, false)
	ret, demo := a.retCookie(r)
	tok, pend, err := a.finishLogin(r.Context(), pending{Prov: x.Provider, UID: x.ProviderUID, Name: x.DisplayName, User: x.Username, Lang: langOf(r), Ret: ret, Demo: demo})
	if err != nil {
		moduleErr(w, 500, "unavailable", "login failed")
		return
	}
	if pend != "" {
		a.setCookie(w, cookiePending, pend, pendingTTL, false)
		writeJSON(w, 200, map[string]any{"status": "consent", "redirect": "/consent"})
		return
	}
	a.setCookie(w, cookieAuth, tok, sessionTTL, true)
	writeJSON(w, 200, map[string]any{"status": "ok", "redirect": ret})
}

// ---- Mini App (Max initData) → Bearer token ----------------------------------------------------

func (a *App) loginWebApp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
		InitData string `json:"init_data"`
		Demo     string `json:"demo"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
		moduleErr(w, 400, "invalid_argument", "bad json")
		return
	}
	var p pending
	switch {
	case in.Provider == "max" && a.providers.Enabled("max"):
		au, err := a.max.ValidateInitData(maxcore.ValidateInitDataInput{InitData: in.InitData})
		if err != nil || au.User == nil {
			moduleErr(w, 401, "signature_invalid", "init data rejected")
			return
		}
		p = pending{Prov: "max", UID: strconv.FormatInt(au.User.ID, 10), Name: strings.TrimSpace(au.User.FirstName + " " + au.User.LastName), User: au.User.Username, Lang: au.User.LanguageCode}
	default:
		moduleErr(w, 400, "invalid_argument", "provider must be max (enabled)")
		return
	}
	p.Demo = in.Demo
	tok, pend, err := a.finishLogin(r.Context(), p)
	if err != nil {
		moduleErr(w, 500, "unavailable", "login failed")
		return
	}
	if pend != "" {
		writeJSON(w, 200, map[string]any{"need_consent": true, "pending": pend, "consent_url": a.cfg.PublicURL + "/consent/demo",
			"terms_url": a.cfg.PublicURL + "/terms", "contact_url": a.cfg.PublicURL + "/consent/contact", "privacy_url": a.cfg.PrivacyURL,
			"consent_version": a.cfg.ConsentVer, "terms_version": a.cfg.TermsVer, "contact_version": a.cfg.ContactVer})
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok, "expires_in": sessionTTL})
}

// ---- consent ----------------------------------------------------------------------------------------------

func (a *App) consentPage(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookiePending)
	p, perr := (*pending)(nil), err
	if err == nil {
		p, perr = a.openPending(c.Value)
	}
	if perr != nil {
		http.Redirect(w, r, "/login?error=expired", http.StatusSeeOther)
		return
	}
	a.render(w, r, "consent.html", map[string]any{"Name": p.Name, "ProviderLabel": providerLabel(p.Prov, langOf(r)), "Channel": contactChannel(p.Prov)})
}

func (a *App) createFromPending(r *http.Request, p *pending, contact bool) (string, error) {
	c := &Consent{Demo: a.cfg.ConsentVer, Terms: a.cfg.TermsVer}
	if contact && contactChannel(p.Prov) != "" {
		c.Contact = a.cfg.ContactVer
	}
	u, created, err := a.store.UpsertUser(r.Context(), Profile{Provider: p.Prov, ProviderUID: p.UID, DisplayName: p.Name, Username: p.User, Lang: p.Lang}, c, p.Demo, a.now())
	if err != nil {
		return "", err
	}
	if created && p.Demo != "" {
		_ = a.store.RecordVisit(r.Context(), u.ID, p.Demo, a.now())
	}
	return a.issue(r.Context(), u.ID, p.Prov)
}

func (a *App) consentSubmit(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookiePending)
	if err != nil {
		http.Redirect(w, r, "/login?error=expired", http.StatusSeeOther)
		return
	}
	p, err := a.openPending(c.Value)
	if err != nil {
		http.Redirect(w, r, "/login?error=expired", http.StatusSeeOther)
		return
	}
	if r.FormValue("cancel") == "1" { // nothing stored: the pending identity only lived in this cookie
		a.setCookie(w, cookiePending, "", -1, false)
		http.Redirect(w, r, a.safeReturn(p.Ret), http.StatusSeeOther)
		return
	}
	if r.FormValue("agree_pd") != "1" || r.FormValue("agree_terms") != "1" {
		http.Redirect(w, r, "/consent", http.StatusSeeOther)
		return
	}
	tok, err := a.createFromPending(r, p, r.FormValue("contact_ok") == "1")
	if err != nil {
		slog.Error("consent", "err", err)
		http.Error(w, "failed", 500)
		return
	}
	a.setCookie(w, cookiePending, "", -1, false)
	a.setCookie(w, cookieAuth, tok, sessionTTL, true)
	http.Redirect(w, r, a.safeReturn(p.Ret), http.StatusSeeOther)
}

// consentAPI: Mini App flow — {pending, agree_pd, agree_terms, contact_ok?} → {token}. The Mini App must show the same
// three separate checkboxes as /consent (legal/ui-texts.md §1).
func (a *App) consentAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pending    string `json:"pending"`
		AgreePD    bool   `json:"agree_pd"`
		AgreeTerms bool   `json:"agree_terms"`
		ContactOK  bool   `json:"contact_ok"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in) != nil || !in.AgreePD || !in.AgreeTerms {
		moduleErr(w, 400, "invalid_argument", "both consents (agree_pd, agree_terms) required")
		return
	}
	p, err := a.openPending(in.Pending)
	if err != nil {
		moduleErr(w, 401, "unauthenticated", "pending expired")
		return
	}
	tok, err := a.createFromPending(r, p, in.ContactOK)
	if err != nil {
		moduleErr(w, 500, "unavailable", "failed")
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok, "expires_in": sessionTTL})
}

// ---- account ----------------------------------------------------------------------------------------------

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if _, s := a.authUser(r.Context(), tokenFromRequest(r)); s != nil {
		_ = a.store.RevokeSession(r.Context(), s.Sid, a.now())
	}
	a.setCookie(w, cookieAuth, "", -1, true)
	http.Redirect(w, r, a.safeReturn(r.FormValue("return")), http.StatusSeeOther)
}

func (a *App) accountPage(w http.ResponseWriter, r *http.Request) {
	u, _ := a.authUser(r.Context(), tokenFromRequest(r))
	if u == nil {
		a.render(w, r, "account.html", map[string]any{"User": nil, "Deleted": r.URL.Query().Get("deleted") == "1"})
		return
	}
	visits, _ := a.store.Visits(r.Context(), u.ID)
	type row struct {
		Demo  string
		Items []map[string]any
	}
	var demos []row
	for _, v := range visits {
		demos = append(demos, row{Demo: v.Demo, Items: a.remaining(r, u.ID, v.Demo)})
	}
	ch := ""
	if _, s := a.authUser(r.Context(), tokenFromRequest(r)); s != nil {
		ch = contactChannel(s.Provider)
	}
	a.render(w, r, "account.html", map[string]any{"User": u, "Demos": demos, "Channel": ch})
}

func (a *App) accountContact(w http.ResponseWriter, r *http.Request) {
	if u, _ := a.authUser(r.Context(), tokenFromRequest(r)); u != nil {
		_ = a.store.SetContact(r.Context(), u.ID, r.FormValue("contact_ok") == "1", a.now())
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (a *App) accountExport(w http.ResponseWriter, r *http.Request) {
	u, _ := a.authUser(r.Context(), tokenFromRequest(r))
	if u == nil {
		http.Redirect(w, r, "/login?return=/account", http.StatusSeeOther)
		return
	}
	data, err := a.store.ExportUser(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "failed", 500)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="sedlab-account.json"`)
	writeJSON(w, 200, data)
}

func (a *App) accountDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := a.authUser(r.Context(), tokenFromRequest(r))
	if u == nil || r.FormValue("confirm") != "1" {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	lead, _ := a.store.LeadByUser(r.Context(), u.ID)
	if err := a.store.DeleteUser(r.Context(), u.ID); err != nil {
		http.Error(w, "failed", 500)
		return
	}
	a.setCookie(w, cookieAuth, "", -1, true)
	if lead != nil {
		a.notifyOwner(r.Context(), "Аккаунт удалён владельцем данных: лид "+lead.ID+" ("+lead.Provider+")", "del-"+lead.ID)
	}
	http.Redirect(w, r, "/account?deleted=1", http.StatusSeeOther)
}

// ---- public API for demos' widget (CORS, cookie) ----------------------------------------------------------

func (a *App) allowedOrigin(o string) bool {
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if a.cfg.Dev && (h == "localhost" || h == "127.0.0.1") {
		return true
	}
	return u.Scheme == "https" && strings.HasSuffix(h, a.cfg.ReturnSuffix)
}

func (a *App) cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && a.allowedOrigin(o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		h(w, r)
	}
}

func (a *App) apiMe(w http.ResponseWriter, r *http.Request) {
	u, s := a.authUser(r.Context(), tokenFromRequest(r))
	if u == nil {
		writeJSON(w, 200, map[string]any{"authenticated": false, "login_url": a.cfg.PublicURL + "/login"})
		return
	}
	writeJSON(w, 200, map[string]any{"authenticated": true, "name": u.DisplayName, "provider": s.Provider, "account_url": a.cfg.PublicURL + "/account"})
}

func (a *App) apiMeLimits(w http.ResponseWriter, r *http.Request) {
	u, _ := a.authUser(r.Context(), tokenFromRequest(r))
	demo := r.URL.Query().Get("demo")
	if u == nil {
		writeJSON(w, 200, map[string]any{"authenticated": false, "resources": []any{}})
		return
	}
	writeJSON(w, 200, map[string]any{"authenticated": true, "demo": demo, "resources": a.remaining(r, u.ID, demo)})
}

func (a *App) remaining(r *http.Request, userID, demo string) []map[string]any {
	var out []map[string]any
	for _, res := range a.limits.DemoResources(demo) {
		rc := a.limits.Resources[res]
		d, err := a.store.Check(r.Context(), userID, demo, res, 1, rc.Limit(), a.now())
		if err != nil {
			continue
		}
		label := rc.LabelRU
		if langOf(r) == "en" && rc.LabelEN != "" {
			label = rc.LabelEN
		}
		if label == "" {
			label = res
		}
		out = append(out, map[string]any{"resource": res, "label": label, "kind": rc.Kind, "unit": rc.Unit, "user_left": d.UserLeft, "user_limit": d.UserLimit,
			"resets_at": d.ResetsAt, "live_available": d.Allowed, "reason": d.Reason})
	}
	return out
}

// ---- dev emulator (AUTH_DEV=1 only) -----------------------------------------------------------------------

func (a *App) devLogin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	prov := q.Get("provider")
	if prov == "" {
		prov = "max"
	}
	id := q.Get("id")
	if id == "" {
		id = "1000001"
	}
	ret, demo := a.retCookie(r)
	if q.Get("return") != "" {
		ret = a.safeReturn(q.Get("return"))
	}
	a.completeBrowser(w, r, pending{Prov: prov, UID: id, Name: "Dev " + id, User: "dev" + id, Lang: langOf(r), Ret: ret, Demo: demo})
}
