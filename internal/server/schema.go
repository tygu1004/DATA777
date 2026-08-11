package server

import (
	"errors"
	"net/http"

	"data777/internal/catalog"
)

func (s *Server) handleGetSchema(w http.ResponseWriter, r *http.Request) {
	fields, err := s.cat.Schema(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

func (s *Server) handleDefineField(w http.ResponseWriter, r *http.Request) {
	var f catalog.FieldDef
	if err := readJSON(r, &f); err != nil || f.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid field definition")
		return
	}
	if err := s.cat.DefineField(r.Context(), f); err != nil {
		if errors.Is(err, catalog.ErrFieldConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, f)
}
