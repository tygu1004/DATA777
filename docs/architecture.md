# data777 Architecture

This document records the architectural decisions behind data777, why they were made,
and which alternatives were rejected. It exists so that contributors do not have to
re-litigate settled questions, and so that anyone evaluating the project can see what
it is optimized for before reading a line of code.

## What this project is

An open-source tool for visualizing and curating machine-learning datasets — browsing,
filtering, tagging, and reviewing large collections of images and video.

The core commitment: **every feature stays free.** Comparable tools converge on a model
where individual/local workflows are free and team collaboration infrastructure is
commercial. data777 does not split along that line. If a revenue model is ever needed
it will come from managed hosting or consulting, not from withholding features.

## Target scale

**10 million to 1 billion samples.** This number is the single most important design
constraint in the project, and every interface is designed against the upper bound.

This matters because scale is not a dimension you can retrofit. Designs that work at
100k samples fail structurally — not gradually — at 100M:

| Naive design | Why it breaks at 1B |
|---|---|
| Load the dataset into memory client-side | 1B objects; the browser dies long before |
| `SELECT COUNT(*)` per page request | Full scan on every scroll |
| `LIMIT n OFFSET m` for random access | offset 500M scans 500M rows |
| "Select all" as a list of IDs | 1B-element array over the wire |
| Tag mutation as one operation per sample | 1B-row write for one user action |
| One HTTP request per thumbnail | 1B requests |

The rule that follows: **interfaces exchange descriptors, not enumerations.** A filter
is the condition itself, not the IDs matching it. A selection is a rule, not a list.
This is what makes "select everything and tag it" a constant-size request.

## Data layers

The workload splits into two halves with opposite characteristics, so the storage
splits with it.

| Layer | Characteristics | Default | Scale-out trigger | Scale-out |
|---|---|---|---|---|
| Sample metadata | Written once at index time, then large analytical scans, filters, counts | SQLite | > 10M samples | ClickHouse |
| Commits, HEAD, sessions | Small, frequently mutated, must be exact | SQLite | Multiple API replicas | PostgreSQL |
| Tag state | Set membership per tag | Roaring bitmaps (BLOB) | — | unchanged |
| Label state | Typed objects (classification/detection/keypoints) per sample, edited a few at a time by a human reviewer | SQLite, alongside sample metadata | — | unchanged |

Each layer sits behind an interface and is replaced independently. Application code and
the HTTP API do not change when an implementation is swapped.

Label state is not a fourth storage engine — it is closer in character to sample metadata
(read far more than written) than to tag state (which is pure set membership). It gets a
row of its own here because its *mutation* path is different from either: label edits are
per-sample value patches, not bitmap deltas or bulk analytical writes. See
[api.md](api.md#fields) for the field kinds this maps to and
[api.md](api.md#post-apicommits) for why patch commits do not face the same billion-row
risk that tag-per-sample rows did.

### Why ClickHouse for sample metadata

Columnar storage means a filter on `width` never reads the other columns, so counting
matching rows in a billion-row table takes tens to hundreds of milliseconds rather than
seconds. Its Go client (`clickhouse-go`) is pure Go, so it does not compromise the
single-static-binary property. Apache-2.0.

ClickHouse is weak at frequent small updates, which is exactly why tag state does not
live there as mutable rows (see below).

### Why the Postgres trigger is *multiple servers*, not *multiple users*

This is a common misreading worth stating plainly. data777 is a web server: ten
concurrent users still means **one Go process** writing to the database. SQLite's
single-writer constraint is handled inside that process, and in WAL mode readers never
block the writer. Commits happen at the speed of a human clicking a button, which is
nothing for SQLite.

Postgres becomes necessary when the API server itself is replicated behind a load
balancer — several processes cannot share a SQLite file safely — or when failover and
replication are required.

### Why roaring bitmaps for tag state

A tag is a set of sample IDs. Stored as rows, applying a tag to 500M samples is a 500M-row
write and undoing it is another. Stored as a [roaring bitmap](https://roaringbitmap.org/),
that same set is a few megabytes, and undo is a single set operation.

`RoaringBitmap/roaring` is pure Go and is the same structure used by InfluxDB, Bleve,
Lucene, and Elasticsearch. Because a bitmap is just a BLOB column, this layer is identical
whether the transactional store is SQLite or Postgres.

This also resolves the tension with ClickHouse: tag mutations never touch the analytical
store, so its weakness at small updates never comes into play.

## Optional dependencies, not required ones

data777 delegates heavy work to external engines rather than embedding them. The
distinction that matters is not "external vs embedded" but **required vs optional**:

- A **required** external service is adoption friction. Needing a database running before
  the tool does anything is the main reason people bounce off a project on first try.
- An **optional** external service is good design. The core runs without it; you attach it
  when your scale or feature needs justify it.

data777 takes the second form. A single binary with no external services handles the small
end. Point it at ClickHouse when the dataset outgrows that. This mirrors patterns already
used in the codebase — `storage.Source` with `Local`/`S3` implementations behind one
interface, and the planned `can(user, action, resource)` authorization gate that can be
backed by a simple role check or by Casbin/OPA.

There is no meaningful performance cost to keeping engines out of process. A container
network hop is 0.1–1ms against analytical queries measured in tens to hundreds of
milliseconds, and responses carry only the window currently on screen.

## Deployment tiers

The same image, three levels of effort:

1. **Individual** — one binary, no external services. SQLite, local filesystem.
2. **Team** — `docker compose up`: data777 + ClickHouse + an S3-compatible object store
   ([RustFS](https://rustfs.com/), Apache-2.0).
3. **Large scale** — Helm: replicated API servers, Postgres for transactional state,
   ClickHouse, cloud object storage.

No outbound network calls are required at any tier — air-gapped deployment is supported.
Telemetry and license checks are not present; if ever added they would be opt-in.

## Language

**Go**, and heavy computation is delegated rather than embedded.

The API server routes requests, manages sessions and the commit log, and relays queries
to the analytical engine. That is an I/O-bound workload where Go's simplicity, single-binary
output, and easy deployment all pay off, and where its weaker data-processing ecosystem
never becomes the bottleneck.

Rust would win if the analytical engine were embedded in-process — Lance, DataFusion,
Polars, and Arrow are all Rust-native. That path was considered and not taken; with
external delegation the advantage disappears. Heavy workers (embedding, video transcoding)
are separate processes and may be written in Rust or Python later without touching the
server.

## Version control as a first-class concept

Every mutation — tagging, label edits — is recorded as a commit. Point-in-time queries,
diffs, and audit logs fall out of this naturally, and undo is simply "return to the parent
commit" rather than a separate undo stack. A batch operation is one commit, so undoing a
bulk tag is a single action.

At target scale a commit stores *which rule was applied to which set* — a tag, an operation,
and a roaring bitmap — never one row per affected sample.

## Rejected and deferred technologies

Recorded so the same evaluations are not repeated.

**Lance / LanceDB** — *rejected.* Technically the best fit for multimodal ML datasets:
fast random access, built-in versioning, vector indexing. Its Go SDK requires CGO plus
separately downloaded prebuilt native libraries, which breaks the single-static-binary and
air-gap properties. This is the same pattern that disqualified Apache OpenDAL earlier. Lance
would likely have been the first choice had the project been written in Rust.

**Apache OpenDAL** — *rejected.* Considered as a storage abstraction, but its Go binding
depends on a prebuilt native shared library loaded at runtime, requiring per-platform native
binaries. The actual requirement was only "talk to S3-compatible endpoints", which the pure-Go
`aws-sdk-go-v2` S3 client satisfies with custom endpoint and path-style settings.

**go-duckdb** — *rejected.* Requires CGO. Became unnecessary once analytical work was
delegated to an external engine.

**MinIO as the self-hosted object store example** — *rejected* in favor of RustFS. The MinIO
server is AGPLv3, which imposes source-disclosure obligations when providing a network
service, and its web console moved to the commercial edition — the same "core features go
paid" pattern this project exists to avoid.

**MongoDB** — *rejected.* A required heavyweight stateful dependency, which is precisely the
adoption friction described above.

**Apache Iceberg** — *deferred, not rejected.* Iceberg is a table format, not a query engine,
so it does not compete with ClickHouse; ClickHouse can read Iceberg tables. `iceberg-go` is
pure Go with solid write support, and its snapshots would provide time travel for free, which
fits the version-control model well. Because it would sit behind the same `Catalog` interface,
adopting it later changes no application code — so the decision is left open.

## Frontend

Rendering a grid of hundreds of thousands of visible-over-time thumbnails is a GPU problem,
not a DOM problem.

- **PixiJS v8** with WebGPU preferred and automatic WebGL fallback, driving the sample grid.
  A DOM scroll container with a fixed canvas overlay preserves native scrollbars.
- **TanStack Virtual** computes the visible window. It is renderer-agnostic, so the same
  windowing math drives canvas rendering; it needs only the total count, never the items.
- **TanStack Query** for data fetching, using sparse chunk-keyed queries. Infinite-scroll page
  accumulation is deliberately avoided — dragging the scrollbar to an arbitrary position must
  work, and distant chunks must be evictable.
- **Tailwind CSS v4 + shadcn/ui** for the interface shell, dark theme.
- **React 19**.

Texture memory is bounded by an LRU cache; at 144×144 RGBA a thumbnail is roughly 83KB, so
resident textures must be capped rather than grown freely.

Thumbnail delivery batches multiple thumbnails into a single atlas image per request. One
HTTP request per thumbnail does not survive contact with the target scale, and an atlas maps
directly onto a GPU texture atlas, so the transport and the renderer want the same shape.

## See also

- [API contract](api.md) — the filter and selection descriptors, and the endpoints built on them.
- [Plugin contract](plugins.md) — operators and panels, run as external HTTP services rather
  than Go plugins, following the same optional-attachment pattern as the data layers above.
- [Open structural work](roadmap.md) — what this architecture does not yet cover, and which
  gaps revise the contracts above rather than extend them.
