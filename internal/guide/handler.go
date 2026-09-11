package guide

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed embed/*
var embedded embed.FS

// Handler serves the bilingual product guide at / and /guide.
func Handler() http.Handler {
	sub, err := fs.Sub(embedded, "embed")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "guide unavailable", http.StatusInternalServerError)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/guide", "/guide/", "/index.html":
			data, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "guide missing", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return
		default:
			http.NotFound(w, r)
		}
	})
}
