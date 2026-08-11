// Package vectorindex implements the VectorIndex interface from docs/api.md#vectorindex:
// approximate-nearest-neighbor reads over embedding fields, chosen independently of Catalog.
// BruteForce is the only implementation — an external ANN engine (Qdrant recommended, see
// architecture.md#why-a-brute-force-default-for-vectors-and-qdrant-beyond-that) is a documented
// scale-out path left for when a field actually crosses ~1M vectors.
package vectorindex

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	"github.com/RoaringBitmap/roaring/v2"
)

type ScoredSample struct {
	ID       int64   `json:"id"`
	Distance float32 `json:"distance"`
}

// Index deviates from api.md's Go sketch in one way: Search takes an optional candidate-ID
// bitmap instead of a catalog.Filter, since this package cannot import internal/catalog
// (which itself evaluates filters and would need to import this package to run `near`).
// internal/catalog evaluates the non-`near` predicates of a Filter into a candidate set and
// passes it here — same "search restricted to a filtered set" behavior the doc describes.
type Index interface {
	Upsert(ctx context.Context, field string, id int64, vector []float32) error
	Delete(ctx context.Context, field string, id int64) error
	Get(ctx context.Context, field string, id int64) ([]float32, error)
	Search(ctx context.Context, field string, query []float32, metric string, k int, candidateIDs *roaring.Bitmap) ([]ScoredSample, error)
}

type BruteForce struct {
	db *sql.DB
}

func NewBruteForce(db *sql.DB) *BruteForce {
	return &BruteForce{db: db}
}

func encodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeVector(buf []byte) []float32 {
	v := make([]float32, len(buf)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return v
}

func (b *BruteForce) Upsert(ctx context.Context, field string, id int64, vector []float32) error {
	_, err := b.db.ExecContext(ctx,
		`INSERT INTO embeddings (field, sample_id, vector) VALUES (?, ?, ?)
		 ON CONFLICT(field, sample_id) DO UPDATE SET vector = excluded.vector`,
		field, id, encodeVector(vector),
	)
	if err != nil {
		return fmt.Errorf("upsert embedding: %w", err)
	}
	return nil
}

func (b *BruteForce) Delete(ctx context.Context, field string, id int64) error {
	_, err := b.db.ExecContext(ctx, `DELETE FROM embeddings WHERE field = ? AND sample_id = ?`, field, id)
	if err != nil {
		return fmt.Errorf("delete embedding: %w", err)
	}
	return nil
}

func (b *BruteForce) Get(ctx context.Context, field string, id int64) ([]float32, error) {
	var buf []byte
	err := b.db.QueryRowContext(ctx, `SELECT vector FROM embeddings WHERE field = ? AND sample_id = ?`, field, id).Scan(&buf)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no embedding for sample %d in field %q", id, field)
	}
	if err != nil {
		return nil, fmt.Errorf("get embedding: %w", err)
	}
	return decodeVector(buf), nil
}

// distance is smaller-is-closer for every supported metric: cosine and dot are converted to
// (1 - similarity) so callers never branch on metric when sorting.
func distance(metric string, a, b []float32) float32 {
	switch metric {
	case "l2":
		var sum float32
		for i := range a {
			d := a[i] - b[i]
			sum += d * d
		}
		return float32(math.Sqrt(float64(sum)))
	case "dot":
		var dot float32
		for i := range a {
			dot += a[i] * b[i]
		}
		return 1 - dot
	default: // cosine
		var dot, na, nb float32
		for i := range a {
			dot += a[i] * b[i]
			na += a[i] * a[i]
			nb += b[i] * b[i]
		}
		if na == 0 || nb == 0 {
			return 1
		}
		return 1 - dot/float32(math.Sqrt(float64(na))*math.Sqrt(float64(nb)))
	}
}

// Search is a linear scan — O(n·d) per query, documented as accurate up to roughly a million
// vectors per field (architecture.md). candidateIDs, when non-nil, restricts the scan to a
// filtered set exactly rather than over-fetching and re-filtering, since a brute-force index
// visits every candidate anyway (api.md#vectorindex).
func (b *BruteForce) Search(ctx context.Context, field string, query []float32, metric string, k int, candidateIDs *roaring.Bitmap) ([]ScoredSample, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT sample_id, vector FROM embeddings WHERE field = ?`, field)
	if err != nil {
		return nil, fmt.Errorf("scan embeddings: %w", err)
	}
	defer rows.Close()

	var scored []ScoredSample
	for rows.Next() {
		var id int64
		var buf []byte
		if err := rows.Scan(&id, &buf); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		if candidateIDs != nil && !candidateIDs.Contains(uint32(id)) {
			continue
		}
		scored = append(scored, ScoredSample{ID: id, Distance: distance(metric, query, decodeVector(buf))})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].Distance < scored[j].Distance })
	if k > 0 && len(scored) > k {
		scored = scored[:k]
	}
	return scored, nil
}
