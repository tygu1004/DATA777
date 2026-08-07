CREATE TABLE samples (
  id          INTEGER PRIMARY KEY,
  path        TEXT NOT NULL UNIQUE,
  filename    TEXT NOT NULL,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  filesize    INTEGER NOT NULL,
  format      TEXT NOT NULL,
  indexed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE commits (
  id          INTEGER PRIMARY KEY,
  parent_id   INTEGER REFERENCES commits(id),
  message     TEXT,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE commit_ops (
  id          INTEGER PRIMARY KEY,
  commit_id   INTEGER NOT NULL REFERENCES commits(id),
  sample_id   INTEGER NOT NULL REFERENCES samples(id),
  tag         TEXT NOT NULL,
  op          TEXT NOT NULL CHECK (op IN ('add','remove'))
);

CREATE TABLE head (
  id          INTEGER PRIMARY KEY CHECK (id = 1),
  commit_id   INTEGER REFERENCES commits(id)
);

CREATE TABLE sample_tags (
  sample_id   INTEGER NOT NULL REFERENCES samples(id),
  tag         TEXT NOT NULL,
  PRIMARY KEY (sample_id, tag)
);

INSERT INTO head (id, commit_id) VALUES (1, NULL);
