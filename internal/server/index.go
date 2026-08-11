package server

import (
	"context"
	"net/http"

	"data777/internal/jobs"
)

type startIndexRequest struct {
	Path string `json:"path"`
}

// handleStartIndex folds the old bespoke GET /api/index/status into the general job model
// (api.md#post-apiindex) — indexing was always this shape (submit, poll, watch a counter),
// just without cancellation until now.
func (s *Server) handleStartIndex(w http.ResponseWriter, r *http.Request) {
	var req startIndexRequest
	if err := readJSON(r, &req); err != nil || req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	job, err := s.jobsMgr.Enqueue(r.Context(), "index", func(ctx context.Context, report jobs.ProgressFunc) (any, error) {
		return s.idx.Run(ctx, req.Path, report)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}
