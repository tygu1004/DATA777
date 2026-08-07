package server

import (
	"net/http"
	"strconv"

	"data777/internal/store"
)

type listSamplesResponse struct {
	Total int            `json:"total"`
	Items []store.Sample `json:"items"`
}

func (s *Server) handleListSamples(w http.ResponseWriter, r *http.Request) {
	offset, limit := parsePagination(r, 200)

	samples, err := s.db.ListSamples(offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := s.db.CountSamples()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, listSamplesResponse{Total: total, Items: samples})
}

func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	srcPath, err := s.db.GetSamplePath(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "sample not found")
		return
	}

	thumbPath, err := s.thumbs.GetOrGenerate(id, srcPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, thumbPath)
}

func parsePagination(r *http.Request, defaultLimit int) (offset, limit int) {
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	return offset, limit
}
