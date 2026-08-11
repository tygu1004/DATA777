package server

import (
	"log"
	"net/http"
	"time"

	"data777/internal/auth"
	"data777/internal/catalog"
	"data777/internal/indexer"
	"data777/internal/jobs"
	"data777/internal/plugins"
	"data777/internal/thumbnail"
	"data777/internal/vectorindex"
)

type Server struct {
	cat        *catalog.SQLiteCatalog
	vector     vectorindex.Index
	idx        *indexer.Indexer
	thumbs     *thumbnail.Generator
	previews   *thumbnail.Generator
	jobsMgr    *jobs.Manager
	tokens     *auth.Store
	pluginsReg *plugins.Registry
}

func New(cat *catalog.SQLiteCatalog, vector vectorindex.Index, idx *indexer.Indexer, thumbs, previews *thumbnail.Generator, jobsMgr *jobs.Manager, tokens *auth.Store, pluginsReg *plugins.Registry) *Server {
	return &Server{cat: cat, vector: vector, idx: idx, thumbs: thumbs, previews: previews, jobsMgr: jobsMgr, tokens: tokens, pluginsReg: pluginsReg}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/schema", s.handleGetSchema)
	mux.HandleFunc("POST /api/schema/fields", s.handleDefineField)

	mux.HandleFunc("POST /api/index", s.handleStartIndex)
	mux.HandleFunc("GET /api/samples", s.handleListSamples)
	mux.HandleFunc("GET /api/samples/count", s.handleCountSamples)
	mux.HandleFunc("GET /api/tags", s.handleTagCounts)
	mux.HandleFunc("GET /api/thumbnails/{id}", s.handleThumbnail)
	mux.HandleFunc("GET /api/previews/{id}", s.handlePreview)

	mux.HandleFunc("POST /api/commits", s.handleCreateCommit)
	mux.HandleFunc("GET /api/commits", s.handleListCommits)
	mux.HandleFunc("POST /api/undo", s.handleUndo)

	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleCancelJob)

	mux.HandleFunc("POST /api/tokens", s.handleCreateToken)
	mux.HandleFunc("GET /api/tokens", s.handleListTokens)
	mux.HandleFunc("DELETE /api/tokens/{id}", s.handleRevokeToken)

	mux.HandleFunc("POST /api/embeddings/{field}", s.handleUpsertEmbeddings)
	mux.HandleFunc("GET /api/embeddings/{field}/{sample_id}", s.handleGetEmbedding)

	mux.HandleFunc("GET /api/plugins", s.handleListPlugins)
	mux.HandleFunc("POST /api/plugins/reload", s.handleReloadPlugins)
	mux.HandleFunc("POST /api/plugins/{plugin}/operators/{operator}", s.handleRunOperator)
	mux.HandleFunc("GET /api/plugins/{plugin}/panels/{panel}/{rest...}", s.handlePanelProxy)

	mux.Handle("/", staticHandler())

	return withLogging(auth.Middleware(s.tokens, mux))
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
