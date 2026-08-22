package catalog

import (
	"context"
	"path/filepath"
	"testing"

	"data777/internal/store"
	"data777/internal/vectorindex"
)

func setupTestCatalog(t *testing.T) (*SQLiteCatalog, *store.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	vector := vectorindex.NewBruteForce(db.DB)
	cat := NewSQLite(db.DB, vector)
	return cat, db
}

func seedSample(t *testing.T, db *store.DB, id int64, filename string, width, height int, filesize int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO samples (id, path, filename, width, height, filesize, format, media_type, parent_id, group_id, t, slice, duration, fps)
		VALUES (?, ?, ?, ?, ?, ?, 'jpg', 'image', 0, 0, 0, '', 0, 0)
	`, id, "/path/"+filename, filename, width, height, filesize)
	if err != nil {
		t.Fatalf("seed sample %d: %v", id, err)
	}
}

// TestApplySet_DeltaCorrectness validates the critical Roaring Bitmap delta computation:
// 1. Adding a tag to untagged samples reports exact affected count.
// 2. Adding a tag when some samples ALREADY have it must only count/delta the truly new samples (2026-08-11 fix).
// 3. Removing a tag only counts samples that actually had that tag.
// 4. Removing a non-existent tag reports 0 affected and produces no erroneous mutations.
func TestApplySet_DeltaCorrectness(t *testing.T) {
	ctx := context.Background()
	cat, db := setupTestCatalog(t)

	// Seed 4 samples
	seedSample(t, db, 1, "img1.jpg", 1920, 1080, 50000)
	seedSample(t, db, 2, "img2.jpg", 1920, 1080, 60000)
	seedSample(t, db, 3, "img3.jpg", 1280, 720, 40000)
	seedSample(t, db, 4, "img4.jpg", 800, 600, 30000)

	// 1. Add "cat" to samples 1, 2, 3 -> Affected should be 3
	commit1, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1, 2, 3}}, "tags", OpAdd, "cat", "add cat to 1,2,3", nil)
	if err != nil {
		t.Fatalf("ApplySet commit1: %v", err)
	}
	if commit1.AffectedCount != 3 {
		t.Errorf("commit1 AffectedCount = %d, want 3", commit1.AffectedCount)
	}

	// 2. Add "cat" to samples 2, 3, 4 (2 and 3 already have "cat") -> Affected should be ONLY 1 (sample 4)
	commit2, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{2, 3, 4}}, "tags", OpAdd, "cat", "add cat to 2,3,4", nil)
	if err != nil {
		t.Fatalf("ApplySet commit2: %v", err)
	}
	if commit2.AffectedCount != 1 {
		t.Errorf("commit2 AffectedCount = %d, want 1 (only sample 4 was newly tagged)", commit2.AffectedCount)
	}

	// Verify all 4 samples have "cat"
	tags, err := cat.TagCounts(ctx, Filter{}, nil)
	if err != nil {
		t.Fatalf("TagCounts: %v", err)
	}
	if len(tags) != 1 || tags[0].Tag != "cat" || tags[0].Count != 4 {
		t.Errorf("tag counts = %+v, want [{Tag: cat, Count: 4}]", tags)
	}

	// 3. Remove "cat" from samples 1, 2 -> Affected should be 2
	commit3, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1, 2}}, "tags", OpRemove, "cat", "remove cat from 1,2", nil)
	if err != nil {
		t.Fatalf("ApplySet commit3: %v", err)
	}
	if commit3.AffectedCount != 2 {
		t.Errorf("commit3 AffectedCount = %d, want 2", commit3.AffectedCount)
	}

	// 4. Try to remove "dog" (which no sample has) -> Affected should be 0
	commit4, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1, 2, 3, 4}}, "tags", OpRemove, "dog", "remove dog from 1,2,3,4", nil)
	if err != nil {
		t.Fatalf("ApplySet commit4: %v", err)
	}
	if commit4.AffectedCount != 0 {
		t.Errorf("commit4 AffectedCount = %d, want 0", commit4.AffectedCount)
	}
}

// TestUndo_RestoresOriginalTags verifies that sequential Undos accurately revert
// the Roaring Bitmaps to their exact prior states.
func TestUndo_RestoresOriginalTags(t *testing.T) {
	ctx := context.Background()
	cat, db := setupTestCatalog(t)

	seedSample(t, db, 1, "img1.jpg", 1920, 1080, 50000)
	seedSample(t, db, 2, "img2.jpg", 1920, 1080, 60000)
	seedSample(t, db, 3, "img3.jpg", 1280, 720, 40000)

	// Commit 1: Tag sample 1 with "cat"
	c1, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1}}, "tags", OpAdd, "cat", "c1: cat to 1", nil)
	if err != nil {
		t.Fatalf("apply c1: %v", err)
	}

	// Commit 2: Tag samples 2, 3 with "cat"
	c2, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{2, 3}}, "tags", OpAdd, "cat", "c2: cat to 2,3", nil)
	if err != nil {
		t.Fatalf("apply c2: %v", err)
	}

	// Commit 3: Tag samples 1, 2 with "dog"
	c3, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1, 2}}, "tags", OpAdd, "dog", "c3: dog to 1,2", nil)
	if err != nil {
		t.Fatalf("apply c3: %v", err)
	}

	// Check HEAD is c3
	head, err := cat.Head(ctx)
	if err != nil || head == nil || *head != c3.ID {
		t.Fatalf("HEAD = %v, want %d", head, c3.ID)
	}

	// Undo 1: Revert c3 -> "dog" should disappear, "cat" should remain on 1, 2, 3
	newHead1, err := cat.Undo(ctx, &c3.ID)
	if err != nil {
		t.Fatalf("Undo c3: %v", err)
	}
	if newHead1 == nil || *newHead1 != c2.ID {
		t.Errorf("Undo new HEAD = %v, want c2 (%d)", newHead1, c2.ID)
	}

	tags, _ := cat.TagCounts(ctx, Filter{}, nil)
	if len(tags) != 1 || tags[0].Tag != "cat" || tags[0].Count != 3 {
		t.Errorf("after undo1, tags = %+v, want [{Tag: cat, Count: 3}]", tags)
	}

	// Undo 2: Revert c2 -> "cat" should only remain on sample 1
	newHead2, err := cat.Undo(ctx, &c2.ID)
	if err != nil {
		t.Fatalf("Undo c2: %v", err)
	}
	if newHead2 == nil || *newHead2 != c1.ID {
		t.Errorf("Undo new HEAD = %v, want c1 (%d)", newHead2, c1.ID)
	}

	tags, _ = cat.TagCounts(ctx, Filter{}, nil)
	if len(tags) != 1 || tags[0].Tag != "cat" || tags[0].Count != 1 {
		t.Errorf("after undo2, tags = %+v, want [{Tag: cat, Count: 1}]", tags)
	}

	// Undo 3: Revert c1 -> All tags should disappear, HEAD becomes nil
	newHead3, err := cat.Undo(ctx, &c1.ID)
	if err != nil {
		t.Fatalf("Undo c1: %v", err)
	}
	if newHead3 != nil {
		t.Errorf("Undo c1 returned HEAD = %v, want nil", newHead3)
	}

	head, _ = cat.Head(ctx)
	if head != nil {
		t.Errorf("HEAD after all undos = %v, want nil", head)
	}

	tags, _ = cat.TagCounts(ctx, Filter{}, nil)
	if len(tags) != 0 {
		t.Errorf("after undo3, tags = %+v, want []", tags)
	}

	// Undo 4: Undo on empty commit history must return ErrNoParentCommit
	_, err = cat.Undo(ctx, nil)
	if err != ErrNoParentCommit {
		t.Errorf("undo on empty history err = %v, want ErrNoParentCommit", err)
	}
}

// TestUndo_ExpectedHeadConflict ensures concurrent mutations are detected and rejected.
func TestUndo_ExpectedHeadConflict(t *testing.T) {
	ctx := context.Background()
	cat, db := setupTestCatalog(t)

	seedSample(t, db, 1, "img1.jpg", 1920, 1080, 50000)

	// Create commit 1
	c1, err := cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1}}, "tags", OpAdd, "tag1", "c1", nil)
	if err != nil {
		t.Fatalf("apply c1: %v", err)
	}

	// Create commit 2
	_, err = cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1}}, "tags", OpAdd, "tag2", "c2", nil)
	if err != nil {
		t.Fatalf("apply c2: %v", err)
	}

	// Client tries to undo expecting HEAD to be c1 (but someone else already committed c2)
	_, err = cat.Undo(ctx, &c1.ID)
	if err != ErrHeadMoved {
		t.Errorf("Undo with stale expected_head err = %v, want ErrHeadMoved", err)
	}
}
