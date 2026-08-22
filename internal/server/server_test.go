package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"data777/internal/auth"
	"data777/internal/catalog"
	"data777/internal/indexer"
	"data777/internal/jobs"
	"data777/internal/plugins"
	"data777/internal/storage"
	"data777/internal/store"
	"data777/internal/thumbnail"
	"data777/internal/vectorindex"
)

func setupTestServer(t *testing.T) (*httptest.Server, *store.DB, *catalog.SQLiteCatalog) {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	src := storage.NewLocal()
	vector := vectorindex.NewBruteForce(db.DB)
	cat := catalog.NewSQLite(db.DB, vector)
	idx := indexer.New(db, src)
	thumbs, _ := thumbnail.New(filepath.Join(dbDir, "thumbs"), src)
	previews, _ := thumbnail.NewPreview(filepath.Join(dbDir, "previews"), src)
	jobsMgr := jobs.New(db.DB)
	tokens := auth.NewStore(db.DB)
	pluginsReg, _ := plugins.Load("nonexistent.yaml")

	srv := New(cat, vector, idx, thumbs, previews, jobsMgr, tokens, pluginsReg)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	return ts, db, cat
}

func seedServerSample(t *testing.T, db *store.DB, id int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO samples (id, path, filename, width, height, filesize, format, media_type, parent_id, group_id, t, slice, duration, fps)
		VALUES (?, ?, ?, 1920, 1080, 50000, 'jpg', 'image', 0, 0, 0, '', 0, 0)
	`, id, fmt.Sprintf("/path/img_%d.jpg", id), fmt.Sprintf("img_%d.jpg", id))
	if err != nil {
		t.Fatalf("seed sample: %v", err)
	}
}

func TestServer_Schema(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	res, err := http.Get(ts.URL + "/api/schema")
	if err != nil {
		t.Fatalf("GET /api/schema: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}

	var schema struct {
		Fields []catalog.FieldDef `json:"fields"`
	}
	if err := json.NewDecoder(res.Body).Decode(&schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if len(schema.Fields) == 0 {
		t.Errorf("schema fields is empty")
	}
}

func TestServer_CommitAndUndo(t *testing.T) {
	ts, db, _ := setupTestServer(t)

	seedServerSample(t, db, 1)
	seedServerSample(t, db, 2)

	// 1. POST /api/commits (Set tag "cat") -> Expect 202 Accepted + job_id
	reqBody, _ := json.Marshal(map[string]any{
		"message": "tag cat",
		"kind":    "set",
		"field":   "tags",
		"selection": map[string]any{
			"mode": "explicit",
			"ids":  []int64{1, 2},
		},
		"op":    "add",
		"value": "cat",
	})
	res, err := http.Post(ts.URL+"/api/commits", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /api/commits: %v", err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/commits status = %d, want 202", res.StatusCode)
	}

	var commitJob struct {
		JobID string `json:"job_id"`
	}
	json.NewDecoder(res.Body).Decode(&commitJob)
	res.Body.Close()

	if commitJob.JobID == "" {
		t.Fatalf("empty job_id returned")
	}

	// 2. Poll job with ?wait=2
	jobRes, err := http.Get(ts.URL + "/api/jobs/" + commitJob.JobID + "?wait=2")
	if err != nil {
		t.Fatalf("GET /api/jobs: %v", err)
	}
	var job jobs.Job
	json.NewDecoder(jobRes.Body).Decode(&job)
	jobRes.Body.Close()

	if job.Status != jobs.StatusSucceeded {
		t.Errorf("job status = %s, want succeeded", job.Status)
	}

	// 3. GET /api/tags -> should have "cat" count: 2
	tagRes, err := http.Get(ts.URL + "/api/tags")
	if err != nil {
		t.Fatalf("GET /api/tags: %v", err)
	}
	var tagsPayload struct {
		Items []catalog.TagCount `json:"items"`
	}
	json.NewDecoder(tagRes.Body).Decode(&tagsPayload)
	tagRes.Body.Close()

	if len(tagsPayload.Items) != 1 || tagsPayload.Items[0].Tag != "cat" || tagsPayload.Items[0].Count != 2 {
		t.Errorf("tags payload = %+v, want [{Tag: cat, Count: 2}]", tagsPayload)
	}

	// 4. POST /api/undo -> Expect 202 Accepted + job_id
	undoReq, _ := json.Marshal(map[string]any{})
	undoRes, err := http.Post(ts.URL+"/api/undo", "application/json", bytes.NewReader(undoReq))
	if err != nil {
		t.Fatalf("POST /api/undo: %v", err)
	}
	if undoRes.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/undo status = %d, want 202", undoRes.StatusCode)
	}

	var undoJob struct {
		JobID string `json:"job_id"`
	}
	json.NewDecoder(undoRes.Body).Decode(&undoJob)
	undoRes.Body.Close()

	// 5. Poll undo job
	pollUndoRes, err := http.Get(ts.URL + "/api/jobs/" + undoJob.JobID + "?wait=2")
	if err != nil {
		t.Fatalf("GET undo job: %v", err)
	}
	var uj jobs.Job
	json.NewDecoder(pollUndoRes.Body).Decode(&uj)
	pollUndoRes.Body.Close()

	if uj.Status != jobs.StatusSucceeded {
		t.Errorf("undo job status = %s, want succeeded", uj.Status)
	}

	// 6. GET /api/tags -> should now be empty
	tagRes2, _ := http.Get(ts.URL + "/api/tags")
	var tagsPayload2 struct {
		Items []catalog.TagCount `json:"items"`
	}
	json.NewDecoder(tagRes2.Body).Decode(&tagsPayload2)
	tagRes2.Body.Close()

	if len(tagsPayload2.Items) != 0 {
		t.Errorf("after undo, tags = %+v, want []", tagsPayload2.Items)
	}
}

func TestServer_Tokens(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	// Create token
	body, _ := json.Marshal(map[string]string{"name": "test-token"})
	res, err := http.Post(ts.URL+"/api/tokens", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/tokens: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Errorf("POST /api/tokens status = %d, want 201", res.StatusCode)
	}
	var created struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Secret string `json:"secret"`
	}
	json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()

	if created.Secret == "" || created.ID == "" {
		t.Errorf("token payload invalid = %+v", created)
	}

	// List tokens
	listRes, err := http.Get(ts.URL + "/api/tokens")
	if err != nil {
		t.Fatalf("GET /api/tokens: %v", err)
	}
	var tokensPayload struct {
		Items []auth.TokenMeta `json:"items"`
	}
	json.NewDecoder(listRes.Body).Decode(&tokensPayload)
	listRes.Body.Close()

	if len(tokensPayload.Items) != 1 || tokensPayload.Items[0].ID != created.ID {
		t.Errorf("tokens payload = %+v, want token ID %s", tokensPayload, created.ID)
	}
}
