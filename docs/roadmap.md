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

## 2. No extension points — *resolved 2026-08-10*

Adding anything used to require forking the Go binary and editing core, so every feature had
to pass through one maintainer and niche needs (DICOM, satellite imagery, a specific label
format) never landed. A project whose stated goal is community sustainability had no
mechanism for it.

Addressed in [plugins.md](plugins.md): operators (an action run against a selection,
declaring JSON-Schema inputs the UI renders as a form) and panels (a registered UI surface,
mounted at `sidebar` / `sample-detail` / `tab`), following FiftyOne's split of the same idea.
Go's lack of a practical in-process plugin story settled the mechanism — plugins run as
external HTTP services, registered via static config, with both operator execution and panel
UI proxied through data777's own server so a plugin is never called directly from the
browser. An operator does its work through the same public API a script would, so it can
never do anything `POST /api/undo` cannot revert.

Long-running operators (embedding computation, batch inference) return a `job_id` backed by
the job model resolved in item 6, below.

## 3. The data model is only tags — *resolved 2026-08-10*

A sample used to carry `tags: []string` and nothing else, which could not express
"show me images where the model predicted *car* below 0.5 confidence but the ground
truth says *car*."

Addressed in [api.md](api.md#fields): a typed field schema (`scalar` / `tags` / `labels`,
with `classification` / `detection` / `keypoints` label types), `Filter` generalized from
hardcoded top-level keys to a predicate list over named fields (with `elem_match` for
per-object conditions on a labels field), and the commit model split into `set` (bulk,
selection-scoped, membership fields only — same free-inversion property tags always had)
and `patch` (per-sample value edits, bounded by human review volume rather than dataset
size).

**Left open by that resolution, not solved:** cheap undo for a bulk, pipeline-driven
overwrite of a non-tag scalar field across an arbitrary-size selection. `set`'s free
inversion only holds for membership fields; a scalar overwrite needs each sample's prior
value to revert, which does not compress the way a bitmap delta does. Revisit if a
concrete use case for large-scale scalar reclassification shows up — segmentation masks are
similarly deferred, since mask storage is its own design question.

## 4. No embeddings or similarity search — *resolved 2026-08-10*

At 10 million samples, 200 per screen means 50,000 screens. Manual browsing is not a
strategy, so tagging is the *output* of curation rather than the method. What actually
works at that scale is vector-based: near-duplicate removal, diversity sampling against a
labeling budget, "find more like this", and outlier detection. LightlyStudio is built
entirely around this.

Addressed in [api.md](api.md#fields): a fourth field kind, `embedding` (fixed dims, a
declared metric), with its own bulk write path rather than going through commits — an
embedding is a model's output, not a curation decision, so there's nothing there to version
or undo. `Filter` gained a `near` operator and `sort` a `near` variant, both reusing the same
`/api/samples` pagination contract rather than adding a parallel search endpoint. Backing
storage follows the established small-to-large arc:
[brute-force by default, Qdrant past ~1M vectors](architecture.md#why-a-brute-force-default-for-vectors-and-qdrant-beyond-that).

**Scope line drawn deliberately:** near-duplicate removal, diversity sampling, and outlier
detection are curation *workflows* built on top of `near`, not new query syntax — they
become [operators](plugins.md) (item 2) that search and then write `set` commits, which is
why item 2 needed to land first. Bolting workflow-specific verbs onto the filter language
itself was the tempting shortcut avoided here.

## 5. `Filter` should grow into a view pipeline — *additive*

"Pick 10,000 samples, balanced across classes, for the labeling budget" is not a filter.
A filter selects by condition; this selects by sampling policy. FiftyOne chains stages —
match, sort, sample, group, limit — each transforming a view.

This can wait because the existing flat filter becomes the first stage of a pipeline
without breaking anything: `{"stages": [{"type": "filter", …}]}`.

## 6. No job model — *resolved 2026-08-10*

Computing embeddings for 10M images takes hours on a GPU, and applying a large `set` commit
can itself take real wall-clock time to resolve a filter into a bitmap — the original design
had one hardcoded job with one global status (`/api/index`, `/api/index/status`), where two
users could not run two jobs, nothing could be cancelled, progress was a single counter, and
a restart lost everything.

Addressed in [api.md](api.md#jobs): a general `Job` resource (`queued` / `running` /
`succeeded` / `failed` / `canceled`, with progress and a typed result) that `set`/`patch`
commits, undo, indexing, and plugin operators all go through — indexing's ad hoc status
endpoint is retired in favor of it. `?wait=Ns` long-polling means small jobs still feel
synchronous without the API needing two response shapes.

**Retry** is not a separate mechanism: a job's input is the same request body a client
already has, so retrying a failure is resubmitting that request, which creates a new job. No
failed-job resume state needs to be kept around.

**Left open:** what happens to a job's bookkeeping across a server restart. Job records
(id, kind, status, progress, result) are persisted so history survives a restart, but a Go
goroutine does not — a job still `running` or `queued` when the process stops cannot resume
mid-computation. The honest behavior is to mark such jobs `failed` on the next startup
(with a distinct error, e.g. "interrupted by restart") rather than silently losing them or
pretending they completed; the client's own retry (resubmit) is what recovers. Not yet
implemented, but the intended behavior is recorded here so it doesn't get decided
accidentally.

Worth noting that FiftyOne sells orchestration in its enterprise tier, so this is a gap
data777 fills for free.

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

1. ~~Data model and filter generalization (item 3)~~ — done, see [api.md](api.md#fields)
2. ~~Extension point contract (item 2)~~ — done, see [plugins.md](plugins.md)
3. ~~Job model (item 6)~~ — done, see [api.md](api.md#jobs); resolved out of order because
   item 2's long-running operators and item 3's original `set`-commit design both turned out
   to depend on it once the scale implications of "apply a commit" were worked through
4. ~~Vector index layer, including similarity ordering (item 4)~~ — done, see
   [api.md](api.md#fields) and [architecture.md](architecture.md#data-layers)
5. API additions for SDK access patterns (item 1)
6. View pipeline and the "does not do" section (items 5, 7) — independent of everything above,
   can happen anytime
