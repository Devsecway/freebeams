package remote

import (
	"embed"
	"io/fs"
	"net/http"
)

// web is the phone app: plain HTML, CSS and JavaScript, no external
// resources (church Wi-Fi is often offline).
//
//go:embed web
var web embed.FS

// staticHandler serves the phone app. Pages are revalidated on every load so
// an updated FreeBeams never serves a stale script.
func staticHandler() http.Handler {
	sub, err := fs.Sub(web, "web")
	if err != nil {
		panic(err) // the embed pattern guarantees the directory
	}
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-cache")
		h.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws://"+r.Host+" wss://"+r.Host+
			"; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		files.ServeHTTP(w, r)
	})
}
