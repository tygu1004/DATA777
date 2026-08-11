# data777 Media and Sequence Model

How video, synchronized multi-sensor captures, and media types beyond images fit into the
contracts defined in [api.md](api.md). Resolves
[roadmap item 10](roadmap.md#10-video-and-sequences-are-absent-from-the-contract--resolved-2026-08-11).

The result is deliberately small: **five scalar fields, one pipeline stage that was already
reserved, and one plugin manifest field.** Everything else — tags, labels, embeddings,
commits, undo, similarity search — applies to a video frame with no new machinery, because a
frame is an ordinary sample.

## What the prior art does, and where it breaks

| | Model | Borrowed | Not borrowed |
|---|---|---|---|
| [FiftyOne](https://docs.voxel51.com/user_guide/using_views.html#video-views) | A video sample holds a `frames` map — one MongoDB document per frame. Multi-sensor capture uses `group` + named `slice`s, with one slice active at a time | Frame-level labels; the group/slice concept itself | One document per frame is the scale ceiling: an hour at 30fps is 108,000 documents, and tagging 100,000 frames is 100,000 document writes. Frame labels also have no version history at all |
| [Rerun](https://rerun.io/docs/concepts/timelines) | Entities and components logged against several *named* timelines (`frame_nr`, `sensor_time`, `log_time`), each stream sparse along them | The idea that position is a value on a shared timeline — a 30fps camera and a 10Hz lidar cannot be aligned by frame number | Multiple named timelines. That solves robotics debugging, where log time and sensor time genuinely disagree; a curation tool needs one axis (see [Non-goals](#non-goals)) |
| CVAT | Task split into jobs of frame chunks; on-demand decoding | Chunked review assignment — deferred, it is a workflow feature, not a data model one | — |

## The model

Five scalar fields, added to the fixed set every `Catalog` implementation carries
([api.md](api.md#fields)):

| Field | Type | Meaning |
|---|---|---|
| `media_type` | string | `image` \| `video` \| `frame` \| `point_cloud` \| `audio` — extensible, see [Other media types](#other-media-types) |
| `parent_id` | int | `0` for a root sample; otherwise the sample this one was derived from (a frame → its video) |
| `group_id` | int | `0` for none; otherwise **the identity of a shared timeline** |
| `t` | float | position on that timeline, in seconds. `0` for a root image |
| `slice` | string | which sensor within the group (`cam_left`, `lidar`); `""` when ungrouped |

Stated as one sentence: **`group_id` identifies a timeline, `t` is a position on it, `slice`
says which sensor produced this view of it, and `parent_id` says which media file this row
was derived from.**

`parent_id` and `group_id` are orthogonal, and both are needed. `parent_id` is vertical — a
frame belongs to a video. `group_id` is horizontal — the left camera's sample and the right
camera's sample are peers describing the same instant. A rig of three synchronized cameras is
three video samples sharing a `group_id`; **their frames inherit that `group_id` from the
parent**, so "all three camera views at t=42.3s" is expressible with predicates that already
exist:

```jsonc
{ "match": [
    { "field": "group_id", "op": "eq",  "value": 7 },
    { "field": "t",        "op": "gte", "value": 42.2 },
    { "field": "t",        "op": "lte", "value": 42.4 }
] }
```

No new filter syntax. A still multi-camera rig is the degenerate case — several samples
sharing a `group_id` with `t = 0` — so one mechanism covers both.

Two more scalars are recorded at index time where they apply: `duration` (video, audio) and
`fps` (video). They are ordinary metadata, not part of the addressing model.

## Why frames are samples, not subdocuments

This is the one real design decision here, and it goes the opposite way from FiftyOne.

FiftyOne nests frames inside a video sample because its sample *is* a media file and MongoDB
nests documents naturally. data777's sample is already an abstract row addressed by an
`int64`, and **every core primitive is keyed by exactly that**. Making a frame a sample means
each of these applies to frames without a line of new code:

| Primitive | Applied to a frame |
|---|---|
| Tags as roaring bitmaps | Unchanged. Tagging 100,000 frames is one bitmap operation, not 100,000 writes |
| Labels as `sample_id`-keyed rows | Unchanged. A box on a frame is that frame sample's label |
| Embeddings and the `near` filter/sort | Unchanged. Per-frame embeddings make "find frames like this one" fall out for free |
| `set` / `patch` commits and undo | Unchanged. **Frame-level label edits get version history**, which FiftyOne has for nothing |
| `Filter` predicates | Unchanged. The five fields above are scalars; `eq` / `in` / `lt` / `gte` express everything |
| `Selection` in `filter` mode | Unchanged, including at frame counts |

The cost is stated in [Ongoing costs](#ongoing-costs) below rather than buried: the default
view now has to scope itself, because a count over "samples" would otherwise mix videos and
their frames.

## Frame density: stride at index time

Materializing every frame is what puts FiftyOne against a wall, and it would do the same
here — an hour of 30fps video is 108,000 rows whether they are documents or samples.

**Frames are extracted at a stride, defaulting to 1fps.** Consecutive frames at 30fps are
near-duplicates of each other; no one curates at that density, which is why CVAT and FiftyOne
workflows both sample keyframes in practice. An hour becomes 3,600 rows and a
thousand-hour corpus becomes 3.6M — comfortable inside the
[target scale](architecture.md#target-scale).

Densifying a region — "give me every frame between 41s and 44s" — is an
[operator](plugins.md), not a core code path. It adds frame samples through the ordinary
write path, which means it needs no privileged access and is covered by the same job,
progress, and cancellation machinery as everything else.

**Frame identity is `(parent_id, t)`, and re-indexing upserts on it.** Reallocating sample
IDs on a re-index would silently misalign every tag bitmap that referenced those frames —
the bitmaps store positions, and nothing else would notice the shift.

## Rolling frames up to their parent

"Find videos containing at least one low-confidence car detection" is not expressible as a
predicate: the condition holds on frames, but the answer is a set of videos.

[api.md](api.md#view-pipeline) reserved a `group` stage for exactly this shape. It is
specified here as `rollup`:

```jsonc
{ "stages": [
    { "type": "match",  "match": [
        { "field": "media_type",  "op": "eq", "value": "frame" },
        { "field": "predictions", "op": "elem_match", "value": [
            { "field": "label",      "op": "eq", "value": "car" },
            { "field": "confidence", "op": "lt", "value": 0.5 } ] } ] },
    { "type": "rollup", "by": "parent_id" },
    { "type": "sample", "size": 500 }
] }
```

`by` accepts `parent_id`, `group_id`, or any scalar field. The stage replaces the view with
the distinct samples those values name — for `parent_id`, the parent videos; for `group_id`,
one representative per group. It composes with the stages already defined, which is the point
of it being a stage rather than a query flag.

A rollup's cost is the distinct-value cardinality of the field over the matched set, not the
matched set itself, so `sort` and `sample` after it operate on the reduced view.

## Filmstrips are atlases

The real expense in frame thumbnails is seeking: decoding an arbitrary offset means finding
the preceding keyframe and decoding forward to it. Doing that per frame, on demand, is the
version of this that does not survive contact with the target scale.

It does not have to be paid, because **a video's filmstrip is exactly the atlas image
[api.md](api.md#get-apiatlas-reserved--not-yet-implemented) already defines for the grid.**
Extracting the stride frames at
index time is one sequential decode — no seeking at all — packed row-major into a single
image. That one artifact then serves three consumers that would otherwise each need their own:

- the grid's video cell, which draws four to eight of its cells as a filmstrip preview
- scrubbing in the detail view, since the frames are already resident as one GPU texture
- the frame grid for that video, which is the same atlas at a different zoom

Thumbnail generation becomes a job kind (`thumbnail`) alongside `index` — it is long-running,
cancelable, and progress-reporting for the same reasons indexing is.

## Other media types

Once `media_type` exists, supporting a new modality needs three things, and the plugin
contract already provides two of them:

| Needed | Provided by |
|---|---|
| A viewer (point cloud renderer, audio waveform) | **Already exists** — a [panel](plugins.md#manifest) mounted at `sample-detail` |
| Label geometry for that modality (`detection_3d`, `segment`) | **Already exists** — `type` on [`POST /api/schema/fields`](api.md#post-apischemafields) |
| A thumbnail/preview generator | Added here |

The third is one manifest field:

```jsonc
{ "name": "pcd-renderer",
  "media_handlers": [
    { "media_type": "point_cloud", "extensions": [".pcd", ".ply"] }
  ] }
```

data777 calls `POST {url}/thumbnail` with a sample when it needs a preview for a media type it
does not handle itself. The core knows images and video; everything else attaches — the same
optional-attachment pattern used for [storage backends, analytical engines, and
authorization](architecture.md#optional-dependencies-not-required-ones).

This is what makes "handles diverse data" true without growing the core for each new format.
A modality that data777 itself never learns to decode is still fully curatable: it can be
filtered, tagged, labeled, embedded, versioned, and searched by similarity, because none of
those paths ever look at the pixels.

## Non-goals

- **Multiple named timelines (Rerun's model).** There is one axis, `t`. Rerun needs several
  because log time, sensor time, and simulation time genuinely disagree while debugging a
  robot; a curation tool asks "what is at this moment," not "which clock was right." If a
  second axis is ever needed it is another scalar field, not a timeline subsystem.
- **Object tracking across frames.** A track is a model's output, not a curation decision —
  the same line drawn for [embeddings](api.md#fields). A `track_id` on a label object
  expresses it today; a track-level editing UI is a separate question, deferred until frame
  review exists to build it on.
- **Indexing every frame by default.** Stride is the default; densifying is an operator.
- **Video transcoding in the core.** A worker's job, per
  [architecture.md](architecture.md#language) — unchanged by anything here.

## Ongoing costs

Recorded rather than glossed, since each one is a tax paid continuously rather than a
one-time implementation cost.

1. **Every default view has to scope itself.** With frames as samples, an unqualified
   `/api/samples/count` counts videos and their frames together. The root view carries an
   implicit `parent_id = 0`, and any UI surface that shows a total has to be explicit about
   which population it means. This is the one genuine drawback of the frames-as-samples
   choice, and it does not go away.
2. **`parent_id` belongs in the ClickHouse sort key** for "the frames of video 77" to benefit
   from sparse-index skipping. It interacts with the unresolved question of positional access
   under an arbitrary sort — see [roadmap item 8](roadmap.md#8-a-filter-spans-storage-engines-that-cannot-join).
3. **Group selection semantics are honest but need to be visible.** Tagging while a view is
   pinned to one slice tags that slice's samples only, which is correct — a bad label on the
   left camera is not bad on the right — but the UI has to make the scope obvious or it reads
   as a bug.
4. **Synchronization tolerance is currently the client's problem.** Matching a moment across
   sensors means `t` within some ± window, and every client picking its own is a bad default.
   Wrapping it — bucketing `t` inside `rollup by: group_id`, or a tolerance on the group
   itself — is left open.

## See also

- [API contract](api.md) — the fields, filter predicates, and pipeline stages this builds on.
- [Architecture](architecture.md) — storage layers and the scale constraints behind them.
- [Plugin contract](plugins.md) — panels, operators, and the media handler above.
