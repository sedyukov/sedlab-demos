-- 0001: base schema. Portable SQL (SQLite now; types and statements also valid in Postgres except randomblob).
-- Statements are separated by ";" at the end of a line: both migrators split on that.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT INTO settings (key, value) VALUES ('token_secret', lower(hex(randomblob(32))));
CREATE TABLE workspaces (
  id           TEXT PRIMARY KEY,
  auth_method  TEXT NOT NULL,
  owner_ref    TEXT,
  order_seq    INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL,
  last_seen_at TEXT NOT NULL
);
CREATE UNIQUE INDEX workspaces_owner ON workspaces (owner_ref);
CREATE TABLE customers (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  name         TEXT NOT NULL,
  city         TEXT,
  search       TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX customers_ws_created ON customers (workspace_id, created_at, id);
CREATE TABLE orders (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  number       INTEGER NOT NULL,
  customer_id  TEXT NOT NULL,
  title        TEXT NOT NULL,
  search       TEXT NOT NULL,
  status       TEXT NOT NULL,
  total        INTEGER NOT NULL,
  currency     TEXT NOT NULL,
  tags         TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  UNIQUE (workspace_id, number)
);
CREATE INDEX orders_ws_created ON orders (workspace_id, created_at, id);
CREATE INDEX orders_ws_customer ON orders (workspace_id, customer_id);
CREATE TABLE jobs (
  id                TEXT PRIMARY KEY,
  workspace_id      TEXT NOT NULL,
  order_id          TEXT NOT NULL,
  type              TEXT NOT NULL,
  status            TEXT NOT NULL,
  attempts          INTEGER NOT NULL DEFAULT 0,
  max_attempts      INTEGER NOT NULL,
  simulate_failures INTEGER NOT NULL DEFAULT 0,
  run_at            TEXT NOT NULL,
  locked_until      TEXT,
  last_error        TEXT,
  result            TEXT,
  processed_by      TEXT,
  history           TEXT NOT NULL,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);
CREATE INDEX jobs_queue ON jobs (status, run_at);
CREATE INDEX jobs_ws_created ON jobs (workspace_id, created_at, id);
CREATE TABLE idempotency_keys (
  workspace_id  TEXT NOT NULL,
  key           TEXT NOT NULL,
  route         TEXT NOT NULL,
  body_hash     TEXT NOT NULL,
  status        INTEGER NOT NULL,
  response      TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  PRIMARY KEY (workspace_id, key)
);
