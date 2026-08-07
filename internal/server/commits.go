package server

import (
	"errors"
	"net/http"

	"data777/internal/store"
)

type createCommitRequest struct {
	Message string        `json:"message"`
	Ops     []store.TagOp `json:"ops"`
}

func (s *Server) handleCreateCommit(w http.ResponseWriter, r *http.Request) {
	var req createCommitRequest
	if err := readJSON(r, &req); err != nil || len(req.Ops) == 0 {
		writeError(w, http.StatusBadRequest, "ops must be a non-empty array")
		return
	}

	commit, err := s.db.CreateCommit(req.Message, req.Ops)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, commit)
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	offset, limit := parsePagination(r, 50)

	commits, err := s.db.ListCommits(offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": commits})
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	newHead, err := s.db.Undo()
	if err != nil {
		if errors.Is(err, store.ErrNoParentCommit) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"head_commit_id": newHead})
}
