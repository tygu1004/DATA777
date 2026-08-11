// Package catalog implements the internal/catalog interface from docs/api.md: the typed
// field schema, the generalized Filter/Selection query language, and the set/patch commit
// model. SQLiteCatalog is the only implementation — ClickHouseCatalog is a documented
// scale-out path (architecture.md#data-layers) left for when a deployment actually needs it.
package catalog

import (
	"context"
	"time"
)

type FieldKind string

const (
	KindScalar    FieldKind = "scalar"
	KindTags      FieldKind = "tags"
	KindLabels    FieldKind = "labels"
	KindEmbedding FieldKind = "embedding"
)

type FieldDef struct {
	Name   string    `json:"name"`
	Kind   FieldKind `json:"kind"`
	Type   string    `json:"type,omitempty"`   // scalar: int|string; labels: classification|detection|keypoints
	Dims   int       `json:"dims,omitempty"`   // embedding only
	Metric string    `json:"metric,omitempty"` // embedding only: cosine|l2|dot
}

// LabelValue is a typed label object (classification/detection/keypoints), kept as a loose
// JSON map rather than a Go union type — the type tag lives in the field's schema, and typed
// deserialization is the Python SDK's job (docs/sdk.md#typed-models), not the wire format's.
type LabelValue map[string]any

// MediaType/ParentID/GroupID/T/Slice/Duration/FPS are the media.md addressing fields — every
// sample carries them (root images default to media_type=image, parent_id=0, t=0), though
// this build only ever produces images: video/frame ingestion is not implemented, only the
// addressing model these fields need to exist is.
type Sample struct {
	ID        int64                   `json:"id"`
	Path      string                  `json:"path"`
	Filename  string                  `json:"filename"`
	Width     int                     `json:"width"`
	Height    int                     `json:"height"`
	Filesize  int64                   `json:"filesize"`
	Format    string                  `json:"format"`
	MediaType string                  `json:"media_type"`
	ParentID  int64                   `json:"parent_id"`
	GroupID   int64                   `json:"group_id"`
	T         float64                 `json:"t"`
	Slice     string                  `json:"slice"`
	Duration  float64                 `json:"duration"`
	FPS       float64                 `json:"fps"`
	Tags      []string                `json:"tags"`
	Labels    map[string][]LabelValue `json:"labels,omitempty"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int64  `json:"count"`
}

type Op string

const (
	OpAdd    Op = "add"
	OpRemove Op = "remove"
)

// Selection describes a set of samples a mutation applies to (api.md#selection).
type Selection struct {
	Mode     string  `json:"mode"` // explicit | filter
	IDs      []int64 `json:"ids,omitempty"`
	Filter   *Filter `json:"filter,omitempty"`
	Excluded []int64 `json:"excluded,omitempty"`
}

type Patch struct {
	SampleID int64      `json:"sample_id"`
	Index    *int       `json:"index"` // nil appends
	Value    LabelValue `json:"value"`
}

type CommitKind string

const (
	CommitSet   CommitKind = "set"
	CommitPatch CommitKind = "patch"
)

type Commit struct {
	ID            int64      `json:"id"`
	ParentID      *int64     `json:"parent_id"`
	Message       string     `json:"message"`
	Kind          CommitKind `json:"kind"`
	Field         string     `json:"field"`
	CreatedAt     time.Time  `json:"created_at"`
	AffectedCount int        `json:"affected_count"`
	OpCount       int        `json:"op_count"`
	IsHead        bool       `json:"is_head"`
}

// ProgressFunc lets a long-running Catalog call report progress back to the job wrapping it
// (internal/jobs). Implementations should call it periodically, not per-row.
type ProgressFunc func(processed, total int64)

type ListOptions struct {
	Offset   int
	Cursor   string
	Limit    int
	AtCommit *int64
}

type ListResult struct {
	Items      []Sample
	NextCursor string
	// Seed echoes back a server-generated seed for a `sample` pipeline stage, per
	// api.md#view-pipeline, so the client can request identical pages of the same draw.
	Seed *int64
}

type Catalog interface {
	Schema(ctx context.Context) ([]FieldDef, error)
	DefineField(ctx context.Context, f FieldDef) error

	ListSamples(ctx context.Context, f Filter, opts ListOptions) (ListResult, error)
	CountSamples(ctx context.Context, f Filter, atCommit *int64) (int64, error)
	TagCounts(ctx context.Context, f Filter, atCommit *int64) ([]TagCount, error)

	ApplySet(ctx context.Context, sel Selection, field string, op Op, value string, message string, report ProgressFunc) (Commit, error)
	ApplyPatch(ctx context.Context, field string, patches []Patch, message string) (Commit, error)
	ListCommits(ctx context.Context, offset, limit int) ([]Commit, error)
	// Undo's expectedHead, when non-nil, guards against undoing a HEAD moved by a concurrent
	// commit from someone else — see the SQLiteCatalog doc comment for why this exists.
	Undo(ctx context.Context, expectedHead *int64) (*int64, error)
}
