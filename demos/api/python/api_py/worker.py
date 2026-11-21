"""Background worker and housekeeping: the same queue protocol as go/worker.go, on the shared SQLite table."""
from __future__ import annotations

import json
import logging
import threading
from datetime import datetime, timedelta

from .core import Config, connect, one, ts, tx, utcnow

log = logging.getLogger("api_py.worker")

SIMULATED_ERROR = "upstream returned 503 (simulated)"
LOCK_FOR = timedelta(seconds=30)
WORK_SECONDS = 0.4


def job_result(typ: str, number: int, total: int, currency: str, status: str) -> dict:
    if typ == "send_invoice":
        return {"invoice_number": f"INV-{number:05d}", "amount": total, "currency": currency}
    if typ == "sync_crm":
        return {"crm_deal_id": f"deal-{number}", "stage": status}
    return {"channel": "email", "template": f"order_{status}", "delivered": True}


class Worker:
    def __init__(self, cfg: Config, stop: threading.Event):
        self.cfg, self.stop = cfg, stop
        self.conn = connect(cfg.db_path)

    def claim(self, now: datetime) -> str | None:
        with tx(self.conn) as c:
            row = one(c, "SELECT id FROM jobs WHERE (status IN ('queued', 'retrying') AND run_at <= ?) "
                         "OR (status = 'running' AND locked_until < ?) ORDER BY run_at, id LIMIT 1", (ts(now), ts(now)))
            if row is None:
                return None
            c.execute("UPDATE jobs SET status = 'running', locked_until = ?, updated_at = ? WHERE id = ?",
                      (ts(now + LOCK_FOR), ts(now), row[0]))
            return row[0]

    def work_once(self) -> bool:
        jid = self.claim(utcnow())
        if jid is None:
            return False
        if self.stop.wait(WORK_SECONDS):  # the "work": an API call to an external system
            return True
        self.finish(jid, utcnow())
        return True

    def finish(self, jid: str, now: datetime) -> None:
        with tx(self.conn) as c:
            row = one(c, "SELECT workspace_id, order_id, type, attempts, max_attempts, simulate_failures, history "
                         "FROM jobs WHERE id = ? AND status = 'running'", (jid,))
            if row is None:
                return
            _ws, order_id, typ, attempts, max_att, sim, hist = row
            history = json.loads(hist)
            attempt = attempts + 1
            entry = {"attempt": attempt, "at": ts(now), "worker": "python", "outcome": "ok", "error": None}
            err = ""
            order = None
            if attempt <= sim:
                err = SIMULATED_ERROR
            else:
                order = one(c, "SELECT number, total, currency, status FROM orders WHERE id = ?", (order_id,))
                if order is None:
                    err = "order not found"
            if err:
                entry["outcome"], entry["error"] = "error", err
                history.append(entry)
                hb = json.dumps(history, ensure_ascii=False, separators=(",", ":"))
                if attempt >= max_att or err == "order not found":
                    c.execute("UPDATE jobs SET status = 'dead', attempts = ?, last_error = ?, history = ?, locked_until = NULL, "
                              "updated_at = ? WHERE id = ?", (attempt, err, hb, ts(now), jid))
                else:
                    nxt = now + timedelta(seconds=1 << (attempt - 1))
                    c.execute("UPDATE jobs SET status = 'retrying', attempts = ?, last_error = ?, history = ?, run_at = ?, "
                              "locked_until = NULL, updated_at = ? WHERE id = ?", (attempt, err, hb, ts(nxt), ts(now), jid))
            else:
                history.append(entry)
                hb = json.dumps(history, ensure_ascii=False, separators=(",", ":"))
                rb = json.dumps(job_result(typ, *order), ensure_ascii=False, separators=(",", ":"))
                c.execute("UPDATE jobs SET status = 'succeeded', attempts = ?, last_error = NULL, result = ?, processed_by = 'python', "
                          "history = ?, locked_until = NULL, updated_at = ? WHERE id = ?", (attempt, rb, hb, ts(now), jid))

    def run(self) -> None:
        while not self.stop.is_set():
            try:
                worked = self.work_once()
            except Exception:  # noqa: BLE001 — keep the loop alive, log and retry
                log.exception("worker")
                worked = False
            if not worked:
                self.stop.wait(0.25)


def housekeep(conn, now: datetime) -> None:
    conds = [
        f"(auth_method = 'sandbox' AND last_seen_at < '{ts(now - timedelta(hours=12))}')",
        f"(auth_method = 'accounts' AND last_seen_at < '{ts(now - timedelta(days=30))}')",
    ]
    today = now.strftime("%Y-%m-%d")
    with tx(conn) as c:
        if now.hour >= 3:
            row = one(c, "SELECT value FROM settings WHERE key = 'last_wipe'")
            if row is None or row[0] != today:
                conds.append(f"(auth_method = 'sandbox' AND created_at < '{today}T03:00:00.000Z')")
                c.execute("INSERT INTO settings (key, value) VALUES ('last_wipe', ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value",
                          (today,))
        sel = "SELECT id FROM workspaces WHERE " + " OR ".join(conds)
        for t in ("jobs", "orders", "customers", "idempotency_keys"):
            c.execute(f"DELETE FROM {t} WHERE workspace_id IN ({sel})")
        c.execute(f"DELETE FROM workspaces WHERE id IN ({sel})")
        c.execute("DELETE FROM idempotency_keys WHERE created_at < ?", (ts(now - timedelta(hours=24)),))


def run_housekeeping(cfg: Config, stop: threading.Event) -> None:
    conn = connect(cfg.db_path)
    while not stop.is_set():
        try:
            housekeep(conn, utcnow())
        except Exception:  # noqa: BLE001
            log.exception("housekeeping")
        stop.wait(600)
