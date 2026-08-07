package server

import "net/http"

type startIndexRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleStartIndex(w http.ResponseWriter, r *http.Request) {
	var req startIndexRequest
	if err := readJSON(r, &req); err != nil || req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := s.idx.StartIndex(req.Path); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleIndexStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.idx.Status())
}
