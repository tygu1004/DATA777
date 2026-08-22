package vectorindex

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"data777/internal/store"
	"github.com/RoaringBitmap/roaring/v2"
)

func setupTestVectorIndex(t *testing.T) (*BruteForce, *store.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	idx := NewBruteForce(db.DB)
	return idx, db
}

func seedSampleForVector(t *testing.T, db *store.DB, id int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO samples (id, path, filename, width, height, filesize, format, media_type, parent_id, group_id, t, slice, duration, fps)
		VALUES (?, ?, ?, 100, 100, 1000, 'jpg', 'image', 0, 0, 0, '', 0, 0)
	`, id, fmt.Sprintf("/path/img_%d.jpg", id), fmt.Sprintf("img_%d.jpg", id))
	if err != nil {
		t.Fatalf("seed sample: %v", err)
	}
}

func TestVectorIndex_BruteForce(t *testing.T) {
	ctx := context.Background()
	idx, db := setupTestVectorIndex(t)

	// Seed samples
	seedSampleForVector(t, db, 1)
	seedSampleForVector(t, db, 2)
	seedSampleForVector(t, db, 3)
	seedSampleForVector(t, db, 4)

	// Declare embedding field
	_, err := db.Exec(`INSERT INTO fields (name, kind, dims, metric) VALUES ('clip', 'embedding', 3, 'cosine')`)
	if err != nil {
		t.Fatalf("insert field: %v", err)
	}

	// Upsert vectors:
	// Sample 1: [1.0, 0.0, 0.0] -> Exact match to query [1.0, 0.0, 0.0]
	// Sample 2: [0.9, 0.1, 0.0] -> Very close
	// Sample 3: [0.0, 1.0, 0.0] -> Orthogonal (distance = 1.0)
	// Sample 4: [-1.0, 0.0, 0.0] -> Opposite (distance = 2.0)
	if err := idx.Upsert(ctx, "clip", 1, []float32{1.0, 0.0, 0.0}); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if err := idx.Upsert(ctx, "clip", 2, []float32{0.9, 0.1, 0.0}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	if err := idx.Upsert(ctx, "clip", 3, []float32{0.0, 1.0, 0.0}); err != nil {
		t.Fatalf("upsert 3: %v", err)
	}
	if err := idx.Upsert(ctx, "clip", 4, []float32{-1.0, 0.0, 0.0}); err != nil {
		t.Fatalf("upsert 4: %v", err)
	}

	// Test Get
	v1, err := idx.Get(ctx, "clip", 1)
	if err != nil || len(v1) != 3 || v1[0] != 1.0 {
		t.Errorf("Get(1) = %+v, err = %v", v1, err)
	}

	// Search top 3 nearest to [1.0, 0.0, 0.0]
	results, err := idx.Search(ctx, "clip", []float32{1.0, 0.0, 0.0}, "cosine", 3, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("results len = %d, want 3", len(results))
	}
	if results[0].ID != 1 {
		t.Errorf("1st nearest = %d (dist: %f), want sample 1", results[0].ID, results[0].Distance)
	}
	if results[1].ID != 2 {
		t.Errorf("2nd nearest = %d (dist: %f), want sample 2", results[1].ID, results[1].Distance)
	}
	if results[2].ID != 3 {
		t.Errorf("3rd nearest = %d (dist: %f), want sample 3", results[2].ID, results[2].Distance)
	}

	// Search with candidate filter (only allow sample 3 and 4)
	cand := roaring.New()
	cand.Add(3)
	cand.Add(4)
	filteredRes, err := idx.Search(ctx, "clip", []float32{1.0, 0.0, 0.0}, "cosine", 2, cand)
	if err != nil {
		t.Fatalf("Filtered Search: %v", err)
	}
	if len(filteredRes) != 2 || filteredRes[0].ID != 3 || filteredRes[1].ID != 4 {
		t.Errorf("Filtered search got %+v, want [sample 3, sample 4]", filteredRes)
	}
}
