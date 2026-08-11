package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/RoaringBitmap/roaring/v2"
)

// baseSamples loads every sample with its tags and labels attached, as of atCommit (nil means
// live HEAD state). This is the input every pipeline stage narrows from — see the ponytail
// note on evaluate() in query.go for the scale ceiling this implies.
func (c *SQLiteCatalog) baseSamples(ctx context.Context, atCommit *int64) ([]Sample, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT id, path, filename, width, height, filesize, format,
		        media_type, parent_id, group_id, t, slice, duration, fps
		 FROM samples ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query samples: %w", err)
	}
	samples := []Sample{}
	for rows.Next() {
		var s Sample
		if err := rows.Scan(&s.ID, &s.Path, &s.Filename, &s.Width, &s.Height, &s.Filesize, &s.Format,
			&s.MediaType, &s.ParentID, &s.GroupID, &s.T, &s.Slice, &s.Duration, &s.FPS); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan sample: %w", err)
		}
		samples = append(samples, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	tagsByID, err := c.liveTagsBySample(ctx)
	if err != nil {
		return nil, err
	}
	labelsByID, err := c.liveLabelsBySample(ctx)
	if err != nil {
		return nil, err
	}

	if atCommit != nil {
		tagsByID, labelsByID, err = c.overlayAtCommit(ctx, *atCommit, tagsByID, labelsByID)
		if err != nil {
			return nil, err
		}
	}

	labelFields, err := c.labelFieldNames(ctx)
	if err != nil {
		return nil, err
	}

	for i := range samples {
		samples[i].Tags = tagsByID[samples[i].ID]
		if samples[i].Tags == nil {
			samples[i].Tags = []string{}
		}
		if len(labelFields) > 0 {
			labels := labelsToSlices(labelsByID[samples[i].ID])
			for _, f := range labelFields {
				if labels[f] == nil {
					labels[f] = []LabelValue{}
				}
			}
			samples[i].Labels = labels
		}
	}
	return samples, nil
}

func (c *SQLiteCatalog) labelFieldNames(ctx context.Context) ([]string, error) {
	fields, err := c.Schema(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range fields {
		if f.Kind == KindLabels {
			names = append(names, f.Name)
		}
	}
	return names, nil
}

func (c *SQLiteCatalog) liveTagsBySample(ctx context.Context) (map[int64][]string, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT tag, bitmap FROM tag_bitmaps`)
	if err != nil {
		return nil, fmt.Errorf("query tag bitmaps: %w", err)
	}
	defer rows.Close()

	result := map[int64][]string{}
	for rows.Next() {
		var tag string
		var buf []byte
		if err := rows.Scan(&tag, &buf); err != nil {
			return nil, fmt.Errorf("scan tag bitmap: %w", err)
		}
		bm := roaring.New()
		if _, err := bm.FromBuffer(buf); err != nil {
			return nil, fmt.Errorf("decode bitmap %q: %w", tag, err)
		}
		it := bm.Iterator()
		for it.HasNext() {
			id := int64(it.Next())
			result[id] = append(result[id], tag)
		}
	}
	return result, rows.Err()
}

// liveLabelsBySample keys by the label's stored idx (not list position) so overlayAtCommit
// can set/delete an entry by the exact idx a commit_patches row refers to, even if idx values
// have gaps — see the ponytail note there for why positional indexing would be a bug.
func (c *SQLiteCatalog) liveLabelsBySample(ctx context.Context) (map[int64]map[string]map[int]LabelValue, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT sample_id, field, idx, value FROM labels`)
	if err != nil {
		return nil, fmt.Errorf("query labels: %w", err)
	}
	defer rows.Close()

	result := map[int64]map[string]map[int]LabelValue{}
	for rows.Next() {
		var sampleID int64
		var field string
		var idx int
		var raw string
		if err := rows.Scan(&sampleID, &field, &idx, &raw); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		var lv LabelValue
		if err := json.Unmarshal([]byte(raw), &lv); err != nil {
			return nil, fmt.Errorf("decode label value: %w", err)
		}
		if result[sampleID] == nil {
			result[sampleID] = map[string]map[int]LabelValue{}
		}
		if result[sampleID][field] == nil {
			result[sampleID][field] = map[int]LabelValue{}
		}
		result[sampleID][field][idx] = lv
	}
	return result, rows.Err()
}

// labelsToSlices converts the idx-keyed working structure into the ordered, contiguous slices
// the public Sample.Labels shape uses, sorted by idx ascending.
func labelsToSlices(byField map[string]map[int]LabelValue) map[string][]LabelValue {
	out := map[string][]LabelValue{}
	for field, byIdx := range byField {
		indices := make([]int, 0, len(byIdx))
		for idx := range byIdx {
			indices = append(indices, idx)
		}
		sort.Ints(indices)
		list := make([]LabelValue, 0, len(indices))
		for _, idx := range indices {
			list = append(list, byIdx[idx])
		}
		out[field] = list
	}
	return out
}

func (c *SQLiteCatalog) ListSamples(ctx context.Context, f Filter, opts ListOptions) (ListResult, error) {
	items, seed, err := c.evaluate(ctx, f, opts.AtCommit)
	if err != nil {
		return ListResult{}, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	page, next, err := paginate(items, opts.Offset, opts.Cursor, limit)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page, NextCursor: next, Seed: seed}, nil
}

func (c *SQLiteCatalog) CountSamples(ctx context.Context, f Filter, atCommit *int64) (int64, error) {
	items, _, err := c.evaluate(ctx, f, atCommit)
	if err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}

func (c *SQLiteCatalog) TagCounts(ctx context.Context, f Filter, atCommit *int64) ([]TagCount, error) {
	items, _, err := c.evaluate(ctx, f, atCommit)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for _, s := range items {
		for _, t := range s.Tags {
			counts[t]++
		}
	}
	out := []TagCount{}
	for tag, n := range counts {
		out = append(out, TagCount{Tag: tag, Count: n})
	}
	return out, nil
}

func (c *SQLiteCatalog) GetSamplePath(ctx context.Context, id int64) (string, error) {
	var path string
	err := c.db.QueryRowContext(ctx, `SELECT path FROM samples WHERE id = ?`, id).Scan(&path)
	if err != nil {
		return "", fmt.Errorf("get sample path: %w", err)
	}
	return path, nil
}
