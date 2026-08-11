-- media_type/parent_id/group_id/t/slice/duration/fps are the media.md addressing fields.
-- Every current indexer writes are plain images, so these all take their defaults (root,
-- ungrouped, t=0) — video/frame ingestion is not implemented, only the addressing model is.
CREATE TABLE samples (
  id          INTEGER PRIMARY KEY,
  path        TEXT NOT NULL UNIQUE,
  filename    TEXT NOT NULL,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  filesize    INTEGER NOT NULL,
  format      TEXT NOT NULL,
  media_type  TEXT NOT NULL DEFAULT 'image',
  parent_id   INTEGER NOT NULL DEFAULT 0,
  group_id    INTEGER NOT NULL DEFAULT 0,
  t           REAL NOT NULL DEFAULT 0,
  slice       TEXT NOT NULL DEFAULT '',
  duration    REAL NOT NULL DEFAULT 0,
  fps         REAL NOT NULL DEFAULT 0,
  indexed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Declared field schema (api.md#fields). Scalar fields are seeded once, fixed forever.
-- labels/embedding fields are added at runtime via POST /api/schema/fields.
CREATE TABLE fields (
  name    TEXT PRIMARY KEY,
  kind    TEXT NOT NULL CHECK (kind IN ('scalar','tags','labels','embedding')),
  type    TEXT,     -- scalar: int|string; labels: classification|detection|keypoints
  dims    INTEGER,  -- embedding only
  metric  TEXT       -- embedding only: cosine|l2|dot
);

INSERT INTO fields (name, kind, type) VALUES
  ('id', 'scalar', 'int'),
  ('width', 'scalar', 'int'),
  ('height', 'scalar', 'int'),
  ('filesize', 'scalar', 'int'),
  ('format', 'scalar', 'string'),
  ('filename', 'scalar', 'string'),
  ('path', 'scalar', 'string'),
  ('media_type', 'scalar', 'string'),
  ('parent_id', 'scalar', 'int'),
  ('group_id', 'scalar', 'int'),
  ('t', 'scalar', 'float'),
  ('slice', 'scalar', 'string'),
  ('duration', 'scalar', 'float'),
  ('fps', 'scalar', 'float'),
  ('tags', 'tags', NULL);

-- One roaring bitmap per tag value (architecture.md "Why roaring bitmaps for tag state").
CREATE TABLE tag_bitmaps (
  tag     TEXT PRIMARY KEY,
  bitmap  BLOB NOT NULL
);

-- Ordered list of typed label objects per sample per declared labels field.
CREATE TABLE labels (
  sample_id  INTEGER NOT NULL REFERENCES samples(id),
  field      TEXT NOT NULL REFERENCES fields(name),
  idx        INTEGER NOT NULL,
  value      TEXT NOT NULL, -- JSON: {label, confidence?, bbox?, points?}
  PRIMARY KEY (sample_id, field, idx)
);

-- Fixed-length vector per sample per declared embedding field. Bulk-written, never committed.
CREATE TABLE embeddings (
  field      TEXT NOT NULL REFERENCES fields(name),
  sample_id  INTEGER NOT NULL REFERENCES samples(id),
  vector     BLOB NOT NULL,
  PRIMARY KEY (field, sample_id)
);

-- Commits fork into "set" (bulk, selection-scoped, membership fields, free inversion via the
-- stored affected-set bitmap) and "patch" (per-sample value edits, prior value stored for undo).
-- See api.md#post-apicommits.
CREATE TABLE commits (
  id             INTEGER PRIMARY KEY,
  parent_id      INTEGER REFERENCES commits(id),
  message        TEXT,
  kind           TEXT NOT NULL CHECK (kind IN ('set','patch')),
  field          TEXT NOT NULL,
  op             TEXT,    -- set only: add|remove
  value          TEXT,    -- set only: the tag value
  bitmap         BLOB,    -- set only: affected-set roaring bitmap at apply time
  affected_count INTEGER NOT NULL DEFAULT 0,
  created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE commit_patches (
  id           INTEGER PRIMARY KEY,
  commit_id    INTEGER NOT NULL REFERENCES commits(id),
  sample_id    INTEGER NOT NULL,
  idx          INTEGER NOT NULL, -- resolved position; a patch that appended still gets one here
  prior_value  TEXT,             -- JSON, NULL if this patch appended a new label (undo = delete)
  new_value    TEXT NOT NULL
);

CREATE TABLE head (
  id          INTEGER PRIMARY KEY CHECK (id = 1),
  commit_id   INTEGER REFERENCES commits(id)
);
INSERT INTO head (id, commit_id) VALUES (1, NULL);

-- Job bookkeeping for the async mutation model (api.md#jobs). Records survive a restart;
-- the goroutine that was running one does not (internal/jobs marks it failed on startup).
CREATE TABLE jobs (
  id           TEXT PRIMARY KEY,
  kind         TEXT NOT NULL,
  status       TEXT NOT NULL,
  processed    INTEGER NOT NULL DEFAULT 0,
  total        INTEGER,
  error        TEXT,
  result       TEXT, -- JSON
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at   TIMESTAMP,
  finished_at  TIMESTAMP
);

-- Bearer tokens for non-browser clients (SDK, scripts, plugin operators). api.md#authentication.
CREATE TABLE tokens (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  hash        TEXT NOT NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at  TIMESTAMP
);
