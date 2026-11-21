-- Portable SQL (SQLite + Postgres): TEXT/BIGINT/INTEGER only, epoch seconds UTC, no AUTOINCREMENT/rowid/JSON functions.
CREATE TABLE users (
  id              TEXT PRIMARY KEY,
  display_name    TEXT NOT NULL DEFAULT '',
  username        TEXT NOT NULL DEFAULT '',
  lang            TEXT NOT NULL DEFAULT '',
  consent_version TEXT NOT NULL,
  consent_at      BIGINT NOT NULL,
  created_at      BIGINT NOT NULL,
  last_seen_at    BIGINT NOT NULL
);
CREATE INDEX users_last_seen ON users (last_seen_at);

CREATE TABLE identities (
  provider     TEXT NOT NULL,
  provider_uid TEXT NOT NULL,
  user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at   BIGINT NOT NULL,
  PRIMARY KEY (provider, provider_uid)
);
CREATE INDEX identities_user ON identities (user_id);

CREATE TABLE sessions (
  sid        TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  provider   TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  expires_at BIGINT NOT NULL,
  revoked_at BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX sessions_user ON sessions (user_id);

-- Bot deep-link logins. status: pending | confirmed | used. Identity fields filled on confirm (until consent: no users row).
CREATE TABLE login_nonces (
  nonce        TEXT PRIMARY KEY,
  provider     TEXT NOT NULL,
  browser_hash TEXT NOT NULL,
  status       TEXT NOT NULL,
  provider_uid TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  username     TEXT NOT NULL DEFAULT '',
  expires_at   BIGINT NOT NULL
);

CREATE TABLE demo_visits (
  user_id  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  demo     TEXT NOT NULL,
  first_at BIGINT NOT NULL,
  last_at  BIGINT NOT NULL,
  visits   INTEGER NOT NULL,
  PRIMARY KEY (user_id, demo)
);

-- Per-user daily usage. day = YYYY-MM-DD (UTC).
CREATE TABLE usage_daily (
  user_id  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  demo     TEXT NOT NULL,
  resource TEXT NOT NULL,
  day      TEXT NOT NULL,
  used     BIGINT NOT NULL,
  PRIMARY KEY (user_id, demo, resource, day)
);

-- Global counters, NOT tied to users (survive account deletion). period = YYYY-MM-DD (daily) or YYYY-MM (monthly).
CREATE TABLE usage_global (
  resource TEXT NOT NULL,
  period   TEXT NOT NULL,
  used     BIGINT NOT NULL,
  PRIMARY KEY (resource, period)
);

CREATE TABLE consumptions (
  id              TEXT PRIMARY KEY,
  user_id         TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  demo            TEXT NOT NULL,
  resource        TEXT NOT NULL,
  units           BIGINT NOT NULL,
  idempotency_key TEXT NOT NULL,
  status          TEXT NOT NULL,
  day             TEXT NOT NULL,
  created_at      BIGINT NOT NULL,
  UNIQUE (demo, idempotency_key)
);

CREATE TABLE leads (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  provider    TEXT NOT NULL,
  first_demo  TEXT NOT NULL DEFAULT '',
  created_at  BIGINT NOT NULL,
  notified_at BIGINT NOT NULL DEFAULT 0
);
