package server

import "net/http"

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.jobsMgr.Get(r.Context(), r.PathValue("id"), parseWait(r))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if err := s.jobsMgr.Cancel(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	list, err := s.jobsMgr.List(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}
