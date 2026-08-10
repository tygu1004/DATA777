# data777 API Contract

This document defines the HTTP API and the internal `Catalog` interface it is built on.

Both are designed against the project's [target scale](architecture.md#target-scale) of
1 billion samples. The single principle behind every signature here:

> **Exchange descriptors, not enumerations.**

A filter is the condition itself, not the IDs matching it. A selection is a rule, not a
list. Requests stay constant-size no matter how large the dataset is, which is what makes
"select every matching sample and tag it" a few hundred bytes instead of gigabytes.

---

## Descriptors

### Filter

Describes a subset of the dataset. An absent or empty filter means the whole dataset.

```jsonc
{
  "tags": {
    "all":  ["cat", "verified"],   // must have every one
    "any":  ["indoor", "outdoor"], // must have at least one
    "none": ["blurry"]             // must have none
  },
  "width":    { "gte": 1000 },
  "height":   { "lte": 4000 },
  "filesize": { "gte": 1024 },
  "format":   { "in": ["jpeg", "png"] },
  "filename": { "contains": "IMG_" },

  "sort": { "field": "id", "dir": "asc" }  // field: id | filename | filesize | width | height
}
```

Every key is optional; present keys are combined with AND. `sort` defaults to
`{"field": "id", "dir": "asc"}` and **must be total** — the grid addresses samples by
position, so the ordering has to be stable across requests. Implementations append `id`
as a tiebreaker for non-unique sort fields.

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
               "tags": ["cat"] } ] }
```

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

Applies one tag operation to a selection, as a single commit.

```jsonc
// request
{ "message": "tag cats", "selection": { /* Selection */ }, "tag": "cat", "op": "add" }

// response — 201
{ "id": 12, "parent_id": 11, "message": "tag cats",
  "created_at": "2026-08-10T12:00:00Z", "affected_count": 84021 }
```

The request body carries a **selection**, not an operation per sample. Tagging 500 million
samples is the same request size as tagging one. `affected_count` is computed server-side.

### `GET /api/commits`

Returns the commit chain from HEAD back through `parent_id` — not every commit that ever
existed, so that undone commits disappear from the history as expected.

```jsonc
{ "items": [ { "id": 12, "parent_id": 11, "message": "tag cats",
               "created_at": "...", "op_count": 84021, "is_head": true } ] }
```

### `POST /api/undo`

Moves HEAD to the parent commit. Returns `409` when there is no parent.

```jsonc
{ "head_commit_id": 11 }   // null once history is empty
```

Undo is a bitmap operation against the stored tag set, not a replay of per-sample rows.

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
    ListSamples(ctx context.Context, f Filter, offset, limit int) ([]Sample, error)
    CountSamples(ctx context.Context, f Filter) (int64, error)
    TagCounts(ctx context.Context, f Filter) ([]TagCount, error)

    ApplyTag(ctx context.Context, sel Selection, tag string, op Op) (Commit, error)
    ListCommits(ctx context.Context, offset, limit int) ([]Commit, error)
    Undo(ctx context.Context) (*int64, error)
}
```

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
| One tag operation per sample | `POST /api/commits` takes a selection; server computes `affected_count` |
| One commit row per affected sample | Commits store a tag, an operation, and a roaring bitmap |
| One HTTP request per thumbnail | `/api/atlas` batches a window into a single image |
