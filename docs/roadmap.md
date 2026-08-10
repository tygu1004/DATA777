# Open Structural Work

What the architecture does not yet cover, ordered by how expensive it gets to add later.

This list came out of comparing data777 against projects that solved similar problems —
FiftyOne, LightlyStudio, Rerun, CVAT, and the lakeFS/DVC line of data versioning tools.
Items marked **structural** change contracts that other code will be built on, so they are
cheapest to settle before the UI exists. Items marked **additive** extend the contract
without breaking it and can wait.

---

## 1. No Python SDK — *structural*

Curation results are trapped in the browser. An engineer who tags 50,000 hard negatives in
the dashboard has no way to feed that selection into a training loop; FiftyOne and
LightlyStudio both make `pip install` the entry point. The reverse direction is missing too —
model predictions need a path *into* the tool to be reviewed, and today only folder indexing
exists.

The SDK itself is thin, because [the API contract](api.md) already exists and the dashboard
uses the same endpoints. What matters now is that scripts need access patterns a UI never
does, and those belong in the contract before an SDK is written:

- Streaming/cursor iteration over an entire result set, not a window
- Reads pinned to a commit, so a training export is not disturbed by concurrent edits
- Token authentication for non-browser clients
- View export in standard formats (COCO, YOLO, Parquet)

## 2. No extension points — *structural*

Adding anything — say, auto-tagging blurry images — currently requires forking the Go binary
and editing core. Every feature has to pass through one maintainer, and niche needs (DICOM,
satellite imagery, a specific label format) never land because they would bloat core. A
project whose stated goal is community sustainability has no mechanism for it.

FiftyOne splits extensions in two, which is a useful reference: **operators** (an action run
against a selection, declaring typed inputs the UI renders as a form) and **panels** (a
registered UI surface, like an embedding scatter plot).

Two consequences for data777 specifically:

- The UI needs *slots* where plugin-contributed actions and panels appear, plus a way to
  render plugin-declared forms. A toolbar with hardcoded buttons has to be rebuilt to get
  them.
- Go has no practical in-process plugin story — the `plugin` package is Linux-only and
  fragile. Extensions likely have to run out-of-process (subprocess or HTTP), which is
  itself an API design decision.

## 3. The data model is only tags — *structural, and revises existing contracts*

A sample carries `tags: []string` and nothing else. Real curation needs classifications,
bounding boxes, segmentation masks, keypoints, and arbitrary metadata — "show me images
where the model predicted *car* below 0.5 confidence but the ground truth says *car*"
is not expressible today.

This one is not additive. It revises what is already committed:

- **`Filter` hardcodes field names.** `{"tags": …, "width": …}` has no room for nested,
  per-object predicates. The generalization is a field path plus an operator —
  `{"field": "predictions.detections.confidence", "op": "lt", "value": 0.5}` — which is
  the model FiftyOne uses.
- **The commit model does not cover label edits.** Roaring bitmaps express *set membership*:
  a tag is on or off. A label is a *value* — nudging a bounding box by five pixels is not a
  set operation. Tag commits and value commits need different representations, and
  [api.md](api.md) currently only describes the first.

## 4. No embeddings or similarity search — *structural*

At 10 million samples, 200 per screen means 50,000 screens. Manual browsing is not a
strategy, so tagging is the *output* of curation rather than the method. What actually
works at that scale is vector-based: near-duplicate removal, diversity sampling against a
labeling budget, "find more like this", and outlier detection. LightlyStudio is built
entirely around this.

Consequences: a vector index becomes a fourth data layer (optional and external, following
the same pattern as ClickHouse), embeddings become a sample field (which depends on item 3),
and the API needs ordering by similarity to a reference sample — something the current
`sort`, which only handles scalar fields, cannot express.

## 5. `Filter` should grow into a view pipeline — *additive*

"Pick 10,000 samples, balanced across classes, for the labeling budget" is not a filter.
A filter selects by condition; this selects by sampling policy. FiftyOne chains stages —
match, sort, sample, group, limit — each transforming a view.

This can wait because the existing flat filter becomes the first stage of a pipeline
without breaking anything: `{"stages": [{"type": "filter", …}]}`.

## 6. No job model — *additive*

Computing embeddings for 10M images takes hours on a GPU. The current design has one
hardcoded job with one global status (`/api/index`, `/api/index/status`): two users cannot
run two jobs, nothing can be cancelled or retried, progress is a single counter, and a
restart loses everything.

What is needed is ordinary: submit a job, get an id, poll or stream progress, cancel, retry
failures, see history — with workers in separate processes. Additive, since indexing becomes
one job type among several.

Worth noting that FiftyOne sells orchestration in its enterprise tier, so this is a gap
data777 can fill for free.

## 7. The versioning boundary is not written down — *additive*

data777 versions **what you said about the data** — tags, labels, curation decisions — not
the data itself. Media files are referenced by path and treated as immutable.

This is the right boundary, but leaving it unstated invites the question "if I delete an
image, can I restore it from a commit?" Answering yes means storing image content and
reimplementing lakeFS or DVC, along with terabytes of file history. Anyone who needs file
versioning should run lakeFS underneath and point data777 at a branch.

An explicit "what this project does not do" section prevents contributors from building the
wrong thing.

---

## Suggested order

1. Data model and filter generalization (item 3) — the only one that revises committed
   contracts, so it blocks anything built on them
2. Extension point contract (item 2)
3. Vector index layer, including similarity ordering (item 4)
4. API additions for SDK access patterns (item 1)
5. View pipeline, job model, and the "does not do" section (items 5–7)
