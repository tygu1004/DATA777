# data777 Python SDK

Resolves [roadmap item 1](roadmap.md#1-no-python-sdk--resolved-2026-08-10). This document
covers the package itself; the access patterns it depends on — tokens, cursors, `at_commit`,
jobs — are specified in [api.md](api.md), not repeated here.

## Why a thin client, not a second implementation

A curation decision made in the dashboard is unreachable from a training script today, and a
model's predictions have no path into the dashboard for review — the dashboard is the only
first-class client. The SDK's job is to make it one of several, all speaking the same
contract, rather than becoming a second thing with its own logic to keep in sync.

Every method below is a direct translation of one HTTP call already defined in
[api.md](api.md). There is no client-side query planner, no local cache of dataset state, and
no reimplementation of filtering or selection — those stay server-side so the SDK, the
dashboard, and a [plugin operator](plugins.md) can never disagree about what a view contains.

## Shape

```python
import data777

client = data777.connect("https://data777.example.com", token="d777_...")

# Iteration — cursor-based, handles paging transparently (api.md's `cursor`, not `offset`)
for sample in client.samples(filter={"match": [{"field": "tags", "op": "all", "value": ["cat"]}]}):
    ...

# Tagging — creates a job, .wait() polls api.md's `?wait=` until terminal
job = client.tag(selection={"mode": "filter", "filter": {...}}, tag="reviewed", op="add")
job.wait()
print(job.result)  # {"commit_id": ..., "affected_count": ...}

# Similarity search — the `near` filter, not a separate method namespace
similar = client.samples(filter={"match": [
    {"field": "clip_embedding", "op": "near", "value": {"sample_id": 1042, "max_distance": 0.3}}
]})

# A view pinned to the commit current right now, so a long export isn't disturbed by
# curation that happens while it runs (api.md's `at_commit`)
view = client.view(at_commit=client.head())
view.export(format="coco", path="./export")
```

`data777.connect` is the only entry point — no separate classes for "read" vs "write" access,
matching the API itself having no such split (a token is coarse-grained, per
[api.md](api.md#authentication)).

## Typed models

Sample, Commit, and Job responses are [Pydantic](https://docs.pydantic.dev/) models generated
from the same field/label type definitions in [api.md](api.md#fields), not hand-maintained
copies — a `detection` label deserializes to a typed object with `.label`, `.confidence`,
`.bbox`, not a raw dict, and a script gets IDE autocomplete and a validation error instead of
a `KeyError` three steps later. Pydantic over a hand-rolled schema for the same reason
`aws-sdk-go-v2` was chosen over writing an S3 client from scratch: a mature, widely-used
library beats custom code doing the same job, matching this project's general stance on
[adopting popular tools](../CLAUDE.md) rather than minimizing dependencies.

## Export is client-side, not a server endpoint

`view.export(format=...)` runs entirely in the SDK: it iterates samples and labels through
the ordinary read API and writes COCO/YOLO/Parquet locally. This was a deliberate line, not
an oversight — teaching the *server* format-specific serialization would grow its surface
with every new ML framework's preferred layout, for something that is pure data
transformation the client already has all the inputs for. The same scoping call already made
for [near-duplicate removal and diversity sampling](roadmap.md#4-no-embeddings-or-similarity-search--resolved-2026-08-10):
keep the server's job to answering queries and applying commits, push workflow-specific logic
to the layer that's supposed to have it.

## Sync first

Synchronous, matching how ML engineers actually reach for a tool like this — a script or a
notebook cell, not a service. An async variant is not ruled out, but isn't built until a
concrete use case (a plugin operator driving many concurrent requests, say) asks for it
rather than being spawned speculatively alongside the sync one from day one.

## Non-goals

- **No local dataset cache or offline mode.** Every call is a live request; the target scale
  makes "sync the dataset locally" the wrong default, not a missing feature.
- **No query builder DSL beyond passing `Filter`/`Selection` dicts (or the Pydantic models
  for them).** They're already small, serializable, and shared with every other client —
  wrapping them in SDK-specific builder syntax would be one more thing to keep in sync with
  [api.md](api.md) rather than reusing it directly.
