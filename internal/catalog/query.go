package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"

	"github.com/RoaringBitmap/roaring/v2"
)

// ponytail: evaluate() materializes every sample in Go rather than pushing predicates into
// SQL. Correct at any dataset size, but scans the whole table per query — the same ceiling
// architecture.md documents for SQLiteCatalog generally (fine to ~10M rows). ClickHouseCatalog
// is the documented scale-out path (api.md#internal-interface) and is not implemented here.
func (c *SQLiteCatalog) evaluate(ctx context.Context, f Filter, atCommit *int64) ([]Sample, *int64, error) {
	samples, err := c.baseSamples(ctx, atCommit)
	if err != nil {
		return nil, nil, err
	}
	working := samples

	var seed *int64
	sorted := false
	for _, stage := range f.Stages {
		switch stage.Type {
		case "match":
			working, err = c.applyMatch(ctx, working, stage.Match)
			if err != nil {
				return nil, nil, err
			}
		case "sort":
			if stage.Sort == nil {
				return nil, nil, fmt.Errorf("sort stage missing sort spec")
			}
			working, err = c.applySort(ctx, working, *stage.Sort)
			if err != nil {
				return nil, nil, err
			}
			sorted = true
		case "sample":
			working, seed, err = applySample(working, SampleStage{Size: stage.Size, Balance: stage.Balance, Seed: stage.Seed})
			if err != nil {
				return nil, nil, err
			}
		case "rollup":
			working, err = c.applyRollup(ctx, working, stage.By)
			if err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, fmt.Errorf("unknown stage type %q", stage.Type)
		}
	}

	if !sorted {
		working, err = c.applySort(ctx, working, SortSpec{Field: "id", Dir: "asc"})
		if err != nil {
			return nil, nil, err
		}
	}
	return working, seed, nil
}

func (c *SQLiteCatalog) applyMatch(ctx context.Context, in []Sample, preds []Predicate) ([]Sample, error) {
	out := in
	for _, p := range preds {
		kind, err := c.fieldKind(ctx, p.Field)
		if err != nil {
			return nil, err
		}

		switch kind {
		case KindScalar:
			filtered := out[:0:0]
			for _, s := range out {
				val, ok := scalarValue(s, p.Field)
				if !ok {
					continue
				}
				match, err := compareValue(val, p.Op, p.Value)
				if err != nil {
					return nil, fmt.Errorf("field %q: %w", p.Field, err)
				}
				if match {
					filtered = append(filtered, s)
				}
			}
			out = filtered

		case KindTags:
			filtered := out[:0:0]
			for _, s := range out {
				match, err := evalTagsPredicate(s.Tags, p.Op, p.Value)
				if err != nil {
					return nil, err
				}
				if match {
					filtered = append(filtered, s)
				}
			}
			out = filtered

		case KindLabels:
			if p.Op != "elem_match" {
				return nil, fmt.Errorf("labels field %q only supports elem_match", p.Field)
			}
			var nested []Predicate
			if err := json.Unmarshal(p.Value, &nested); err != nil {
				return nil, fmt.Errorf("elem_match value must be a predicate array: %w", err)
			}
			filtered := out[:0:0]
			for _, s := range out {
				match, err := evalElemMatch(s.Labels[p.Field], nested)
				if err != nil {
					return nil, err
				}
				if match {
					filtered = append(filtered, s)
				}
			}
			out = filtered

		case KindEmbedding:
			if p.Op != "near" {
				return nil, fmt.Errorf("embedding field %q only supports near", p.Field)
			}
			var near NearValue
			if err := json.Unmarshal(p.Value, &near); err != nil {
				return nil, fmt.Errorf("near value: %w", err)
			}
			filtered, err := c.filterNear(ctx, out, p.Field, near)
			if err != nil {
				return nil, err
			}
			out = filtered

		default:
			return nil, fmt.Errorf("unknown field kind %q for %q", kind, p.Field)
		}
	}
	return out, nil
}

func (c *SQLiteCatalog) resolveQueryVector(ctx context.Context, field string, sampleID *int64, vector []float32) ([]float32, error) {
	if len(vector) > 0 {
		return vector, nil
	}
	if sampleID == nil {
		return nil, fmt.Errorf("near requires either sample_id or vector")
	}
	return c.vector.Get(ctx, field, *sampleID)
}

func (c *SQLiteCatalog) filterNear(ctx context.Context, in []Sample, field string, near NearValue) ([]Sample, error) {
	def, err := c.namedFieldDef(ctx, field)
	if err != nil {
		return nil, err
	}
	query, err := c.resolveQueryVector(ctx, field, near.SampleID, near.Vector)
	if err != nil {
		return nil, err
	}

	candidates := roaring.New()
	for _, s := range in {
		candidates.Add(uint32(s.ID))
	}

	scored, err := c.vector.Search(ctx, field, query, def.Metric, 0, candidates)
	if err != nil {
		return nil, err
	}

	byID := make(map[int64]Sample, len(in))
	for _, s := range in {
		byID[s.ID] = s
	}

	out := []Sample{}
	for _, sc := range scored {
		if near.MaxDistance != nil && sc.Distance > *near.MaxDistance {
			continue
		}
		if s, ok := byID[sc.ID]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// applyRollup implements the `rollup` stage (media.md#rolling-frames-up-to-their-parent):
// replace the view with the distinct samples a field's values name. `parent_id` resolves by
// ID lookup against the whole table — a frame's parent video is generally outside the frame
// match that got it here. Any other field (group_id included) keeps one representative sample
// per distinct value, drawn from the matched set itself, since a group_id is an opaque shared
// identity rather than a sample id to look up.
func (c *SQLiteCatalog) applyRollup(ctx context.Context, in []Sample, by string) ([]Sample, error) {
	if by == "" {
		return nil, fmt.Errorf(`rollup stage requires "by"`)
	}

	if by == "parent_id" {
		seen := map[int64]bool{}
		var ids []int64
		for _, s := range in {
			if s.ParentID > 0 && !seen[s.ParentID] {
				seen[s.ParentID] = true
				ids = append(ids, s.ParentID)
			}
		}
		all, err := c.baseSamples(ctx, nil)
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]Sample, len(all))
		for _, s := range all {
			byID[s.ID] = s
		}
		out := []Sample{}
		for _, id := range ids {
			if s, ok := byID[id]; ok {
				out = append(out, s)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out, nil
	}

	seen := map[string]bool{}
	out := []Sample{}
	for _, s := range in {
		val, ok := scalarValue(s, by)
		if !ok {
			return nil, fmt.Errorf("rollup: unknown or non-scalar field %q", by)
		}
		key := fmt.Sprintf("%v", val)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *SQLiteCatalog) namedFieldDef(ctx context.Context, name string) (FieldDef, error) {
	fields, err := c.Schema(ctx)
	if err != nil {
		return FieldDef{}, err
	}
	for _, f := range fields {
		if f.Name == name {
			return f, nil
		}
	}
	return FieldDef{}, fmt.Errorf("unknown field %q", name)
}

func (c *SQLiteCatalog) applySort(ctx context.Context, in []Sample, spec SortSpec) ([]Sample, error) {
	out := make([]Sample, len(in))
	copy(out, in)

	if spec.Near != nil {
		query, err := c.resolveQueryVector(ctx, spec.Near.Field, spec.Near.SampleID, spec.Near.Vector)
		if err != nil {
			return nil, err
		}
		def, err := c.namedFieldDef(ctx, spec.Near.Field)
		if err != nil {
			return nil, err
		}
		candidates := roaring.New()
		for _, s := range out {
			candidates.Add(uint32(s.ID))
		}
		scored, err := c.vector.Search(ctx, spec.Near.Field, query, def.Metric, 0, candidates)
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]Sample, len(out))
		for _, s := range out {
			byID[s.ID] = s
		}
		ranked := []Sample{}
		for _, sc := range scored {
			if s, ok := byID[sc.ID]; ok {
				ranked = append(ranked, s)
			}
		}
		// samples without a vector for this field cannot be ranked by distance; append last,
		// ordered by id so the result stays deterministic.
		ranked, err = appendMissing(ranked, out)
		if err != nil {
			return nil, err
		}
		return ranked, nil
	}

	field := spec.Field
	if field == "" {
		field = "id"
	}
	dir := spec.Dir
	if dir == "" {
		dir = "asc"
	}

	sort.SliceStable(out, func(i, j int) bool {
		vi, oki := scalarValue(out[i], field)
		vj, okj := scalarValue(out[j], field)
		if !oki || !okj {
			return out[i].ID < out[j].ID
		}
		less := lessValue(vi, vj)
		if dir == "desc" {
			if vi == vj {
				return out[i].ID < out[j].ID // id is always the tiebreaker
			}
			return !less
		}
		if vi == vj {
			return out[i].ID < out[j].ID
		}
		return less
	})
	return out, nil
}

func appendMissing(ranked, all []Sample) ([]Sample, error) {
	seen := make(map[int64]bool, len(ranked))
	for _, s := range ranked {
		seen[s.ID] = true
	}
	rest := []Sample{}
	for _, s := range all {
		if !seen[s.ID] {
			rest = append(rest, s)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].ID < rest[j].ID })
	return append(ranked, rest...), nil
}

func lessValue(a, b any) bool {
	if an, ok := a.(float64); ok {
		if bn, ok := b.(float64); ok {
			return an < bn
		}
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return as < bs
		}
	}
	return false
}

// applySample implements the `sample` pipeline stage (api.md#view-pipeline): reduce to `size`
// samples, deterministically for a given seed, optionally balanced across a field's distinct
// values.
func applySample(in []Sample, spec SampleStage) ([]Sample, *int64, error) {
	seed := spec.Seed
	if seed == nil {
		s := rand.Int63()
		seed = &s
	}
	rng := rand.New(rand.NewSource(*seed))

	if spec.Balance == nil {
		shuffled := make([]Sample, len(in))
		copy(shuffled, in)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if len(shuffled) > spec.Size {
			shuffled = shuffled[:spec.Size]
		}
		sort.Slice(shuffled, func(i, j int) bool { return shuffled[i].ID < shuffled[j].ID })
		return shuffled, seed, nil
	}

	// ponytail: balance groups a sample by a single value (scalar field, or "the one tag it
	// has among the requested field" for a tags field). A sample matching zero or several
	// distinct values for the balance field falls into an "other" bucket rather than being
	// counted in more than one group — exact multi-label balancing is a heavier design this
	// MVP doesn't need yet.
	groups := map[string][]Sample{}
	for _, s := range in {
		key := balanceKey(s, spec.Balance.Field)
		groups[key] = append(groups[key], s)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	quota := spec.Size / max(1, len(keys))
	picked := []Sample{}
	for _, k := range keys {
		members := groups[k]
		rng.Shuffle(len(members), func(i, j int) { members[i], members[j] = members[j], members[i] })
		take := min(quota, len(members))
		picked = append(picked, members[:take]...)
	}
	// fill remaining slots (rounding, or groups that ran out) from whatever's left, in a
	// deterministic order derived from the same seed.
	if len(picked) < spec.Size {
		takenIDs := make(map[int64]bool, len(picked))
		for _, s := range picked {
			takenIDs[s.ID] = true
		}
		var rest []Sample
		for _, s := range in {
			if !takenIDs[s.ID] {
				rest = append(rest, s)
			}
		}
		rng.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
		need := spec.Size - len(picked)
		if need > len(rest) {
			need = len(rest)
		}
		picked = append(picked, rest[:need]...)
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].ID < picked[j].ID })
	return picked, seed, nil
}

func balanceKey(s Sample, field string) string {
	if val, ok := scalarValue(s, field); ok {
		return fmt.Sprintf("%v", val)
	}
	// tags field: exactly one matching tag on the sample makes an unambiguous group
	if field == "tags" {
		if len(s.Tags) == 1 {
			return s.Tags[0]
		}
		return "\x00other"
	}
	return "\x00other"
}

func paginate(items []Sample, offset int, cursor string, limit int) ([]Sample, string, error) {
	start := 0
	if cursor != "" {
		idx, err := resumeIndex(items, cursor)
		if err != nil {
			return nil, "", err
		}
		start = idx
	} else if offset > 0 {
		start = offset
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]

	next := ""
	if end < len(items) {
		next = encodeCursor(items[end-1].ID)
	}
	return page, next, nil
}

func encodeCursor(lastID int64) string {
	payload, _ := json.Marshal(struct {
		ID int64 `json:"id"`
	}{lastID})
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(payload)
}

// resumeIndex finds the position right after the cursor's sample in the freshly evaluated,
// already-ordered item list. ponytail: if that sample no longer matches (deleted, or filtered
// out by a mutation since the cursor was issued), resume from the start rather than trying to
// reconstruct its old rank — a full walk restarting once around a rare edit is an acceptable
// trade for not carrying a persistent cursor index.
func resumeIndex(items []Sample, cursor string) (int, error) {
	raw, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor: %w", err)
	}
	var payload struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, fmt.Errorf("invalid cursor: %w", err)
	}
	for i, s := range items {
		if s.ID == payload.ID {
			return i + 1, nil
		}
	}
	return 0, nil
}
