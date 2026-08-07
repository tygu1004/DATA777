package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:webdist
var webdistFS embed.FS

// staticHandler serves the embedded Vite build, falling back to index.html for any path that
// isn't a real file so client-side routing (if added later) keeps working on refresh.
func staticHandler() http.Handler {
	sub, err := fs.Sub(webdistFS, "webdist")
	if err != nil {
		panic(err) // webdist is embedded at build time; this can only fail if the build is broken
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
