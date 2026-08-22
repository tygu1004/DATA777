package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestQuery_PredicateFiltering(t *testing.T) {
	ctx := context.Background()
	cat, db := setupTestCatalog(t)

	// Seed samples with various dimensions and sizes
	seedSample(t, db, 1, "car1.jpg", 1920, 1080, 50000)
	seedSample(t, db, 2, "car2.jpg", 3840, 2160, 150000)
	seedSample(t, db, 3, "dog1.jpg", 800, 600, 20000)
	seedSample(t, db, 4, "dog2.jpg", 1280, 720, 45000)

	// Tag sample 1, 2 with "vehicle" and "outdoor"
	_, _ = cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1, 2}}, "tags", OpAdd, "vehicle", "tag vehicles", nil)
	_, _ = cat.ApplySet(ctx, Selection{Mode: "explicit", IDs: []int64{1, 3}}, "tags", OpAdd, "outdoor", "tag outdoor", nil)

	// Test 1: Count total
	total, err := cat.CountSamples(ctx, Filter{}, nil)
	if err != nil || total != 4 {
		t.Errorf("Count total = %d, err = %v, want 4", total, err)
	}

	// Test 2: Filter by tag "vehicle"
	vehicleFilter := Filter{
		Stages: []Stage{
			{
				Type: "match",
				Match: []Predicate{
					{Field: "tags", Op: "all", Value: json.RawMessage(`["vehicle"]`)},
				},
			},
		},
	}
	vehicleCount, err := cat.CountSamples(ctx, vehicleFilter, nil)
	if err != nil || vehicleCount != 2 {
		t.Errorf("Count vehicle = %d, err = %v, want 2", vehicleCount, err)
	}

	// Test 3: Scalar predicate width >= 1920
	resolutionFilter := Filter{
		Stages: []Stage{
			{
				Type: "match",
				Match: []Predicate{
					{Field: "width", Op: "gte", Value: json.RawMessage(`1920`)},
				},
			},
		},
	}
	resSamples, err := cat.ListSamples(ctx, resolutionFilter, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListSamples: %v", err)
	}
	if len(resSamples.Items) != 2 {
		t.Errorf("ListSamples width >= 1920 got %d items, want 2", len(resSamples.Items))
	}

	// Test 4: Combined tags and scalar filter (vehicle AND width >= 3000)
	combinedFilter := Filter{
		Stages: []Stage{
			{
				Type: "match",
				Match: []Predicate{
					{Field: "tags", Op: "all", Value: json.RawMessage(`["vehicle"]`)},
					{Field: "width", Op: "gte", Value: json.RawMessage(`3000`)},
				},
			},
		},
	}
	combSamples, err := cat.ListSamples(ctx, combinedFilter, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListSamples combined: %v", err)
	}
	if len(combSamples.Items) != 1 || combSamples.Items[0].ID != 2 {
		t.Errorf("ListSamples combined got %+v, want sample ID 2", combSamples.Items)
	}
}

// TestQuery_SeedSampling verifies that the sample stage produces deterministic draws for the same seed.
func TestQuery_SeedSampling(t *testing.T) {
	ctx := context.Background()
	cat, db := setupTestCatalog(t)

	for i := int64(1); i <= 20; i++ {
		seedSample(t, db, i, fmt.Sprintf("img_%d.jpg", i), 100, 100, 1000)
	}

	seedVal := int64(42)
	sampleFilter := Filter{
		Stages: []Stage{
			{
				Type: "sample",
				Size: 5,
				Seed: &seedVal,
			},
		},
	}

	// Run 1
	res1, err := cat.ListSamples(ctx, sampleFilter, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListSamples draw 1: %v", err)
	}

	// Run 2
	res2, err := cat.ListSamples(ctx, sampleFilter, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListSamples draw 2: %v", err)
	}

	if len(res1.Items) != 5 || len(res2.Items) != 5 {
		t.Fatalf("sample sizes = %d, %d, want 5", len(res1.Items), len(res2.Items))
	}

	for i := range res1.Items {
		if res1.Items[i].ID != res2.Items[i].ID {
			t.Errorf("mismatch at position %d: draw1 = %d, draw2 = %d (same seed must produce identical draws)",
				i, res1.Items[i].ID, res2.Items[i].ID)
		}
	}
}
