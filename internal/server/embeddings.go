package server

import (
	"net/http"
	"strconv"
)

type upsertEmbeddingsRequest struct {
	Items []struct {
		SampleID int64     `json:"sample_id"`
		Vector   []float32 `json:"vector"`
	} `json:"items"`
}

// handleUpsertEmbeddings is not a commit — an embedding is a model's output, not a curation
// decision, so there's nothing here to undo (api.md#fields).
func (s *Server) handleUpsertEmbeddings(w http.ResponseWriter, r *http.Request) {
	field := r.PathValue("field")
	var req upsertEmbeddingsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, item := range req.Items {
		if err := s.vector.Upsert(r.Context(), field, item.SampleID, item.Vector); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetEmbedding(w http.ResponseWriter, r *http.Request) {
	field := r.PathValue("field")
	id, err := strconv.ParseInt(r.PathValue("sample_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}
	vec, err := s.vector.Get(r.Context(), field, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sample_id": id, "vector": vec})
}
