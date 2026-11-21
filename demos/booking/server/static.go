package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The web build (vite → server/static) is embedded: one binary, one container.
//
//go:embed all:static
var staticFS embed.FS

func staticHandler() http.Handler {
	sub, _ := fs.Sub(staticFS, "static")
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(p, "/pay/") && !strings.Contains(p[5:], "."):
			r = r.Clone(r.Context())
			r.URL.Path = "/pay/" // fake checkout page: /pay/<payment id>
			w.Header().Set("Cache-Control", "no-cache")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
