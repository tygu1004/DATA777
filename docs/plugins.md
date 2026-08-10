# data777 Plugin Contract

Two extension points, matching the reference model in
[FiftyOne's plugin system](https://docs.voxel51.com/plugins/developing_plugins.html):
**operators** (an action run against a selection) and **panels** (a registered UI surface).
Resolves [roadmap item 2](roadmap.md#2-no-extension-points--resolved-2026-08-10).

## Why plugins are separate HTTP services, not Go plugins

Go's `plugin` package is Linux-only, fragile across compiler versions, and incompatible
with the single-static-binary, air-gap-friendly deployment this project committed to. It was
never seriously in consideration for the same reason `go-duckdb`'s CGO requirement and
Lance's prebuilt native libraries were rejected elsewhere in this project — see
[architecture.md](architecture.md#rejected-and-deferred-technologies).

A plugin here is an ordinary HTTP service, run however its author likes — subprocess,
container, remote host. This falls directly out of two decisions already made:

- [architecture.md](architecture.md#optional-dependencies-not-required-ones) already
  treats external engines as optional attachments behind an interface. A plugin is the same
  pattern at the application layer instead of the storage layer.
- An operator does its actual work by calling the same public API a human client or the
  [Python SDK](sdk.md) would use — reading samples, writing
  `set`/`patch` commits ([api.md](api.md#post-apicommits)). It needs no special write path
  into the datastore. In effect, **an operator is an SDK script with a declared UI form and
  a way for the dashboard to trigger it** — not a separate integration surface.

One consequence worth naming: because every mutation a plugin makes goes through the normal
commit log, a plugin can never do anything that `POST /api/undo` cannot revert. A buggy
auto-tagger is one undo away from gone, the same as a human's mistake.

## Registration

Plugins are declared in a config file (default `plugins.yaml` in the data directory, or
`--plugins-config`), listing base URLs — not self-registered at runtime. Anything that can
call this API can also write commits, so which processes get that trust is an admin
decision, made once, in a file — not something a process grants itself by showing up. This
also means no plugin traffic exists on an air-gapped deployment unless an admin configured
one.

```yaml
plugins:
  - name: blur-detector
    url: http://blur-detector:8090
  - name: embedding-viewer
    url: http://embedding-viewer:8091
```

At startup (and on `POST /api/plugins/reload`), data777 fetches
`GET {url}/data777-plugin.json` from each entry. A plugin that is unreachable is logged and
skipped — consistent with every other optional dependency in this project, startup does not
fail because one attachment is down.

## Manifest

What a plugin exposes to `GET {url}/data777-plugin.json`:

```jsonc
{
  "name": "blur-detector",
  "operators": [
    {
      "name": "tag-blurry",
      "label": "Tag blurry images",
      "selection": "required",     // required | optional | none
      "inputs": {                  // JSON Schema — the UI renders this as a form
        "type": "object",
        "properties": {
          "threshold": { "type": "number", "default": 0.3 }
        }
      }
    }
  ],
  "panels": [
    {
      "name": "embedding-scatter",
      "label": "Embedding space",
      "mounts": ["sidebar", "tab"]
    }
  ]
}
```

Input schemas are JSON Schema rather than a custom type system, so the UI can use an
off-the-shelf form renderer instead of one built for this project alone.

`selection` covers both per-sample actions (`required`, e.g. "tag these as blurry") and
dataset-level ones (`none`, e.g. "resync from S3") through the same mechanism, rather than
needing a second extension kind for the latter.

`mounts` lists where a panel is allowed to appear: `sidebar` (a persistent side panel),
`sample-detail` (inside the single-sample view), `tab` (a full workspace tab). This is the
*slot* mechanism [roadmap.md](roadmap.md#2-no-extension-points--resolved-2026-08-10) flagged as
missing — a toolbar of hardcoded buttons has nowhere for a plugin to attach; declared mounts
give it one.

## Data777-side endpoints

```
GET  /api/plugins                                    aggregated manifests, for building UI slots
POST /api/plugins/reload                              re-fetch every configured manifest
POST /api/plugins/{plugin}/operators/{operator}        run an operator
GET  /api/plugins/{plugin}/panels/{panel}/*             reverse-proxied panel UI
```

**Both operator execution and panel UI are proxied through data777's own server — a plugin
is never called directly from the browser.** This is a deliberate choice, not the default
you'd fall into: it means a plugin only ever needs to be reachable from the core server (one
network hop, same trust boundary as the database connections already in place), never from
whatever machine happens to be running the browser. In a `docker compose` deployment, that
is the difference between exposing one port and exposing one per plugin; it also sidesteps
CORS and mixed-content entirely, since the iframe `src` for a panel is always same-origin
(`/api/plugins/{plugin}/panels/{panel}/...`), regardless of where the plugin container
actually runs.

### `POST /api/plugins/{plugin}/operators/{operator}`

Always **202**, a [job](api.md#jobs) (`kind: "operator"`) — the same envelope every other
mutation uses, so the UI never needs to know in advance whether a given operator is a
sub-second auto-tagger or an hours-long batch inference run.

```jsonc
// request
{ "selection": { /* Selection, if the operator declares selection: required|optional */ },
  "inputs": { "threshold": 0.3 } }

// response
{ "job_id": "job_2f9a01" }

// GET /api/jobs/{id} once succeeded — a fast operator is done well within one
// ?wait= call, so this is what most operator invocations look like end to end
{ "id": "job_2f9a01", "kind": "operator", "status": "succeeded",
  "result": { "commit_ids": [14] } }
```

The server attaches a scoped API token to the outbound request so the operator can call back
into data777's own API (list samples matching the selection, create commits) using the same
SDK a script would. The operator process reports its own progress back to the job (a small
callback the server hands it alongside the token) so a slow one — embedding computation,
batch inference — shows up the same way a large `set` commit's progress does, and can be
canceled the same way.

## Non-goals for this version

- **Dynamic self-registration.** A plugin cannot add itself by calling an endpoint; an admin
  edits the config file. Once registered, a plugin can write commits, so who gets that
  ability is deliberately not something a process grants itself.
- **Sandboxing plugin code.** data777 does not run plugin code — it calls an HTTP endpoint
  the plugin author operates and trusts. Isolation is whatever the deployment already
  provides (a separate container, a separate host).
- **A plugin marketplace or index.** Out of scope until there are plugins to index.
