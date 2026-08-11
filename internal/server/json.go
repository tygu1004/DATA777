package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"data777/internal/catalog"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
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

// parseFilterParam decodes the base64url-encoded `filter` query parameter (api.md#filter).
func parseFilterParam(r *http.Request) (catalog.Filter, error) {
	return catalog.DecodeFilterParam(r.URL.Query().Get("filter"))
}

// parseAtCommit decodes the optional `at_commit` query parameter (api.md#get-apisamples).
func parseAtCommit(r *http.Request) (*int64, error) {
	raw := r.URL.Query().Get("at_commit")
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func listOptions(offset int, cursor string, limit int, atCommit *int64) catalog.ListOptions {
	return catalog.ListOptions{Offset: offset, Cursor: cursor, Limit: limit, AtCommit: atCommit}
}

// parseWait decodes the optional `?wait=Ns` long-poll parameter jobs endpoints use (api.md#jobs).
func parseWait(r *http.Request) time.Duration {
	raw := r.URL.Query().Get("wait")
	if raw == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}
