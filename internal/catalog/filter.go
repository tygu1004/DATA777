package catalog

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Predicate is one condition in a Filter's match list (api.md#filter).
type Predicate struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

type NearValue struct {
	SampleID    *int64    `json:"sample_id,omitempty"`
	Vector      []float32 `json:"vector,omitempty"`
	MaxDistance *float32  `json:"max_distance,omitempty"`
}

type SortSpec struct {
	Field string     `json:"field,omitempty"`
	Dir   string     `json:"dir,omitempty"` // asc | desc
	Near  *NearField `json:"near,omitempty"`
}

// NearField is "sort by distance to this reference", the sort-position form of a `near`
// predicate (api.md#filter: "near in match and near in sort are two uses of the same primitive").
type NearField struct {
	Field    string    `json:"field"`
	SampleID *int64    `json:"sample_id,omitempty"`
	Vector   []float32 `json:"vector,omitempty"`
}

type BalanceSpec struct {
	Field string `json:"field"`
}

// SampleStage groups a `sample` stage's own fields for internal use (query.go) — the wire
// format keeps them flat on Stage itself, see the comment there.
type SampleStage struct {
	Size    int
	Balance *BalanceSpec
	Seed    *int64
}

// Stage is one step of a view pipeline (api.md#view-pipeline). A `sample` stage's own fields
// (size/balance/seed) sit directly on the stage object, matching the doc's example — unlike
// `match`/`sort`, which nest under a key named after their own type.
type Stage struct {
	Type    string       `json:"type"` // match | sort | sample | rollup
	Match   []Predicate  `json:"match,omitempty"`
	Sort    *SortSpec    `json:"sort,omitempty"`
	Size    int          `json:"size,omitempty"`
	Balance *BalanceSpec `json:"balance,omitempty"`
	Seed    *int64       `json:"seed,omitempty"`
	By      string       `json:"by,omitempty"` // rollup: parent_id | group_id | any scalar field
}

// Filter is either the flat {match, sort} shape or a {stages: [...]} pipeline — the flat shape
// unmarshals into an equivalent two-stage pipeline, per api.md#view-pipeline.
type Filter struct {
	Stages []Stage `json:"-"`
}

func (f Filter) IsEmpty() bool {
	return len(f.Stages) == 0
}

func (f *Filter) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		f.Stages = nil
		return nil
	}

	var raw struct {
		Match  []Predicate `json:"match"`
		Sort   *SortSpec   `json:"sort"`
		Stages []Stage     `json:"stages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode filter: %w", err)
	}

	if raw.Stages != nil {
		f.Stages = raw.Stages
		return nil
	}

	var stages []Stage
	if len(raw.Match) > 0 {
		stages = append(stages, Stage{Type: "match", Match: raw.Match})
	}
	if raw.Sort != nil {
		stages = append(stages, Stage{Type: "sort", Sort: raw.Sort})
	}
	f.Stages = stages
	return nil
}

func (f Filter) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Stages []Stage `json:"stages"`
	}{f.Stages})
}

// DecodeFilterParam decodes the base64url-encoded JSON `filter` query parameter used by
// GET /api/samples, /api/samples/count, /api/tags (api.md#filter: "Encoding in query strings").
func DecodeFilterParam(encoded string) (Filter, error) {
	var f Filter
	if encoded == "" {
		return f, nil
	}
	raw, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(encoded)
	if err != nil {
		// tolerate standard padding too, since not every client strips it
		raw, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return f, fmt.Errorf("decode filter param: %w", err)
		}
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("parse filter json: %w", err)
	}
	return f, nil
}
