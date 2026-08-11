package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"data777/internal/jobs"
)

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.pluginsReg.Manifests())
}

func (s *Server) handleReloadPlugins(w http.ResponseWriter, r *http.Request) {
	s.pluginsReg.Reload(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// handleRunOperator always returns 202 + job_id (plugins.md#post-apipluginspluginoperatorsoperator)
// — the UI never needs to know in advance whether an operator is a sub-second auto-tagger or
// an hours-long batch job. The server attaches a scoped bearer token to the outbound call so
// the operator can call back into data777's own API using the same SDK a script would.
func (s *Server) handleRunOperator(w http.ResponseWriter, r *http.Request) {
	plugin := r.PathValue("plugin")
	operator := r.PathValue("operator")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	expires := time.Now().Add(10 * time.Minute)
	_, secret, err := s.tokens.Create(r.Context(), fmt.Sprintf("operator:%s/%s", plugin, operator), &expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	job, err := s.jobsMgr.Enqueue(r.Context(), "operator", func(ctx context.Context, report jobs.ProgressFunc) (any, error) {
		report(0, 0)
		raw, err := s.pluginsReg.RunOperator(ctx, plugin, operator, body, secret)
		if err != nil {
			return nil, err
		}
		report(1, 1)
		var result any
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("decode operator result: %w", err)
		}
		return result, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// handlePanelProxy reverse-proxies a plugin's panel UI so the iframe src the dashboard uses is
// always same-origin (plugins.md#data777-side-endpoints).
func (s *Server) handlePanelProxy(w http.ResponseWriter, r *http.Request) {
	plugin := r.PathValue("plugin")
	panel := r.PathValue("panel")

	proxy, err := s.pluginsReg.PanelProxy(plugin)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	prefix := fmt.Sprintf("/api/plugins/%s/panels/%s", plugin, panel)
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/panels/" + panel + "/" + strings.TrimPrefix(r.URL.Path, prefix+"/")
	proxy.ServeHTTP(w, r2)
}
