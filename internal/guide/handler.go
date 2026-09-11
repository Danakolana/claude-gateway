package guide

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:embed
var embedded embed.FS

// Handler serves the bilingual product guide at / and /guide,
// plus static files from embed/ (screenshots, etc.).
func Handler() http.Handler {
	sub, err := fs.Sub(embedded, "embed")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "guide unavailable", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(sub))
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
			p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
			if p == "." || p == "" || strings.HasPrefix(p, "..") {
				http.NotFound(w, r)
				return
			}
			ext := strings.ToLower(path.Ext(p))
			switch ext {
			case ".png", ".jpg", ".jpeg", ".webp", ".svg", ".gif":
				files.ServeHTTP(w, r)
			default:
				http.NotFound(w, r)
			}
		}
	})
}
