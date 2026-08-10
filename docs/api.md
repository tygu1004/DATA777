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

A sample has four kinds of fields. This split exists because they are stored, filtered,
and mutated differently — see [architecture.md](architecture.md#data-layers) for the
storage side.

| Kind | Examples | Storage | Mutated by |
|---|---|---|---|
| `scalar` | `width`, `height`, `filesize`, `format`, `filename` | fixed, written once at index time | never (re-index only) |
| `tags` | the `tags` field | roaring bitmap per tag | `set` commits |
| `labels` | `ground_truth`, `predictions`, any dataset-defined name | list of typed label objects per sample | `patch` commits |
| `embedding` | `clip_embedding`, any dataset-defined name | fixed-length float vector per sample, in a vector index | bulk upsert, **not commits** — see below |

Scalar fields are fixed and built into every `Catalog` implementation. **Label and
embedding fields are declared per dataset** — a dataset defines which named fields it has
and what type/dimensionality each one holds, since "detections called `predictions`" or "a
512-dim CLIP embedding called `clip_embedding`" is a dataset choice, not a schema constant.

**Embeddings are not versioned like tags or labels, and that's deliberate.** A tag or label
records a human judgment about a sample — that's what the commit log exists to track. An
embedding is a model's output: reproducible from the sample and the model, not a decision
anyone made about it. Treating it as commit history would mean an "undo" for a number nobody
chose, and would put every re-embedding run through the same versioned-mutation machinery
built for curation state. So embeddings get their own bulk write path (below), outside the
commit/job-for-mutation split — closer in spirit to how a thumbnail is generated than to how
a tag is applied.

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
    { "name": "predictions", "kind": "labels", "type": "detection" },
    { "name": "clip_embedding", "kind": "embedding", "dims": 512, "metric": "cosine" }
] }
```

`metric` is one of `cosine`, `l2`, `dot` — how the vector index compares two vectors in that
field. It's part of the field's declaration, not a per-query choice, because an ANN index is
typically built around one metric.

### `POST /api/schema/fields`

Declares a new `labels` or `embedding` field, e.g.
`{"name": "predictions", "kind": "labels", "type": "detection"}` or
`{"name": "clip_embedding", "kind": "embedding", "dims": 512, "metric": "cosine"}`.
Idempotent — declaring the same name with the same definition twice is a no-op; declaring the
same name with a different type/dims/metric is a `409`.

### `POST /api/embeddings/{field}` · `GET /api/embeddings/{field}/{sample_id}`

Bulk upsert and single read for a declared `embedding` field. **Not a commit** — see the note
above; there is nothing here to undo, and no job wraps a single call (the caller batches).

```jsonc
// POST request
{ "items": [
    { "sample_id": 1042, "vector": [0.0123, -0.881, /* … 512 floats */] },
    { "sample_id": 1077, "vector": [0.0091,  0.442, /* … */] }
] }

// GET response
{ "sample_id": 1042, "vector": [0.0123, -0.881, /* … */] }
```

Computing embeddings for a dataset is naturally an [operator](plugins.md) — a model
inference loop that reads samples via the public API and writes vectors back through this
endpoint in batches — rather than a new job kind of its own; it already gets progress,
cancellation, and a token from the [job](#jobs) machinery an operator runs under.

`GET /api/samples` does **not** include vector values in list responses — a 512-float vector
per row would make every grid page an order of magnitude heavier for a field most UI never
needs to see raw. Reading one back is for debugging or export, not the scrollable grid.

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
    ] },

    // near: keep only samples within max_distance of a reference — either a literal
    // vector or, more commonly, "like this other sample" by id
    { "field": "clip_embedding", "op": "near",
      "value": { "sample_id": 1042, "max_distance": 0.3 } }
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
| `embedding` | `near` (value is `{vector}` or `{sample_id}`, plus optional `max_distance`) |

`sort` defaults to `{"field": "id", "dir": "asc"}` and **must be total** — the grid
addresses samples by position, so ordering has to be stable across requests.
Implementations append `id` as a tiebreaker for non-unique sort fields. Sorting by a
`labels` field is not defined (there is no single value to order by).

Sorting by similarity uses a distinct shape instead of `dir`, since "closest first" isn't
ascending or descending anything — it's the `near` predicate's own value pulled up to where
`sort` usually goes:

```jsonc
{ "match": [ { "field": "tags", "op": "all", "value": ["outdoor"] } ],
  "sort": { "near": { "field": "clip_embedding", "sample_id": 1042 } } }
```

This is "find images similar to sample 1042, restricted to ones tagged outdoor" — the same
`/api/samples` pagination contract as everything else, so a similarity-ordered result scrolls
exactly like any other view. It reuses this one query mechanism rather than adding a separate
"similarity search" endpoint alongside it.

`near` in `match` and `near` in `sort` are two uses of the same primitive, not two features:
filtering by distance keeps everything within a radius, ordering by distance keeps
everything, closest first. A client can combine both — filter to a radius, then also make
sure the closest ones page in first.

**What `near` deliberately does not cover.** Near-duplicate removal ("keep one from each
tight cluster"), diversity sampling ("pick 10,000 spread across the embedding space"), and
outlier detection are curation *workflows* built on top of similarity search, not filters —
a Filter says what to keep from a static condition, these decide what to keep by looking at
the whole neighborhood structure. They belong as [operators](plugins.md) that call `near`
searches internally and write `set` commits (e.g. tag the duplicates), not as new query
syntax — see [roadmap item 4](roadmap.md#4-no-embeddings-or-similarity-search--resolved-2026-08-10)
for why this scope line was drawn here.

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

### Jobs

Every mutation that isn't O(1) — applying a `set` commit, undo, indexing, a plugin operator
— runs as a job instead of blocking the HTTP request that started it. This matters most for
`set`: turning a filter into the affected-set bitmap can mean streaming hundreds of millions
of matching row IDs out of the analytical store before anything can be written, which is not
something to hold an HTTP connection open for.

```jsonc
{ "id": "job_8f2e1a", "kind": "commit",           // commit | undo | index | operator
  "status": "running",                             // queued | running | succeeded | failed | canceled
  "progress": { "processed": 84021, "total": 500000000 },  // total omitted if not known in advance
  "created_at": "...", "started_at": "...", "finished_at": null,
  "error": null,
  "result": null }                                 // populated once succeeded; shape depends on kind
```

```
POST /api/jobs/{id}/cancel     cancel a queued or running job
GET  /api/jobs/{id}            job status; ?wait=Ns long-polls up to N seconds for a terminal state
GET  /api/jobs                 list, optionally filtered by ?status=
```

**Why failure never needs an explicit rollback.** A job computes its complete result — the
affected-set bitmap, the resolved patch list — in isolation, and touches the live commit log
and HEAD with exactly one atomic write at the very end. Nothing is visible to any reader
until that final write succeeds, so a job that errors or is canceled midway simply never
reaches it: there is no partial state to undo, because none was ever applied. The same
property makes concurrent reads safe during a huge job — `/api/samples` and `/api/tags`
always show either the state before the job or the state after it, never in between.

**Small jobs stay fast without a separate synchronous path.** Rather than branching the API
on operation size, a client can `GET /api/jobs/{id}?wait=2` right after creating one; a job
that finishes within the wait window returns its terminal state on that same call, so tagging
ten samples feels exactly as immediate as it did as a synchronous `201` — the size-dependent
case just keeps polling.

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

Always **202**, returning a job — see [Jobs](#jobs) above for why, and for how the
`?wait=` param keeps a small commit feeling instant. `{"job_id": "job_8f2e1a"}`.

A commit itself has one of two shapes, because tag mutations and label edits compress
differently. This is a real fork, not a formatting choice — see the note after the examples.

**`kind: "set"`** — the same operation applied to every sample in a selection. Today this
is only valid for `tags` fields (`kind: "tags"` in the schema).

```jsonc
// request
{ "message": "tag cats", "kind": "set", "field": "tags",
  "selection": { /* Selection */ }, "op": "add", "value": "cat" }

// GET /api/jobs/{id} once succeeded
{ "id": "job_8f2e1a", "kind": "commit", "status": "succeeded",
  "progress": { "processed": 84021, "total": 84021 },
  "result": { "commit_id": 12, "parent_id": 11, "affected_count": 84021 } }
```

The request body carries a **selection**, not an operation per sample — tagging 500 million
samples is the same request size as tagging one, and `affected_count` is computed
server-side as the job runs (`progress.processed`), not returned all at once at the end.

**`kind: "patch"`** — a different value for each sample, for label edits made during
interactive review (nudging a box, correcting a classification).

```jsonc
// request
{ "message": "fix 3 boxes", "kind": "patch", "field": "ground_truth",
  "patches": [
    { "sample_id": 1042, "index": 0,    "value": { "label": "car", "bbox": [0.12, 0.30, 0.20, 0.15] } },
    { "sample_id": 1077, "index": null, "value": { "label": "car", "bbox": [0.55, 0.10, 0.18, 0.22] } }
  ] }

// GET /api/jobs/{id} once succeeded — patch jobs are small by construction (see below),
// so in practice this is what a client sees from a single ?wait= call, not repeated polling
{ "id": "job_9c1b04", "kind": "commit", "status": "succeeded",
  "result": { "commit_id": 13, "parent_id": 12, "affected_count": 2 } }
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
> designed. Left open; tracked in [roadmap.md](roadmap.md#3-the-data-model-is-only-tags--resolved-2026-08-10).

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

**202**, a job (`kind: "undo"`). Returns `409` immediately, with no job created, when there
is no parent commit — that check doesn't need a job, it's a lookup against the current HEAD.

```jsonc
// GET /api/jobs/{id} once succeeded
{ "id": "job_1a2b3c", "kind": "undo", "status": "succeeded",
  "result": { "head_commit_id": 11 } }   // null once history is empty
```

For a `set` commit, undo applies the inverse of the bitmap that was stored *when the commit
was made* — it does not recompute the original filter, so its cost does not depend on how
expensive that filter was to evaluate the first time. For a `patch` commit, undo restores
each patch's stored prior value. In practice undo is close to instant either way, since both
paths are one bounded read plus one atomic pointer move; it goes through the job envelope
for the same reason everything else does — so the client never has to guess in advance
whether a given mutation will be fast.

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

### `POST /api/index`

**202**, a job (`kind: "index"`). `{"path": "/data/images"}` → `{"job_id": "..."}`.

This replaces the earlier bespoke `GET /api/index/status` — indexing was always this
pattern (submit, poll, watch a counter climb), just with a one-off status endpoint instead of
the general [Jobs](#jobs) resource. Folding it in is what makes indexing cancelable, and
gives it the same `?wait=` short-poll behavior as everything else, for free.

### `GET /api/plugins` · `POST /api/plugins/reload` · `POST /api/plugins/{plugin}/operators/{operator}` · `GET /api/plugins/{plugin}/panels/{panel}/*`

See [plugins.md](plugins.md) — the operator/panel extension contract. Both operator
execution and panel UI are proxied through these endpoints rather than called directly from
the browser, so a plugin only ever needs to be reachable from the server, never from the
client.

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

These methods stay ordinary synchronous Go calls — a call that takes a while to return is
not a problem inside the server process, only across an HTTP request. The async boundary
belongs to a separate `internal/jobs` package that wraps them, not to `Catalog` itself:

```go
type Jobs interface {
    Enqueue(ctx context.Context, kind string,
        run func(ctx context.Context, report ProgressFunc) (result any, err error)) (*Job, error)
    Get(ctx context.Context, id string) (*Job, error)
    Cancel(ctx context.Context, id string) error
    List(ctx context.Context, status string) ([]Job, error)
}
```

An HTTP handler for `POST /api/commits` enqueues a closure that calls `ApplySet`/`ApplyPatch`
from a worker goroutine, passing `report` down so `Catalog` can update `progress.processed`
as it streams matches; canceling a job cancels that goroutine's `context.Context`, which
`Catalog` implementations are expected to check periodically during a large scan. `Jobs` has
no ClickHouse/SQLite split — it is in-process bookkeeping regardless of which `Catalog` is
active.

Implementations:

| Implementation | Flag | Status |
|---|---|---|
| `SQLiteCatalog` | `--catalog sqlite` (default) | current |
| `ClickHouseCatalog` | `--catalog clickhouse` | planned |

Selecting an implementation is a deployment decision. The HTTP API above and the entire
frontend are identical either way.

### `VectorIndex`

A separate interface from `Catalog`, deliberately — the two are chosen independently (a
deployment can pair `SQLiteCatalog` with an external ANN engine, or `ClickHouseCatalog` with
the brute-force default; see [architecture.md](architecture.md#data-layers) for why).

```go
type VectorIndex interface {
    Upsert(ctx context.Context, field string, id int64, vector []float32) error
    Delete(ctx context.Context, field string, id int64) error
    Search(ctx context.Context, field string, query []float32, k int, f Filter) ([]ScoredSample, error)
}
```

`Search` taking a `Filter` is "similar to this, but only among samples matching this
condition" — the combined query `near` in `match` expresses at the API level. **How well a
backend honors that filter varies and is a backend-quality question, not something this
interface resolves generically.** A brute-force implementation filters trivially, since it
already visits every candidate. A true ANN index either needs native filtered search (some
do) or has to over-fetch past `k` and filter after, which can under-return on a selective
filter — noted here rather than glossed over, since it's a real accuracy/latency tradeoff an
operator author needs to know about, not an implementation detail this contract can hide.

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
| A large `set` commit blocking the HTTP request that started it | mutations run as [jobs](#jobs) — `202` plus a pollable `job_id`, not a held-open connection |
| Manual review as the only way to navigate 1B samples | `near` filter/sort surfaces similarity, so curation starts from structure instead of browsing every screen |
| One HTTP request per thumbnail | `/api/atlas` batches a window into a single image |

**Not yet solved:** cheap, bulk-scale undo for a pipeline-driven overwrite of a non-tag
scalar field. Flagged rather than silently assumed away — see the `patch` note above.
