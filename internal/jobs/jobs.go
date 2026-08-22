// Package jobs implements the async job model from docs/api.md#jobs: every mutation that
// isn't O(1) — a set/patch commit, undo, indexing, a plugin operator — runs through here
// instead of blocking the HTTP request that started it. Catalog methods stay ordinary
// synchronous Go calls (api.md#internal-interface); this package owns the async boundary.
package jobs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Progress struct {
	Processed int64  `json:"processed"`
	Total     *int64 `json:"total,omitempty"`
}

type Job struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Status     Status     `json:"status"`
	Progress   Progress   `json:"progress"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
	Result     any        `json:"result,omitempty"`
}

func (j Job) terminal() bool {
	switch j.Status {
	case StatusSucceeded, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

type ProgressFunc func(processed, total int64)
type RunFunc func(ctx context.Context, report ProgressFunc) (result any, err error)

// Manager runs jobs as goroutines and persists their bookkeeping to SQLite so history survives
// a restart — the goroutine itself does not, which is why MarkInterruptedOnStartup exists
// (api.md#jobs, "Left open": restart handling).
//
// ponytail: live jobs accumulate in an in-memory map for the life of the process rather than
// being evicted after some retention window — fine for a single long-running dev/team-scale
// process, a real memory-bounded cache is only worth adding if a deployment actually runs
// enough jobs in one process lifetime for it to matter.
type Manager struct {
	db *sql.DB

	mu     sync.Mutex
	live   map[string]*Job
	cancel map[string]context.CancelFunc
}

func New(db *sql.DB) *Manager {
	return &Manager{db: db, live: map[string]*Job{}, cancel: map[string]context.CancelFunc{}}
}

// MarkInterruptedOnStartup fails every job left queued/running from a previous process —
// its goroutine is gone, so it can never reach a real terminal state on its own.
func (m *Manager) MarkInterruptedOnStartup(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', error = 'interrupted by restart', finished_at = CURRENT_TIMESTAMP
		 WHERE status IN ('queued','running')`)
	if err != nil {
		return fmt.Errorf("mark interrupted jobs: %w", err)
	}
	return nil
}

func newID() string {
	buf := make([]byte, 6)
	rand.Read(buf)
	return "job_" + hex.EncodeToString(buf)
}

func (m *Manager) Enqueue(ctx context.Context, kind string, run RunFunc) (*Job, error) {
	job := &Job{ID: newID(), Kind: kind, Status: StatusQueued, CreatedAt: time.Now()}

	if _, err := m.db.ExecContext(ctx,
		`INSERT INTO jobs (id, kind, status, processed, created_at) VALUES (?, ?, ?, 0, ?)`,
		job.ID, job.Kind, job.Status, job.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	snapshot := *job
	m.mu.Lock()
	m.live[job.ID] = job
	m.cancel[job.ID] = cancel
	m.mu.Unlock()

	go m.run(runCtx, job.ID, run)

	return &snapshot, nil
}

func (m *Manager) run(ctx context.Context, id string, run RunFunc) {
	if ctx.Err() != nil {
		m.finish(id, StatusCanceled, "canceled before starting", nil)
		return
	}

	now := time.Now()
	m.mu.Lock()
	m.live[id].Status = StatusRunning
	m.live[id].StartedAt = &now
	m.mu.Unlock()
	m.persistStatus(id, StatusRunning, &now, nil)

	result, err := run(ctx, func(processed, total int64) { m.updateProgress(id, processed, total) })

	if ctx.Err() != nil {
		m.finish(id, StatusCanceled, "canceled", nil)
		return
	}
	if err != nil {
		m.finish(id, StatusFailed, err.Error(), nil)
		return
	}
	m.finish(id, StatusSucceeded, "", result)
}

// updateProgress treats total<=0 as "not known in advance" (api.md#jobs) and leaves the
// column/field unset rather than storing a misleading 0.
func (m *Manager) updateProgress(id string, processed, total int64) {
	m.mu.Lock()
	job, ok := m.live[id]
	if ok {
		job.Progress = Progress{Processed: processed}
		if total > 0 {
			t := total
			job.Progress.Total = &t
		}
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	if total > 0 {
		m.db.Exec(`UPDATE jobs SET processed = ?, total = ? WHERE id = ?`, processed, total, id)
	} else {
		m.db.Exec(`UPDATE jobs SET processed = ? WHERE id = ?`, processed, id)
	}
}

func (m *Manager) persistStatus(id string, status Status, startedAt *time.Time, finishedAt *time.Time) {
	m.db.Exec(`UPDATE jobs SET status = ?, started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at) WHERE id = ?`,
		status, startedAt, finishedAt, id)
}

func (m *Manager) finish(id string, status Status, errMsg string, result any) {
	now := time.Now()

	m.mu.Lock()
	job, ok := m.live[id]
	if ok {
		job.Status = status
		job.FinishedAt = &now
		job.Error = errMsg
		job.Result = result
	}
	delete(m.cancel, id)
	m.mu.Unlock()

	var resultJSON []byte
	if result != nil {
		resultJSON, _ = json.Marshal(result)
	}
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	}
	var resultArg any
	if resultJSON != nil {
		resultArg = string(resultJSON)
	}
	if !ok {
		return
	}
	m.db.Exec(`UPDATE jobs SET status = ?, finished_at = ?, error = ?, result = ? WHERE id = ?`,
		status, now, errArg, resultArg, id)
}

// Get returns a job's current state, long-polling up to wait for a terminal status
// (api.md#jobs: "?wait=Ns long-polls up to N seconds for a terminal state").
func (m *Manager) Get(ctx context.Context, id string, wait time.Duration) (*Job, error) {
	deadline := time.Now().Add(wait)
	for {
		job, err := m.snapshot(id)
		if err != nil {
			return nil, err
		}
		if job.terminal() || time.Now().After(deadline) {
			return job, nil
		}
		select {
		case <-ctx.Done():
			return job, nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Manager) snapshot(id string) (*Job, error) {
	m.mu.Lock()
	if job, ok := m.live[id]; ok {
		cp := *job
		m.mu.Unlock()
		return &cp, nil
	}
	m.mu.Unlock()
	return m.fromDB(id)
}

func (m *Manager) fromDB(id string) (*Job, error) {
	var job Job
	var started, finished sql.NullTime
	var errMsg, result sql.NullString
	var total sql.NullInt64
	err := m.db.QueryRow(
		`SELECT id, kind, status, processed, total, error, result, created_at, started_at, finished_at FROM jobs WHERE id = ?`, id,
	).Scan(&job.ID, &job.Kind, &job.Status, &job.Progress.Processed, &total, &errMsg, &result, &job.CreatedAt, &started, &finished)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("read job: %w", err)
	}
	if total.Valid {
		job.Progress.Total = &total.Int64
	}
	if errMsg.Valid {
		job.Error = errMsg.String
	}
	if result.Valid {
		job.Result = json.RawMessage(result.String)
	}
	if started.Valid {
		job.StartedAt = &started.Time
	}
	if finished.Valid {
		job.FinishedAt = &finished.Time
	}
	return &job, nil
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	m.mu.Lock()
	cancel, ok := m.cancel[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %q is not queued or running", id)
	}
	cancel()
	return nil
}

func (m *Manager) List(ctx context.Context, status string) ([]Job, error) {
	query := `SELECT id, kind, status, processed, total, error, result, created_at, started_at, finished_at FROM jobs`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	out := []Job{}
	for rows.Next() {
		var job Job
		var started, finished sql.NullTime
		var errMsg, result sql.NullString
		var total sql.NullInt64
		if err := rows.Scan(&job.ID, &job.Kind, &job.Status, &job.Progress.Processed, &total, &errMsg, &result, &job.CreatedAt, &started, &finished); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if total.Valid {
			job.Progress.Total = &total.Int64
		}
		if errMsg.Valid {
			job.Error = errMsg.String
		}
		if result.Valid {
			job.Result = json.RawMessage(result.String)
		}
		if started.Valid {
			job.StartedAt = &started.Time
		}
		if finished.Valid {
			job.FinishedAt = &finished.Time
		}
		out = append(out, job)
	}
	return out, rows.Err()
}
