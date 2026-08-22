package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"data777/internal/store"
)

func setupTestJobsManager(t *testing.T) (*Manager, *store.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mgr := New(db.DB)
	return mgr, db
}

func TestJobs_Lifecycle(t *testing.T) {
	ctx := context.Background()
	mgr, _ := setupTestJobsManager(t)

	var runs int32
	job, err := mgr.Enqueue(ctx, "test_job", func(jobCtx context.Context, report ProgressFunc) (any, error) {
		atomic.AddInt32(&runs, 1)
		report(50, 100)
		time.Sleep(20 * time.Millisecond)
		report(100, 100)
		return map[string]string{"message": "done"}, nil
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Long poll with wait = 1 second
	polled, err := mgr.Get(ctx, job.ID, 1*time.Second)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if polled.Status != StatusSucceeded {
		t.Errorf("job status = %s, want succeeded", polled.Status)
	}
	if polled.Progress.Processed != 100 {
		t.Errorf("job progress = %d, want 100", polled.Progress.Processed)
	}
	if atomic.LoadInt32(&runs) != 1 {
		t.Errorf("run count = %d, want 1", runs)
	}
}

func TestJobs_Cancel(t *testing.T) {
	ctx := context.Background()
	mgr, _ := setupTestJobsManager(t)

	started := make(chan struct{})
	job, err := mgr.Enqueue(ctx, "long_job", func(jobCtx context.Context, report ProgressFunc) (any, error) {
		close(started)
		select {
		case <-jobCtx.Done():
			return nil, jobCtx.Err()
		case <-time.After(2 * time.Second):
			return "finished", nil
		}
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	<-started // Wait for job to start running

	// Cancel job
	if err := mgr.Cancel(ctx, job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// Wait for canceled state
	polled, err := mgr.Get(ctx, job.ID, 1*time.Second)
	if err != nil {
		t.Fatalf("Get after cancel: %v", err)
	}
	if polled.Status != StatusCanceled {
		t.Errorf("job status = %s, want canceled", polled.Status)
	}
}

func TestJobs_Failure(t *testing.T) {
	ctx := context.Background()
	mgr, _ := setupTestJobsManager(t)

	expectedErr := errors.New("something went wrong")
	job, err := mgr.Enqueue(ctx, "fail_job", func(jobCtx context.Context, report ProgressFunc) (any, error) {
		return nil, expectedErr
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	polled, err := mgr.Get(ctx, job.ID, 1*time.Second)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if polled.Status != StatusFailed {
		t.Errorf("job status = %s, want failed", polled.Status)
	}
	if polled.Error != expectedErr.Error() {
		t.Errorf("job error = %q, want %q", polled.Error, expectedErr.Error())
	}
}

func TestJobs_MarkInterruptedOnStartup(t *testing.T) {
	ctx := context.Background()
	mgr, db := setupTestJobsManager(t)

	// Simulate jobs that were left in running and queued status before process crash
	_, err := db.Exec(`
		INSERT INTO jobs (id, kind, status, processed, created_at)
		VALUES ('job_running_1', 'index', 'running', 10, CURRENT_TIMESTAMP),
		       ('job_queued_2', 'commit', 'queued', 0, CURRENT_TIMESTAMP),
		       ('job_succeeded_3', 'commit', 'succeeded', 100, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("seed jobs: %v", err)
	}

	if err := mgr.MarkInterruptedOnStartup(ctx); err != nil {
		t.Fatalf("MarkInterruptedOnStartup: %v", err)
	}

	j1, _ := mgr.Get(ctx, "job_running_1", 0)
	if j1.Status != StatusFailed || j1.Error != "interrupted by restart" {
		t.Errorf("j1 = %+v, want status=failed, error='interrupted by restart'", j1)
	}

	j2, _ := mgr.Get(ctx, "job_queued_2", 0)
	if j2.Status != StatusFailed || j2.Error != "interrupted by restart" {
		t.Errorf("j2 = %+v, want status=failed, error='interrupted by restart'", j2)
	}

	j3, _ := mgr.Get(ctx, "job_succeeded_3", 0)
	if j3.Status != StatusSucceeded {
		t.Errorf("j3 = %+v, want status=succeeded (already finished jobs should not be touched)", j3)
	}
}
