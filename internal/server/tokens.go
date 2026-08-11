package server

import (
	"net/http"
	"time"
)

type createTokenRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	meta, secret, err := s.tokens.Create(r.Context(), req.Name, req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": meta.ID, "name": meta.Name, "secret": secret,
		"created_at": meta.CreatedAt, "expires_at": meta.ExpiresAt,
	})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.tokens.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if err := s.tokens.Revoke(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
