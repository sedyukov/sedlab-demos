"""Handlers: the same behaviour as the Go backend (go/auth.go, go/resources.go). Plain sync code run in a threadpool."""
from __future__ import annotations

import base64
import hashlib
import hmac
import json
import math
import os
import re
import sqlite3
import threading
import time
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Callable

from .core import ApiError, Config, connect, new_id, not_found, one, ts, tx, utcnow, validation_error
from .validate import MAX_INT64, Query, Validator, parse_object

TOKEN_ISSUER = "sedlab-api"
TOKEN_TTL = 24 * 3600
QUOTAS = {"customers": 100, "orders": 500, "jobs": 300}
ORDER_STATUSES = ("new", "confirmed", "in_progress", "done", "cancelled")
TRANSITIONS = {"new": ("confirmed", "cancelled"), "confirmed": ("in_progress", "cancelled"), "in_progress": ("done", "cancelled")}
JOB_TYPES = ("send_invoice", "sync_crm", "notify_customer")
JOB_STATUSES = ("queued", "running", "retrying", "succeeded", "dead")
MAX_ATTEMPTS = 4


@dataclass
class Principal:
    ws: str
    am: str
    created_at: str
    exp: int


@dataclass
class Req:
    headers: dict[str, str]          # lower-case names
    cookies: dict[str, str]
    query: dict[str, list[str]]
    params: dict[str, str]
    body: bytes
    client_ip: str
    path: str
    p: Principal | None = None


@dataclass
class Result:
    status: int
    value: Any = None
    raw: bytes | None = None          # pre-serialized (idempotent replay)
    headers: dict[str, str] | None = None


def dumps(v: Any) -> bytes:
    return json.dumps(v, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


# ---- base64url / JWT ----

_B64 = re.compile(r"[A-Za-z0-9_-]*")


def b64e(b: bytes) -> str:
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode("ascii")


def b64d(s: str) -> bytes:
    if not _B64.fullmatch(s) or len(s) % 4 == 1:
        raise ValueError("base64")
    return base64.urlsafe_b64decode(s + "=" * (-len(s) % 4))


class Expired(Exception):
    pass


# ---- rate limiters (per process) ----

class BucketLimiter:
    def __init__(self, per_min: int):
        self.per_min = float(per_min)
        self.buckets: dict[str, list[float]] = {}
        self.lock = threading.Lock()

    def take(self, key: str) -> int:
        with self.lock:
            now = time.monotonic()
            if key not in self.buckets and len(self.buckets) > 20000:
                self.buckets = {}
            b = self.buckets.setdefault(key, [self.per_min, now])
            rate = self.per_min / 60
            b[0] = min(self.per_min, b[0] + (now - b[1]) * rate)
            b[1] = now
            if b[0] >= 1:
                b[0] -= 1
                return 0
            return math.ceil((1 - b[0]) / rate)


class WindowLimiter:
    def __init__(self, limit: int, window: float):
        self.limit, self.window = limit, window
        self.hits: dict[str, list[float]] = {}
        self.lock = threading.Lock()

    def take(self, key: str) -> int:
        with self.lock:
            now = time.monotonic()
            if len(self.hits) > 20000:
                self.hits = {}
            h = [t for t in self.hits.get(key, []) if now - t < self.window]
            if len(h) >= self.limit:
                self.hits[key] = h
                return math.ceil(self.window - (now - h[0]))
            h.append(now)
            self.hits[key] = h
            return 0


# ---- service ----

def encode_tags(t: list[str]) -> str:
    return "|" + "|".join(t) + "|" if t else ""


def decode_tags(s: str) -> list[str]:
    return s.strip("|").split("|") if s else []


def customer_search(name: str, city: str | None) -> str:
    return (name + (" " + city if city is not None else "")).lower()


def encode_cursor(sort: str, value: str, id_: str) -> str:
    return b64e(f"{sort}|{value}|{id_}".encode("utf-8"))


_INT = re.compile(r"-?[0-9]+")


def decode_cursor(c: str, sort: str, numeric: bool) -> tuple[str, str] | None:
    try:
        s = b64d(c).decode("utf-8")
    except (ValueError, UnicodeDecodeError):
        return None
    if not s.startswith(sort + "|"):
        return None
    rest = s[len(sort) + 1:]
    i = rest.rfind("|")
    if i < 0 or i == len(rest) - 1:
        return None
    value, id_ = rest[:i], rest[i + 1:]
    if numeric and not _INT.fullmatch(value):
        return None
    return value, id_


@dataclass
class Page:
    sort: str
    col: str
    desc: bool
    numeric: bool
    limit: int
    after: tuple[str, str] | None

    def keyset(self, where: list[str], args: list) -> None:
        if self.after is None:
            return
        op = "<" if self.desc else ">"
        v: Any = int(self.after[0]) if self.numeric else self.after[0]
        where.append(f"({self.col} {op} ? OR ({self.col} = ? AND id {op} ?))")
        args.extend([v, v, self.after[1]])

    def order(self) -> str:
        d = "DESC" if self.desc else "ASC"
        return f" ORDER BY {self.col} {d}, id {d} LIMIT {self.limit + 1}"


def parse_page(q: Query, sorts: tuple[str, ...] | None, default: str, numeric_cols: tuple[str, ...] = ()) -> Page:
    sort = q.enum("sort", default, *sorts) if sorts is not None else default
    limit = q.int_param("limit", 1, 100, 20)
    col = sort.lstrip("-")
    p = Page(sort=sort, col=col, desc=sort.startswith("-"), numeric=col in numeric_cols, limit=limit, after=None)
    c = q.str("cursor")
    if c is not None:
        dec = decode_cursor(c, p.sort, p.numeric)
        if dec is None:
            q.add("cursor", "invalid")
        else:
            p.after = dec
    return p


def list_response(items: list[dict], p: Page, total: int, key: Callable[[dict], tuple[str, str]]) -> dict:
    nxt = None
    if len(items) > p.limit:
        items = items[: p.limit]
        nxt = encode_cursor(p.sort, *key(items[-1]))
    return {"items": items, "next_cursor": nxt, "total_count": total}


CUSTOMER_COLS = "id, name, city, created_at, updated_at"
ORDER_COLS = "id, number, customer_id, title, status, total, currency, tags, due_date, created_at, updated_at"
JOB_COLS = ("id, order_id, type, status, attempts, max_attempts, simulate_failures, run_at, last_error, result, "
            "processed_by, history, created_at, updated_at")


def row_customer(r: tuple) -> dict:
    return {"id": r[0], "name": r[1], "city": r[2], "created_at": r[3], "updated_at": r[4]}


def row_order(r: tuple) -> dict:
    return {"id": r[0], "number": r[1], "customer_id": r[2], "title": r[3], "status": r[4], "total": r[5],
            "currency": r[6], "tags": decode_tags(r[7]), "due_date": r[8], "created_at": r[9], "updated_at": r[10]}


def row_job(r: tuple) -> dict:
    try:
        history = json.loads(r[11])
    except ValueError:
        history = []
    return {"id": r[0], "order_id": r[1], "type": r[2], "status": r[3], "attempts": r[4], "max_attempts": r[5],
            "simulate_failures": r[6], "next_run_at": r[7] if r[3] in ("queued", "retrying") else None,
            "last_error": r[8], "result": json.loads(r[9]) if r[9] is not None else None, "processed_by": r[10],
            "history": history, "created_at": r[12], "updated_at": r[13]}


class Service:
    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.local = threading.local()
        conn = self.conn()
        if cfg.token_secret:
            self.secret = cfg.token_secret.encode()
        else:
            self.secret = one(conn, "SELECT value FROM settings WHERE key = 'token_secret'")[0].encode()
        self.rate = BucketLimiter(cfg.rate_per_min)
        self.sandbox = WindowLimiter(cfg.sandbox_per_hour, 3600)
        self.touched: dict[str, float] = {}
        self.touch_lock = threading.Lock()
        self.acc = None
        if cfg.edition == "ru":
            from sedlab_accounts import from_env
            self.acc = from_env()
        with open(os.path.join(cfg.contract_dir, "seed.json"), encoding="utf-8") as f:
            self.seed = json.load(f)

    def conn(self) -> sqlite3.Connection:
        c = getattr(self.local, "conn", None)
        if c is None:
            c = self.local.conn = connect(self.cfg.db_path)
        return c

    # ---- tokens ----

    def issue_token(self, ws: str, am: str, now: int) -> tuple[str, int]:
        exp = now + TOKEN_TTL
        signing = b64e(b'{"alg":"HS256","typ":"JWT"}') + "." + b64e(dumps({"iss": TOKEN_ISSUER, "sub": ws, "am": am, "iat": now, "exp": exp}))
        sig = hmac.new(self.secret, signing.encode(), hashlib.sha256).digest()
        return signing + "." + b64e(sig), exp

    def verify_token(self, tok: str, now: float) -> dict:
        parts = tok.split(".")
        if len(parts) != 3:
            raise ValueError("malformed")
        head = json.loads(b64d(parts[0]))
        if not isinstance(head, dict) or head.get("alg") != "HS256":
            raise ValueError("alg")
        sig = b64d(parts[2])
        want = hmac.new(self.secret, (parts[0] + "." + parts[1]).encode(), hashlib.sha256).digest()
        if not hmac.compare_digest(sig, want):
            raise ValueError("signature")
        c = json.loads(b64d(parts[1]))
        if not isinstance(c, dict) or c.get("iss") != TOKEN_ISSUER or not isinstance(c.get("sub"), str) or not c["sub"]:
            raise ValueError("claims")
        exp = c.get("exp", 0)
        if isinstance(exp, bool) or not isinstance(exp, (int, float)) or not isinstance(c.get("am", ""), str):
            raise ValueError("claims")
        if now >= exp:
            raise Expired()
        return c

    def authenticate(self, q: Req) -> Principal:
        h = q.headers.get("authorization", "")
        if len(h) < 7 or h[:7].lower() != "bearer " or not h[7:].strip():
            raise ApiError(401, "unauthorized", "missing bearer token")
        now = time.time()
        try:
            c = self.verify_token(h[7:].strip(), int(now))
        except Expired:
            raise ApiError(401, "unauthorized", "token expired")
        except (ValueError, TypeError, UnicodeDecodeError):
            raise ApiError(401, "unauthorized", "invalid token")
        row = one(self.conn(), "SELECT auth_method, created_at FROM workspaces WHERE id = ?", (c["sub"],))
        if row is None:
            raise ApiError(401, "unauthorized", "workspace expired")
        self.touch(c["sub"], now)
        return Principal(ws=c["sub"], am=row[0], created_at=row[1], exp=int(c["exp"]))

    def touch(self, ws: str, now: float) -> None:
        with self.touch_lock:
            last = self.touched.get(ws)
            if last is not None and now - last < 60:
                return
            self.touched[ws] = now
            if len(self.touched) > 10000:
                self.touched = {ws: now}
        self.conn().execute("UPDATE workspaces SET last_seen_at = ? WHERE id = ?", (ts(utcnow()), ws))

    def limit_ip(self, q: Req) -> None:
        wait = self.sandbox.take(q.client_ip)
        if wait > 0:
            raise ApiError(429, "rate_limited", "too many requests", retry_after=wait)

    def limit_ws(self, ws: str) -> None:
        wait = self.rate.take(ws)
        if wait > 0:
            raise ApiError(429, "rate_limited", "too many requests", retry_after=wait)

    def token_response(self, ws: str, am: str, created: str, now: datetime) -> dict:
        tok, _ = self.issue_token(ws, am, int(now.timestamp()))
        return {"access_token": tok, "token_type": "Bearer", "expires_in": TOKEN_TTL,
                "workspace": {"id": ws, "auth_method": am, "created_at": created}}

    # ---- system / auth ----

    def health(self, q: Req) -> Result:
        self.conn().execute("SELECT 1").fetchone()
        return Result(200, {"status": "ok", "backend": "python", "edition": self.cfg.edition, "db": "ok"})

    def create_sandbox(self, q: Req) -> Result:
        self.limit_ip(q)
        now = utcnow()
        ws = new_id("ws_")
        self.create_workspace(ws, "sandbox", None, now)
        return Result(201, self.token_response(ws, "sandbox", ts(now), now))

    def create_workspace(self, ws: str, am: str, owner: str | None, now: datetime) -> None:
        with tx(self.conn()) as c:
            c.execute("INSERT INTO workspaces (id, auth_method, owner_ref, order_seq, created_at, last_seen_at) VALUES (?, ?, ?, 0, ?, ?)",
                      (ws, am, owner, ts(now), ts(now)))
            self.seed_workspace(c, ws, now)

    def seed_workspace(self, c: sqlite3.Connection, ws: str, now: datetime) -> None:
        ed = self.cfg.edition
        cur = "RUB" if ed == "ru" else "USD"
        ids = []
        for i, cu in enumerate(self.seed["customers"]):
            cid = new_id("cus_")
            ids.append(cid)
            created = ts(now - timedelta(hours=30 * 24 - i * 24))
            name, city = cu["name"][ed], cu["city"][ed]
            c.execute("INSERT INTO customers (id, workspace_id, name, city, search, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                      (cid, ws, name, city, customer_search(name, city), created, created))
        orders = self.seed["orders"]
        for i, o in enumerate(orders):
            created = ts(now - timedelta(hours=o["age_hours"]))
            due = None
            if o.get("due_in_days") is not None:
                due = (now + timedelta(days=o["due_in_days"])).date().isoformat()
            self.insert_order(c, ws, {"id": new_id("ord_"), "number": i + 1, "customer_id": ids[o["customer"]],
                                      "title": o["title"][ed], "status": o["status"], "total": o["total"][ed],
                                      "currency": cur, "tags": o["tags"], "due_date": due,
                                      "created_at": created, "updated_at": created})
        c.execute("UPDATE workspaces SET order_seq = ? WHERE id = ?", (len(orders), ws))

    def exchange_accounts(self, q: Req) -> Result:
        if self.acc is None:
            raise ApiError(404, "feature_disabled", "sign-in via accounts is not available in this edition")
        self.limit_ip(q)
        tok = ""
        if q.body.strip():
            v = Validator(parse_object(q.body), False)
            v.str("accounts_token", False, False, 1, 4096)
            v.unknown()
            if v.details:
                raise validation_error(v.details)
            tok = v.out.get("accounts_token") or ""
        if not tok:
            tok = q.cookies.get("sl_auth", "")
        if not tok:
            raise ApiError(401, "unauthorized", "missing accounts session")
        try:
            ident = self.acc.verify_token(tok)
        except Exception:
            raise ApiError(401, "unauthorized", "invalid accounts session")
        owner = "acc:" + ident.user_id
        now = utcnow()
        row = one(self.conn(), "SELECT id, created_at FROM workspaces WHERE owner_ref = ?", (owner,))
        if row is None:
            ws, created = new_id("ws_"), ts(now)
            try:
                self.create_workspace(ws, "accounts", owner, now)
            except sqlite3.IntegrityError:  # created concurrently (maybe by the Go backend)
                ws, created = one(self.conn(), "SELECT id, created_at FROM workspaces WHERE owner_ref = ?", (owner,))
        else:
            ws, created = row

        def visit() -> None:
            try:
                self.acc.visit(tok)
            except Exception:
                pass
        threading.Thread(target=visit, daemon=True).start()
        return Result(200, self.token_response(ws, "accounts", created, now))

    def counts(self, ws: str) -> dict:
        c = self.conn()
        return {t: one(c, f"SELECT COUNT(*) FROM {t} WHERE workspace_id = ?", (ws,))[0] for t in ("customers", "orders", "jobs")}

    def me(self, q: Req) -> Result:
        p = q.p
        return Result(200, {
            "workspace": {"id": p.ws, "auth_method": p.am, "created_at": p.created_at},
            "backend": "python", "edition": self.cfg.edition,
            "token_expires_at": ts(datetime.fromtimestamp(p.exp, timezone.utc)),
            "quotas": dict(QUOTAS), "usage": self.counts(p.ws),
        })

    # ---- idempotency ----

    def idempotent(self, q: Req, h: Callable[[Req], Result]) -> Result:
        key = q.headers.get("idempotency-key", "")
        if key == "":
            return h(q)
        if len(key) > 100:
            raise validation_error([("Idempotency-Key", "too_long")])
        body_hash = hashlib.sha256(q.body).hexdigest()
        route = "POST " + q.path
        row = one(self.conn(), "SELECT route, body_hash, status, response FROM idempotency_keys WHERE workspace_id = ? AND key = ?",
                  (q.p.ws, key))
        if row is not None:
            if row[0] != route or row[1] != body_hash:
                raise ApiError(409, "idempotency_conflict", "idempotency key reused with a different request")
            return Result(row[2], raw=row[3].encode("utf-8"), headers={"Idempotent-Replayed": "true"})
        res = h(q)
        raw = dumps(res.value)
        self.conn().execute(
            "INSERT INTO idempotency_keys (workspace_id, key, route, body_hash, status, response, created_at) "
            "VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (workspace_id, key) DO NOTHING",
            (q.p.ws, key, route, body_hash, res.status, raw.decode("utf-8"), ts(utcnow())))
        return Result(res.status, raw=raw)

    # ---- customers ----

    def check_quota(self, c: sqlite3.Connection, ws: str, table: str) -> None:
        if one(c, f"SELECT COUNT(*) FROM {table} WHERE workspace_id = ?", (ws,))[0] >= QUOTAS[table]:
            raise ApiError(409, "quota_exceeded", f"workspace quota exceeded: {table}")

    def list_customers(self, q: Req) -> Result:
        qv = Query(q.query)
        search = qv.search("q")
        p = parse_page(qv, ("name", "-name", "created_at", "-created_at"), "-created_at")
        if qv.details:
            raise validation_error(qv.details)
        where, args = ["workspace_id = ?"], [q.p.ws]
        if search:
            where.append("instr(search, ?) > 0")
            args.append(search)
        c = self.conn()
        total = one(c, "SELECT COUNT(*) FROM customers WHERE " + " AND ".join(where), args)[0]
        p.keyset(where, args)
        items = [row_customer(r) for r in c.execute(f"SELECT {CUSTOMER_COLS} FROM customers WHERE " + " AND ".join(where) + p.order(), args)]
        return Result(200, list_response(items, p, total, lambda x: (x["name"] if p.col == "name" else x["created_at"], x["id"])))

    def customer_row(self, c: sqlite3.Connection, ws: str, id_: str) -> dict:
        r = one(c, f"SELECT {CUSTOMER_COLS} FROM customers WHERE workspace_id = ? AND id = ?", (ws, id_))
        if r is None:
            raise not_found("customer")
        return row_customer(r)

    def get_customer(self, q: Req) -> Result:
        return Result(200, self.customer_row(self.conn(), q.p.ws, q.params["id"]))

    @staticmethod
    def customer_fields(v: Validator) -> None:
        v.str("name", True, False, 1, 100)
        v.str("city", False, True, 0, 60)
        v.unknown()

    def create_customer(self, q: Req) -> Result:
        v = Validator(parse_object(q.body), False)
        self.customer_fields(v)
        if v.details:
            raise validation_error(v.details)
        with tx(self.conn()) as c:
            self.check_quota(c, q.p.ws, "customers")
            now = ts(utcnow())
            cu = {"id": new_id("cus_"), "name": v.out["name"], "city": v.out.get("city"), "created_at": now, "updated_at": now}
            c.execute("INSERT INTO customers (id, workspace_id, name, city, search, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                      (cu["id"], q.p.ws, cu["name"], cu["city"], customer_search(cu["name"], cu["city"]), now, now))
        return Result(201, cu)

    def update_customer(self, q: Req) -> Result:
        obj = parse_object(q.body)
        if not obj:
            raise validation_error([("body", "empty")])
        v = Validator(obj, True)
        self.customer_fields(v)
        if v.details:
            raise validation_error(v.details)
        with tx(self.conn()) as c:
            cu = self.customer_row(c, q.p.ws, q.params["id"])
            if "name" in v.out:
                cu["name"] = v.out["name"]
            if "city" in v.out:
                cu["city"] = v.out["city"]
            cu["updated_at"] = ts(utcnow())
            c.execute("UPDATE customers SET name = ?, city = ?, search = ?, updated_at = ? WHERE id = ?",
                      (cu["name"], cu["city"], customer_search(cu["name"], cu["city"]), cu["updated_at"], cu["id"]))
        return Result(200, cu)

    def delete_customer(self, q: Req) -> Result:
        with tx(self.conn()) as c:
            cu = self.customer_row(c, q.p.ws, q.params["id"])
            if one(c, "SELECT COUNT(*) FROM orders WHERE workspace_id = ? AND customer_id = ?", (q.p.ws, cu["id"]))[0] > 0:
                raise ApiError(409, "conflict", "customer has orders")
            c.execute("DELETE FROM customers WHERE id = ?", (cu["id"],))
        return Result(204)

    # ---- orders ----

    @staticmethod
    def insert_order(c: sqlite3.Connection, ws: str, o: dict) -> None:
        c.execute("INSERT INTO orders (id, workspace_id, number, customer_id, title, search, status, total, currency, tags, due_date, "
                  "created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                  (o["id"], ws, o["number"], o["customer_id"], o["title"], o["title"].lower(), o["status"], o["total"],
                   o["currency"], encode_tags(o["tags"]), o["due_date"], o["created_at"], o["updated_at"]))

    def list_orders(self, q: Req) -> Result:
        qv = Query(q.query)
        where, args = ["workspace_id = ?"], [q.p.ws]
        s = qv.str("status")
        if s is not None:
            parts = s.split(",")
            if any(x not in ORDER_STATUSES for x in parts):
                qv.add("status", "invalid")
            else:
                where.append("status IN (" + ", ".join("?" * len(parts)) + ")")
                args.extend(parts)
        s = qv.str("customer_id")
        if s is not None:
            where.append("customer_id = ?")
            args.append(s)
        s = qv.str("tag")
        if s is not None:
            where.append("instr(tags, ?) > 0")
            args.append("|" + s + "|")
        s = qv.search("q")
        if s:
            where.append("instr(search, ?) > 0")
            args.append(s)
        d = qv.date("created_from")
        if d:
            where.append("created_at >= ?")
            args.append(d + "T00:00:00.000Z")
        d = qv.date("created_to")
        if d:
            where.append("created_at <= ?")
            args.append(d + "T23:59:59.999Z")
        n = qv.int_param("min_total", 0, MAX_INT64, -1)
        if n >= 0:
            where.append("total >= ?")
            args.append(n)
        n = qv.int_param("max_total", 0, MAX_INT64, -1)
        if n >= 0:
            where.append("total <= ?")
            args.append(n)
        p = parse_page(qv, ("created_at", "-created_at", "total", "-total", "number", "-number"), "-created_at", ("total", "number"))
        if qv.details:
            raise validation_error(qv.details)
        c = self.conn()
        total = one(c, "SELECT COUNT(*) FROM orders WHERE " + " AND ".join(where), args)[0]
        p.keyset(where, args)
        items = [row_order(r) for r in c.execute(f"SELECT {ORDER_COLS} FROM orders WHERE " + " AND ".join(where) + p.order(), args)]

        def key(o: dict) -> tuple[str, str]:
            if p.col in ("total", "number"):
                return str(o[p.col]), o["id"]
            return o["created_at"], o["id"]
        return Result(200, list_response(items, p, total, key))

    def order_row(self, c: sqlite3.Connection, ws: str, id_: str) -> dict:
        r = one(c, f"SELECT {ORDER_COLS} FROM orders WHERE workspace_id = ? AND id = ?", (ws, id_))
        if r is None:
            raise not_found("order")
        return row_order(r)

    def get_order(self, q: Req) -> Result:
        return Result(200, self.order_row(self.conn(), q.p.ws, q.params["id"]))

    @staticmethod
    def order_fields(v: Validator) -> None:
        v.str("customer_id", True, False, 1, 64)
        v.str("title", True, False, 1, 120)
        if v.patch:
            v.enum("status", False, *ORDER_STATUSES)
        v.integer("total", True, 0, 100_000_000)
        v.enum("currency", False, "RUB", "USD", "EUR")
        v.tags("tags")
        v.date("due_date", True)
        v.unknown()

    @staticmethod
    def customer_exists(c: sqlite3.Connection, ws: str, id_: str) -> bool:
        return one(c, "SELECT COUNT(*) FROM customers WHERE workspace_id = ? AND id = ?", (ws, id_))[0] > 0

    def create_order(self, q: Req) -> Result:
        v = Validator(parse_object(q.body), False)
        self.order_fields(v)
        if v.details:
            raise validation_error(v.details)
        with tx(self.conn()) as c:
            if not self.customer_exists(c, q.p.ws, v.out["customer_id"]):
                raise validation_error([("customer_id", "not_found")])
            self.check_quota(c, q.p.ws, "orders")
            number = one(c, "UPDATE workspaces SET order_seq = order_seq + 1 WHERE id = ? RETURNING order_seq", (q.p.ws,))[0]
            now = ts(utcnow())
            o = {"id": new_id("ord_"), "number": number, "customer_id": v.out["customer_id"], "title": v.out["title"],
                 "status": "new", "total": v.out["total"], "currency": v.out.get("currency", "RUB" if self.cfg.edition == "ru" else "USD"),
                 "tags": v.out.get("tags", []), "due_date": v.out.get("due_date"), "created_at": now, "updated_at": now}
            self.insert_order(c, q.p.ws, o)
        return Result(201, o)

    def update_order(self, q: Req) -> Result:
        obj = parse_object(q.body)
        if not obj:
            raise validation_error([("body", "empty")])
        v = Validator(obj, True)
        self.order_fields(v)
        if v.details:
            raise validation_error(v.details)
        with tx(self.conn()) as c:
            o = self.order_row(c, q.p.ws, q.params["id"])
            if "customer_id" in v.out:
                if not self.customer_exists(c, q.p.ws, v.out["customer_id"]):
                    raise validation_error([("customer_id", "not_found")])
                o["customer_id"] = v.out["customer_id"]
            st = v.out.get("status")
            if st is not None and st != o["status"]:
                if st not in TRANSITIONS.get(o["status"], ()):
                    raise ApiError(409, "invalid_transition", f"cannot change status from {o['status']} to {st}",
                                   [{"field": "status", "issue": "invalid"}])
                o["status"] = st
            for k in ("title", "total", "currency", "tags", "due_date"):
                if k in v.out:
                    o[k] = v.out[k]
            o["updated_at"] = ts(utcnow())
            c.execute("UPDATE orders SET customer_id = ?, title = ?, search = ?, status = ?, total = ?, currency = ?, tags = ?, "
                      "due_date = ?, updated_at = ? WHERE id = ?",
                      (o["customer_id"], o["title"], o["title"].lower(), o["status"], o["total"], o["currency"],
                       encode_tags(o["tags"]), o["due_date"], o["updated_at"], o["id"]))
        return Result(200, o)

    def delete_order(self, q: Req) -> Result:
        with tx(self.conn()) as c:
            o = self.order_row(c, q.p.ws, q.params["id"])
            c.execute("DELETE FROM jobs WHERE workspace_id = ? AND order_id = ?", (q.p.ws, o["id"]))
            c.execute("DELETE FROM orders WHERE id = ?", (o["id"],))
        return Result(204)

    # ---- jobs ----

    def enqueue_job(self, q: Req) -> Result:
        v = Validator(parse_object(q.body), False)
        v.enum("type", True, *JOB_TYPES)
        v.integer("simulate_failures", False, 0, 5)
        v.unknown()
        if v.details:
            raise validation_error(v.details)
        with tx(self.conn()) as c:
            o = self.order_row(c, q.p.ws, q.params["id"])
            self.check_quota(c, q.p.ws, "jobs")
            now = ts(utcnow())
            jid = new_id("job_")
            c.execute("INSERT INTO jobs (id, workspace_id, order_id, type, status, attempts, max_attempts, simulate_failures, run_at, "
                      "history, created_at, updated_at) VALUES (?, ?, ?, ?, 'queued', 0, ?, ?, ?, '[]', ?, ?)",
                      (jid, q.p.ws, o["id"], v.out["type"], MAX_ATTEMPTS, v.out.get("simulate_failures", 0), now, now, now))
            job = row_job(one(c, f"SELECT {JOB_COLS} FROM jobs WHERE id = ?", (jid,)))
        return Result(202, job)

    def list_jobs(self, q: Req) -> Result:
        qv = Query(q.query)
        where, args = ["workspace_id = ?"], [q.p.ws]
        s = qv.str("order_id")
        if s is not None:
            where.append("order_id = ?")
            args.append(s)
        if qv.has("status"):
            st = qv.enum("status", "", *JOB_STATUSES)
            if st:
                where.append("status = ?")
                args.append(st)
        p = parse_page(qv, None, "-created_at")
        if qv.details:
            raise validation_error(qv.details)
        c = self.conn()
        total = one(c, "SELECT COUNT(*) FROM jobs WHERE " + " AND ".join(where), args)[0]
        p.keyset(where, args)
        items = [row_job(r) for r in c.execute(f"SELECT {JOB_COLS} FROM jobs WHERE " + " AND ".join(where) + p.order(), args)]
        return Result(200, list_response(items, p, total, lambda j: (j["created_at"], j["id"])))

    def get_job(self, q: Req) -> Result:
        r = one(self.conn(), f"SELECT {JOB_COLS} FROM jobs WHERE workspace_id = ? AND id = ?", (q.p.ws, q.params["id"]))
        if r is None:
            raise not_found("job")
        return Result(200, row_job(r))
