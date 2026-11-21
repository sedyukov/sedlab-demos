package main

// Legal pages: privacy policy, Demo Terms, separate consent pages (152-ФЗ ч.1 ст.9 в ред. 156-ФЗ).
// HTML in web/legal is generated from freelance-ops legal/ by accounts/tools/legal_sync.py — with {{PLACEHOLDERS}}
// instead of the operator's requisites. Values come from env at deploy time: {{X}} ← AUTH_X (HTML-escaped).
// An unset placeholder stays visible, so a missing value is noticed instead of silently blank.

import (
	"html"
	"html/template"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]*)\}\}`)

func fillPlaceholders(s string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-2]
		if v := strings.TrimSpace(os.Getenv("AUTH_" + name)); v != "" {
			return html.EscapeString(v)
		}
		return m
	})
}

func (a *App) legalPage(doc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := langOf(r)
		b, err := webFS.ReadFile("web/legal/" + doc + "." + lang + ".html")
		if err != nil {
			b, err = webFS.ReadFile("web/legal/" + doc + ".ru.html")
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		a.render(w, r, "legal.html", map[string]any{"Body": template.HTML(fillPlaceholders(string(b)))})
	}
}

// providerLabel: how the sign-in service is named in texts (ui-texts §1).
func providerLabel(prov, lang string) string {
	switch prov {
	case "max":
		return "Max"
	case "yandex":
		if lang == "en" {
			return "Yandex ID"
		}
		return "Яндекс ID"
	case "vk":
		return "VK ID"
	}
	return prov
}

// contactChannel: where the owner could message the person (consent §3: only Max or VK allow it).
func contactChannel(prov string) string {
	switch prov {
	case "max":
		return "Max"
	case "vk":
		return "VK"
	}
	return ""
}
