# data777 API Contract

This document defines the HTTP API and the internal `Catalog` interface it is built on.

Both are designed against the project's [target scale](architecture.md#target-scale) of
1 billion samples. The single principle behind every signature here:

> **Exchange descriptors, not enumerations.**

A filter is the condition itself, not the IDs matching it. A selection is a rule, not a
list. Requests stay constant-size no matter how large the dataset is, which is what makes
"select every matching sample and tag it" a few hundred bytes instead of gigabytes.

---

## Fields

A sample has three kinds of fields. This split exists because they are stored, filtered,
and mutated differently — see [architecture.md](architecture.md#data-layers) for the
storage side.

| Kind | Examples | Storage | Mutated by |
|---|---|---|---|
| `scalar` | `width`, `height`, `filesize`, `format`, `filename` | fixed, written once at index time | never (re-index only) |
| `tags` | the `tags` field | roaring bitmap per tag | `set` commits |
| `labels` | `ground_truth`, `predictions`, any dataset-defined name | list of typed label objects per sample | `patch` commits |

Scalar fields are fixed and built into every `Catalog` implementation. **Label fields are
declared per dataset** — a dataset defines which named label fields it has and what type
each one holds, since "detections called `predictions`" is a dataset choice, not a schema
constant.

Three label types are defined now; more (e.g. segmentation masks) are deferred until a
dataset needs one, since mask storage is its own design question.

```jsonc
// classification — one label, optionally scored
{ "label": "cat", "confidence": 0.94 }

// detection — a label plus a normalized [x, y, w, h] box, 0..1 of image dimensions
{ "label": "car", "confidence": 0.81, "bbox": [0.12, 0.30, 0.20, 0.15] }

// keypoints — a label plus an ordered list of normalized [x, y] points
{ "label": "person", "points": [[0.40, 0.10], [0.38, 0.22]], "confidence": 0.77 }
```

### `GET /api/schema`

```jsonc
{ "fields": [
    { "name": "width",       "kind": "scalar", "type": "int" },
    { "name": "tags",        "kind": "tags" },
    { "name": "ground_truth","kind": "labels", "type": "detection" },
    { "name": "predictions", "kind": "labels", "type": "detection" }
] }
```

### `POST /api/schema/fields`

Declares a new label field. `{"name": "predictions", "kind": "labels", "type": "detection"}`.
Idempotent — declaring the same name and type twice is a no-op; declaring the same name with
a different type is a `409`.

---

## Descriptors

### Filter

Describes a subset of the dataset as a list of predicates, combined with AND. An absent or
empty list means the whole dataset.

```jsonc
{
  "match": [
    { "field": "width",  "op": "gte", "value": 1000 },
    { "field": "format",  "op": "in",  "value": ["jpeg", "png"] },
    { "field": "tags",    "op": "all", "value": ["cat", "verified"] },
    { "field": "tags",    "op": "none", "value": ["blurry"] },

    // elem_match: at least one label in the field's list satisfies every nested
    // predicate *jointly* (not just independently) — this is what "low-confidence
    // car detection" needs, since confidence and label must hold on the same box.
    { "field": "predictions", "op": "elem_match", "value": [
        { "field": "label",      "op": "eq", "value": "car" },
        { "field": "confidence", "op": "lt", "value": 0.5 }
    ] }
  ],
  "sort": { "field": "id", "dir": "asc" }
}
```

Every `field` is a name from `GET /api/schema` — this is what makes the filter type-agnostic
instead of hardcoding `tags`/`width`/etc. as special top-level keys. Operators depend on the
field's kind:

| Kind | Operators |
|---|---|
| `scalar` | `eq`, `neq`, `lt`, `lte`, `gt`, `gte`, `in`, `not_in`, `contains` (string only) |
| `tags` | `all`, `any`, `none` |
| `labels` | `elem_match` (value is a nested predicate list, evaluated per label object) |

`sort` defaults to `{"field": "id", "dir": "asc"}` and **must be total** — the grid
addresses samples by position, so ordering has to be stable across requests.
Implementations append `id` as a tiebreaker for non-unique sort fields. Sorting by a
`labels` field is not defined (there is no single value to order by); sorting by embedding
similarity is future work, tracked in [roadmap.md](roadmap.md#4-no-embeddings-or-similarity-search--structural).

Nesting `match` lists inside `match` for OR-of-AND groups is not supported yet — everything
today is one flat AND list, matching what existed before this generalized the shorthand
per-field keys into a uniform predicate.

**Encoding in query strings.** The filter travels as base64url-encoded JSON in a `filter`
parameter. Clients should serialize with sorted keys so that the same logical filter always
produces the same string, which keeps HTTP caching effective.

### Selection

Describes a set of samples a mutation applies to.

```jsonc
// An explicit handful — clicking a few thumbnails.
{ "mode": "explicit", "ids": [1, 2, 3] }

// Everything matching a filter, minus specific exclusions.
// This is how "select all" is expressed. Its size does not depend on the match count.
{ "mode": "filter", "filter": { /* Filter */ }, "excluded": [55, 91] }
```

`excluded` supports the common interaction of selecting everything and then deselecting a
few items. Servers reject an `explicit` selection whose `ids` array exceeds a configured
limit (default 10,000) and direct the client to use `filter` mode instead.

---

## Endpoints

All responses are JSON unless noted. Errors use `{"error": "message"}` with an appropriate
status code. Empty lists always serialize as `[]`, never `null`.

### `GET /api/samples`

Returns one window of samples. **Does not return a total** — see `/api/samples/count`.

| Parameter | Default | Notes |
|---|---|---|
| `filter` | none (all) | base64url JSON |
| `offset` | `0` | position in the sorted order |
| `limit` | `200` | capped server-side |

```jsonc
{ "items": [ { "id": 1, "path": "...", "filename": "a.jpg", "width": 4000,
               "height": 3000, "filesize": 812393, "format": "jpeg",
               "tags": ["cat"],
               "labels": { "predictions": [
                 { "label": "car", "confidence": 0.81, "bbox": [0.12, 0.30, 0.20, 0.15] }
               ] } } ] }
```

`labels` is keyed by field name (from `GET /api/schema`) and present only when the dataset
has declared at least one `labels` field; it is omitted entirely for datasets that only use
tags, so nothing changes for a tags-only sample.

Splitting the count out of this response is deliberate. The current implementation runs
`COUNT(*)` on every page request, which is a full scan per scroll tick.

> **Implementation note.** Positional access via `offset` is what a scrollable grid
> requires — dragging the scrollbar to 80% must work. Plain `LIMIT/OFFSET` is O(offset) and
> degrades badly in a row store. ClickHouse mitigates this substantially through sparse
> primary-index skipping on the sort key. Implementations that cannot skip efficiently should
> maintain a positional index over the active sort order.

### `GET /api/samples/count`

```jsonc
{ "count": 1043221 }
```

Takes `filter`. Called once when the filter changes, not once per page. Results are cached
per filter; exact counts at billion scale are expensive enough that implementations may
serve a cached value while a refresh runs.

### `GET /api/tags`

Tag names with the number of matching samples, for the filter sidebar. Takes an optional
`filter` to scope counts to the current view.

```jsonc
{ "items": [ { "tag": "cat", "count": 84021 }, { "tag": "blurry", "count": 133 } ] }
```

At scale these counts come from the roaring bitmap for each tag — a cardinality query, not
a scan.

### `POST /api/commits`

A commit has one of two shapes, because tag mutations and label edits compress differently.
This is a real fork, not a formatting choice — see the note after the examples.

**`kind: "set"`** — the same operation applied to every sample in a selection. Today this
is only valid for `tags` fields (`kind: "tags"` in the schema).

```jsonc
// request
{ "message": "tag cats", "kind": "set", "field": "tags",
  "selection": { /* Selection */ }, "op": "add", "value": "cat" }

// response — 201
{ "id": 12, "parent_id": 11, "message": "tag cats", "kind": "set",
  "created_at": "2026-08-10T12:00:00Z", "affected_count": 84021 }
```

The request body carries a **selection**, not an operation per sample — tagging 500 million
samples is the same request size as tagging one, and `affected_count` is computed
server-side.

**`kind: "patch"`** — a different value for each sample, for label edits made during
interactive review (nudging a box, correcting a classification).

```jsonc
// request
{ "message": "fix 3 boxes", "kind": "patch", "field": "ground_truth",
  "patches": [
    { "sample_id": 1042, "index": 0,    "value": { "label": "car", "bbox": [0.12, 0.30, 0.20, 0.15] } },
    { "sample_id": 1077, "index": null, "value": { "label": "car", "bbox": [0.55, 0.10, 0.18, 0.22] } }
  ] }

// response — 201
{ "id": 13, "parent_id": 12, "message": "fix 3 boxes", "kind": "patch",
  "created_at": "...", "affected_count": 2 }
```

`index` selects which label object in that sample's list is being replaced; `null` appends
a new one. The server reads and stores each prior value at apply time — a patch commit is
undoable without the client needing to know what it is overwriting.

> **Why two kinds, and why patch isn't held to the same scale guarantee as set.**
> A tag mutation is invertible for free: "add cat to selection S" undoes as "remove cat from
> S", regardless of any other tag's state, because a roaring bitmap only ever expresses
> membership. That property does not hold for a scalar-valued field — reverting a bulk
> overwrite of a classification value needs each sample's *previous* value, which differs
> across the selection in the general case. So `set` stays scoped to membership fields (`tags`
> today) where the free inversion holds, and label edits go through `patch`, whose cost is
> `len(patches)`, not the size of a selection.
>
> `patch` commits are safe at target scale because of *who produces them*: they come from a
> human reviewing samples one at a time, bounded by a review session (tens to low
> thousands), never by dataset size — nobody hand-edits a billion boxes. **Bulk,
> pipeline-driven overwrite of a non-tag scalar field across an arbitrary-size selection is
> not solved by this contract** — cheap undo for that case needs either accepting
> `O(affected)` cost for that operation specifically, or a compression trick not yet
> designed. Left open; tracked in [roadmap.md](roadmap.md#3-the-data-model-is-only-tags--structural-and-revises-existing-contracts).

### `GET /api/commits`

Returns the commit chain from HEAD back through `parent_id` — not every commit that ever
existed, so that undone commits disappear from the history as expected.

```jsonc
{ "items": [
    { "id": 13, "parent_id": 12, "message": "fix 3 boxes", "kind": "patch",
      "created_at": "...", "op_count": 2, "is_head": true },
    { "id": 12, "parent_id": 11, "message": "tag cats", "kind": "set",
      "created_at": "...", "op_count": 84021, "is_head": false }
] }
```

### `POST /api/undo`

Moves HEAD to the parent commit. Returns `409` when there is no parent.

```jsonc
{ "head_commit_id": 11 }   // null once history is empty
```

For a `set` commit, undo is a bitmap operation against the stored tag set. For a `patch`
commit, undo restores each patch's stored prior value. Neither replays one row per affected
sample from the client.

### `GET /api/thumbnails/{id}` · `GET /api/previews/{id}`

Single generated image. Cached immutably. Retained for the lightbox and for small datasets.

### `GET /api/atlas` *(reserved — not yet implemented)*

Returns one image containing many thumbnails packed row-major, for the GPU grid renderer.

| Parameter | Notes |
|---|---|
| `filter`, `offset`, `limit` | same window semantics as `/api/samples` |
| `cell` | cell edge length in pixels, e.g. `144` |

The response is a single image whose cells correspond, in order, to the samples that
`/api/samples` returns for the same parameters — so the client already knows the IDs and
does not need a companion manifest.

One HTTP request per thumbnail does not survive contact with the target scale. An atlas also
maps one-to-one onto a GPU texture atlas, so transport and renderer want the same shape.

### `POST /api/index` · `GET /api/index/status`

Unchanged. Starts indexing a source path and reports progress.

---

## Internal interface

Lives in `internal/catalog`, following the same shape as the existing
`storage.Source` abstraction (`Local` / `S3` behind one interface, chosen by a flag).

```go
type Catalog interface {
    Schema(ctx context.Context) ([]FieldDef, error)
    DefineField(ctx context.Context, f FieldDef) error

    ListSamples(ctx context.Context, f Filter, offset, limit int) ([]Sample, error)
    CountSamples(ctx context.Context, f Filter) (int64, error)
    TagCounts(ctx context.Context, f Filter) ([]TagCount, error)

    ApplySet(ctx context.Context, sel Selection, field string, op Op, value any) (Commit, error)
    ApplyPatch(ctx context.Context, field string, patches []Patch) (Commit, error)
    ListCommits(ctx context.Context, offset, limit int) ([]Commit, error)
    Undo(ctx context.Context) (*int64, error)
}
```

`ApplySet` replaces the earlier `ApplyTag` sketch — same shape, generalized from a
tag-specific method to any `kind: "tags"` field, since the schema is what determines which
fields support it rather than the method signature.

Implementations:

| Implementation | Flag | Status |
|---|---|---|
| `SQLiteCatalog` | `--catalog sqlite` (default) | current |
| `ClickHouseCatalog` | `--catalog clickhouse` | planned |

Selecting an implementation is a deployment decision. The HTTP API above and the entire
frontend are identical either way.

---

## How this contract addresses the scale constraints

| Problem | Resolution |
|---|---|
| Client accumulating the whole dataset | `/api/samples` returns a window only; the frontend fetches sparse chunks and evicts distant ones |
| `COUNT(*)` per page request | Count split into its own cached endpoint, called on filter change |
| `OFFSET` scanning | Sort key skipping in ClickHouse; positional index where unavailable |
| "Select all" as an ID list | `Selection` in `filter` mode — constant size |
| One tag operation per sample | `set` commits take a selection; server computes `affected_count` |
| One commit row per affected sample (tags) | `set` commits store a field, an operation, and a roaring bitmap |
| Per-sample label edits at dataset scale | `patch` commits are bounded by human review volume, not dataset size — see the note under `POST /api/commits` |
| Filter hardcoding field names | `match` is a predicate list over `GET /api/schema` fields, not fixed top-level keys |
| One HTTP request per thumbnail | `/api/atlas` batches a window into a single image |

**Not yet solved:** cheap, bulk-scale undo for a pipeline-driven overwrite of a non-tag
scalar field. Flagged rather than silently assumed away — see the `patch` note above.
