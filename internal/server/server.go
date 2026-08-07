package server

import (
	"log"
	"net/http"
	"time"

	"data777/internal/indexer"
	"data777/internal/store"
	"data777/internal/thumbnail"
)

type Server struct {
	db     *store.DB
	idx    *indexer.Indexer
	thumbs *thumbnail.Generator
}

func New(db *store.DB, idx *indexer.Indexer, thumbs *thumbnail.Generator) *Server {
	return &Server{db: db, idx: idx, thumbs: thumbs}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/index", s.handleStartIndex)
	mux.HandleFunc("GET /api/index/status", s.handleIndexStatus)
	mux.HandleFunc("GET /api/samples", s.handleListSamples)
	mux.HandleFunc("GET /api/thumbnails/{id}", s.handleThumbnail)
	mux.HandleFunc("POST /api/commits", s.handleCreateCommit)
	mux.HandleFunc("GET /api/commits", s.handleListCommits)
	mux.HandleFunc("POST /api/undo", s.handleUndo)

	mux.Handle("/", staticHandler())

	return withLogging(mux)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
