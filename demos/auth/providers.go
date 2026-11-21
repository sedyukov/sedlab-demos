package main

// Pluggable login providers.
//  - Which providers are shown, and in what order, per UI locale — config only:
//      AUTH_LOGIN_PROVIDERS_RU="max,yandex,vk"   AUTH_LOGIN_PROVIDERS_EN="max,yandex,vk"
//    A provider absent from every list is disabled server-side too (its endpoints and webhooks refuse logins).
//    An OAuth provider without client_id/secret in env is skipped even if listed.
//  - Max is built in: bot deep link (browser) / Mini App initData.
//  - OAuth providers (Yandex ID, VK ID; later e.g. Sber ID) are one adapter each implementing OAuthProvider, registered
//    in oauthAdapters; routes /login/oauth/{id}/start|callback are generic. Telegram and Google are NOT used for login
//    (149-ФЗ / owner's decision 2026-11-21).

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	akit "github.com/sedyukov/sedlab-modules/modules/accounts/go/kit"
	"github.com/sedyukov/sedlab-modules/modules/accounts/go/oauth"
)

// oauthAdapters: OAuth providers from the accounts module (accounts/go/oauth). Env prefix ACCOUNTS_:
// YANDEX_CLIENT_ID/_SECRET, VK_CLIENT_ID/_SECRET, endpoint overrides, OAUTH_MOCK=1 (dev/tests).
// A provider without client_id is skipped. Instances are built once (the mock is stateful).
var oauthAdapters = func(cfg *Config) []oauth.Provider {
	var out []oauth.Provider
	for _, id := range []string{oauth.Yandex, oauth.VK} {
		p, err := oauth.FromEnv(akit.Env{Prefix: "ACCOUNTS_"}, id)
		if err != nil {
			continue
		}
		if p.ID() == oauth.Mock {
			p = &renamed{Provider: p, id: id} // dev: the mock answers as yandex/vk
		}
		out = append(out, p)
	}
	return out
}

type renamed struct {
	oauth.Provider
	id string
}

func (r *renamed) ID() string { return r.id }

type LoginMethod struct {
	ID    string
	Kind  string // messenger | oauth
	Label map[string]string
}

var methodLabels = map[string]map[string]string{
	"max":    {"ru": "Войти через Max", "en": "Sign in with Max"},
	"yandex": {"ru": "Войти с Яндекс ID", "en": "Sign in with Yandex ID"},
	"vk":     {"ru": "Войти через VK ID", "en": "Sign in with VK ID"},
}

func parseProviderList(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
	}
	return out
}

type Providers struct {
	byLang map[string][]string
	oauth  map[string]oauth.Provider
}

func NewProviders(cfg *Config) *Providers {
	p := &Providers{byLang: map[string][]string{
		"ru": parseProviderList(env("AUTH_LOGIN_PROVIDERS_RU", "max,yandex,vk")),
		"en": parseProviderList(env("AUTH_LOGIN_PROVIDERS_EN", "max,yandex,vk")),
	}, oauth: map[string]oauth.Provider{}}
	for _, o := range oauthAdapters(cfg) {
		p.oauth[o.ID()] = o
	}
	return p
}

func (p *Providers) usable(id string) bool {
	return id == "max" || p.oauth[id] != nil
}

// Enabled: shown in at least one locale (and implemented).
func (p *Providers) Enabled(id string) bool {
	if !p.usable(id) {
		return false
	}
	for _, l := range p.byLang {
		for _, x := range l {
			if x == id {
				return true
			}
		}
	}
	return false
}

// For: methods for the login page in this locale, in configured order.
func (p *Providers) For(lang string) []LoginMethod {
	var out []LoginMethod
	for _, id := range p.byLang[lang] {
		if !p.usable(id) {
			continue
		}
		kind := "messenger"
		if p.oauth[id] != nil {
			kind = "oauth"
		}
		lbl := methodLabels[id]
		if lbl == nil {
			lbl = map[string]string{"ru": id, "en": id}
		}
		out = append(out, LoginMethod{ID: id, Kind: kind, Label: lbl})
	}
	return out
}

// ---- generic OAuth routes ---------------------------------------------------------------------------------

func oauthRedirect(a *App, id string) string {
	return a.cfg.PublicURL + "/login/oauth/" + id + "/callback"
}

func (a *App) oauthStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o := a.providers.oauth[id]
	if o == nil || !a.providers.Enabled(id) {
		http.NotFound(w, r)
		return
	}
	pend, err := oauth.Begin(o, oauthRedirect(a, id))
	if err != nil {
		slog.Warn("oauth begin", "provider", id, "err", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape(id), http.StatusSeeOther)
		return
	}
	a.setCookie(w, "sl_oauth_"+id, pend.State+"."+pend.Verifier, nonceTTL, false)
	http.Redirect(w, r, pend.URL, http.StatusFound)
}

func (a *App) oauthCallback(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o := a.providers.oauth[id]
	var state, verifier string
	if c, err := r.Cookie("sl_oauth_" + id); err == nil {
		state, verifier, _ = strings.Cut(c.Value, ".")
	}
	a.setCookie(w, "sl_oauth_"+id, "", -1, false)
	if o == nil || !a.providers.Enabled(id) {
		http.NotFound(w, r)
		return
	}
	ident, err := oauth.Complete(r.Context(), o, r.URL.Query(), state, verifier, oauthRedirect(a, id))
	if err != nil {
		var ke *akit.Error
		if errors.As(err, &ke) && ke.Code == akit.PermissionDenied {
			http.Redirect(w, r, "/login?error=cancelled", http.StatusSeeOther)
			return
		}
		slog.Info("oauth failed", "provider", id, "err", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape(id), http.StatusSeeOther)
		return
	}
	ret, demo := a.retCookie(r)
	a.completeBrowser(w, r, pending{Prov: id, UID: ident.ProviderUID, Name: ident.DisplayName, User: ident.Username, Lang: langOf(r), Ret: ret, Demo: demo})
}
