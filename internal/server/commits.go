package server

import (
	"context"
	"net/http"

	"data777/internal/catalog"
	"data777/internal/jobs"
)

type createCommitRequest struct {
	Message   string             `json:"message"`
	Kind      string             `json:"kind"`
	Field     string             `json:"field"`
	Selection *catalog.Selection `json:"selection,omitempty"`
	Op        string             `json:"op,omitempty"`
	Value     string             `json:"value,omitempty"`
	Patches   []catalog.Patch    `json:"patches,omitempty"`
}

func commitResult(c catalog.Commit) map[string]any {
	return map[string]any{"commit_id": c.ID, "parent_id": c.ParentID, "affected_count": c.AffectedCount}
}

// handleCreateCommit always returns 202 + job_id (api.md#post-apicommits) — a set commit may
// need to stream a large filter into a bitmap before anything can be written, which isn't
// something to hold an HTTP connection open for.
func (s *Server) handleCreateCommit(w http.ResponseWriter, r *http.Request) {
	var req createCommitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var run jobs.RunFunc
	switch req.Kind {
	case "set":
		if req.Selection == nil {
			writeError(w, http.StatusBadRequest, "selection is required for a set commit")
			return
		}
		run = func(ctx context.Context, report jobs.ProgressFunc) (any, error) {
			commit, err := s.cat.ApplySet(ctx, *req.Selection, req.Field, catalog.Op(req.Op), req.Value, req.Message, catalog.ProgressFunc(report))
			if err != nil {
				return nil, err
			}
			return commitResult(commit), nil
		}
	case "patch":
		if len(req.Patches) == 0 {
			writeError(w, http.StatusBadRequest, "patches must be a non-empty array")
			return
		}
		run = func(ctx context.Context, report jobs.ProgressFunc) (any, error) {
			commit, err := s.cat.ApplyPatch(ctx, req.Field, req.Patches, req.Message)
			if err != nil {
				return nil, err
			}
			return commitResult(commit), nil
		}
	default:
		writeError(w, http.StatusBadRequest, `kind must be "set" or "patch"`)
		return
	}

	job, err := s.jobsMgr.Enqueue(r.Context(), "commit", run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	offset, limit := parsePagination(r, 50)
	commits, err := s.cat.ListCommits(r.Context(), offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": commits})
}

type undoRequest struct {
	// ExpectedHead, when set, guards against undoing a HEAD moved by someone else's commit
	// since the client last observed it (catalog.ErrHeadMoved) — optional so existing callers
	// that don't track HEAD keep working unchanged.
	ExpectedHead *int64 `json:"expected_head,omitempty"`
}

// handleUndo returns 409 immediately (no job) when there's no parent commit — that check is a
// cheap HEAD lookup, not something worth a job envelope (api.md#post-apiundo).
func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	var req undoRequest
	// body is optional; ignore a missing/empty one rather than treating it as an error
	readJSON(r, &req)

	head, err := s.cat.Head(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if head == nil {
		writeError(w, http.StatusConflict, catalog.ErrNoParentCommit.Error())
		return
	}
	if req.ExpectedHead != nil && *req.ExpectedHead != *head {
		writeError(w, http.StatusConflict, catalog.ErrHeadMoved.Error())
		return
	}

	job, err := s.jobsMgr.Enqueue(r.Context(), "undo", func(ctx context.Context, report jobs.ProgressFunc) (any, error) {
		newHead, err := s.cat.Undo(ctx, req.ExpectedHead)
		if err != nil {
			return nil, err
		}
		return map[string]any{"head_commit_id": newHead}, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}
