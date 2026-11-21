"""Config, errors, time/id helpers, SQLite access and the shared migrator (same rules as go/db.go)."""
from __future__ import annotations

import glob
import os
import re
import secrets
import sqlite3
from contextlib import contextmanager
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Iterator


def _env(k: str, default: str) -> str:
    return os.environ.get(k) or default


def _env_int(k: str, default: int) -> int:
    try:
        return int(os.environ.get(k, ""))
    except ValueError:
        return default


@dataclass
class Config:
    db_path: str = field(default_factory=lambda: _env("DB_PATH", "./data/api.db"))
    edition: str = field(default_factory=lambda: "ru" if os.environ.get("EDITION") == "ru" else "intl")
    contract_dir: str = field(default_factory=lambda: _env("CONTRACT_DIR", "../contract"))
    migrations_dir: str = field(default_factory=lambda: _env("MIGRATIONS_DIR", "../migrations"))
    token_secret: str = field(default_factory=lambda: os.environ.get("API_TOKEN_SECRET", ""))
    sandbox_per_hour: int = field(default_factory=lambda: _env_int("SANDBOX_PER_HOUR", 20))
    rate_per_min: int = field(default_factory=lambda: _env_int("RATE_PER_MIN", 600))
    worker: bool = field(default_factory=lambda: os.environ.get("WORKER", "1") != "0")


# ---- errors ----

class ApiError(Exception):
    def __init__(self, status: int, code: str, message: str, details: list[dict] | None = None, retry_after: int = 0):
        super().__init__(f"{code}: {message}")
        self.status, self.code, self.message, self.details, self.retry_after = status, code, message, details or [], retry_after


def validation_error(details: list[tuple[str, str]]) -> ApiError:
    return ApiError(422, "validation_error", "request validation failed", [{"field": f, "issue": i} for f, i in details])


def not_found(what: str) -> ApiError:
    return ApiError(404, "not_found", f"{what} not found")


# ---- time and ids ----

def utcnow() -> datetime:
    return datetime.now(timezone.utc)


def ts(dt: datetime) -> str:
    dt = dt.astimezone(timezone.utc)
    return dt.strftime("%Y-%m-%dT%H:%M:%S.") + f"{dt.microsecond // 1000:03d}Z"


_ALPHABET = "0123456789abcdefghijklmnopqrstuvwxyz"


def new_id(prefix: str) -> str:
    return prefix + "".join(_ALPHABET[b % len(_ALPHABET)] for b in secrets.token_bytes(16))


# ---- database ----

def connect(path: str) -> sqlite3.Connection:
    """Autocommit connection; transactions are explicit BEGIN IMMEDIATE (see tx)."""
    os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
    conn = sqlite3.connect(path, timeout=10, isolation_level=None, check_same_thread=False)
    conn.execute("PRAGMA busy_timeout = 10000")
    conn.execute("PRAGMA journal_mode = WAL")
    conn.execute("PRAGMA synchronous = NORMAL")
    return conn


@contextmanager
def tx(conn: sqlite3.Connection) -> Iterator[sqlite3.Connection]:
    conn.execute("BEGIN IMMEDIATE")
    try:
        yield conn
    except BaseException:
        conn.execute("ROLLBACK")
        raise
    else:
        conn.execute("COMMIT")


_STMT_SPLIT = re.compile(r";[ \t]*(?:\r?\n|$)")


def split_sql(src: str) -> list[str]:
    out = []
    for chunk in _STMT_SPLIT.split(src):
        lines = [ln for ln in chunk.split("\n") if ln.strip() and not ln.strip().startswith("--")]
        s = "\n".join(lines).strip()
        if s:
            out.append(s)
    return out


def migrate(conn: sqlite3.Connection, directory: str) -> None:
    conn.execute("CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)")
    files = sorted(glob.glob(os.path.join(directory, "*.sql")))
    if not files:
        raise RuntimeError(f"no migrations in {directory}")
    for f in files:
        version = os.path.basename(f)[: -len(".sql")]
        with open(f, encoding="utf-8") as fh:
            src = fh.read()
        with tx(conn):
            if conn.execute("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", (version,)).fetchone()[0]:
                continue
            for st in split_sql(src):
                conn.execute(st)
            conn.execute("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", (version, ts(utcnow())))


def one(conn: sqlite3.Connection, sql: str, args: tuple | list = ()) -> Any:
    row = conn.execute(sql, args).fetchone()
    return None if row is None else row
