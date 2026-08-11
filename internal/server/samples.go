package server

import (
	"net/http"
	"strconv"

	"data777/internal/thumbnail"
)

func (s *Server) handleListSamples(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilterParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	atCommit, err := parseAtCommit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid at_commit")
		return
	}
	offset, limit := parsePagination(r, 200)
	cursor := r.URL.Query().Get("cursor")

	result, err := s.cat.ListSamples(r.Context(), f, listOptions(offset, cursor, limit, atCommit))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]any{"items": result.Items}
	if result.NextCursor != "" {
		resp["next_cursor"] = result.NextCursor
	}
	if result.Seed != nil {
		resp["seed"] = *result.Seed
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCountSamples(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilterParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	atCommit, err := parseAtCommit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid at_commit")
		return
	}
	count, err := s.cat.CountSamples(r.Context(), f, atCommit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

func (s *Server) handleTagCounts(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilterParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	atCommit, err := parseAtCommit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid at_commit")
		return
	}
	counts, err := s.cat.TagCounts(r.Context(), f, atCommit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": counts})
}

func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	s.serveGenerated(w, r, s.thumbs)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	s.serveGenerated(w, r, s.previews)
}

func (s *Server) serveGenerated(w http.ResponseWriter, r *http.Request, gen *thumbnail.Generator) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	srcPath, err := s.cat.GetSamplePath(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "sample not found")
		return
	}

	path, err := gen.GetOrGenerate(r.Context(), id, srcPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}
