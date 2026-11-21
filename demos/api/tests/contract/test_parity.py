"""Both backends at once: the same request must give the same status and the same JSON.

Needs API_BASE_URL and API_OTHER_BASE_URL pointing at the two implementations over the same database
(e.g. http://127.0.0.1:18080/go and http://127.0.0.1:18080/py). Writes alternate between the backends.
"""
from __future__ import annotations

import os
import uuid

import pytest

from conftest import Api, new_sandbox, wait_job


@pytest.fixture
def pair(base):
    other = os.environ.get("API_OTHER_BASE_URL")
    if not other:
        pytest.skip("API_OTHER_BASE_URL is not set")
    a = new_sandbox(base)
    b = Api(other, a.token)
    assert a.get("/health", token=None).json()["backend"] != b.get("/health", token=None).json()["backend"]
    return a, b


def norm(v):
    if isinstance(v, dict):
        return {k: norm(x) for k, x in v.items() if k != "backend"}
    if isinstance(v, list):
        return [norm(x) for x in v]
    return v


def same(a: Api, b: Api, method: str, template: str, **kw):
    ra, rb = a.call(method, template, **kw), b.call(method, template, **kw)
    assert ra.status_code == rb.status_code, (template, kw, ra.text, rb.text)
    ja = ra.json() if ra.content else None
    jb = rb.json() if rb.content else None
    assert norm(ja) == norm(jb), (template, kw, ja, jb)
    return ja


READS = [
    ("/v1/me", {}),
    ("/v1/customers", {}),
    ("/v1/customers", {"sort": "name", "limit": 2}),
    ("/v1/customers", {"q": "o", "sort": "-name"}),
    ("/v1/orders", {}),
    ("/v1/orders", {"status": "new,in_progress", "sort": "total", "limit": 3}),
    ("/v1/orders", {"tag": "b2b", "sort": "-number"}),
    ("/v1/orders", {"min_total": "5000", "max_total": "900000", "sort": "-total", "limit": 4}),
    ("/v1/orders", {"q": "КОФЕ"}),
    ("/v1/orders", {"q": "coffee"}),
    ("/v1/orders", {"created_from": "2000-01-01", "created_to": "2100-01-01", "sort": "created_at", "limit": 5}),
    ("/v1/jobs", {}),
    # errors
    ("/v1/orders", {"status": "x", "limit": "0", "sort": "?", "min_total": "-3", "cursor": "!"}),
    ("/v1/customers", {"q": "я" * 101}),
    ("/v1/jobs", {"status": "nope"}),
]


def test_reads_identical(pair):
    a, b = pair
    for path, params in READS:
        same(a, b, "GET", path, params=params)


def test_pagination_cursors_interchangeable(pair):
    a, b = pair
    for sort in ("total", "-created_at", "number"):
        page = same(a, b, "GET", "/v1/orders", params={"sort": sort, "limit": 5})
        while page["next_cursor"]:
            page = same(a, b, "GET", "/v1/orders", params={"sort": sort, "limit": 5, "cursor": page["next_cursor"]})


BAD_BODIES = [
    ("/v1/customers", {}),
    ("/v1/customers", {"name": "x" * 101, "city": 1, "b": 1, "a": 2}),
    ("/v1/orders", {"customer_id": 5, "title": "", "total": 1.5, "tags": ["A", 1], "due_date": "2026-02-29", "currency": "XXX"}),
    ("/v1/orders", {"customer_id": "cus_nope", "title": "t", "total": 1}),
    ("/v1/orders", [1, 2]),
]


def test_errors_identical(pair):
    a, b = pair
    for path, body in BAD_BODIES:
        same(a, b, "POST", path, json=body)
    same(a, b, "POST", "/v1/orders", content=b"{oops", headers={"Content-Type": "application/json"})
    same(a, b, "GET", "/v1/orders/{id}", path={"id": "ord_missing"})
    same(a, b, "PATCH", "/v1/orders/{id}", path={"id": "ord_missing"}, json={"title": "x"})
    same(a, b, "GET", "/v1/me", token="garbage")
    same(a, b, "GET", "/v1/me", token=None)


def test_write_on_one_read_on_other(pair):
    a, b = pair
    cus = b.post("/v1/customers", json={"name": "Parity LLC", "city": "Томск"}).json()
    assert same(a, b, "GET", "/v1/customers/{id}", path={"id": cus["id"]}) == cus
    o = a.post("/v1/orders", json={"customer_id": cus["id"], "title": "Parity", "total": 777, "tags": ["x"]}).json()
    o2 = b.post("/v1/orders", json={"customer_id": cus["id"], "title": "Parity 2", "total": 1}).json()
    assert (o["number"], o2["number"]) == (13, 14)
    b.patch("/v1/orders/{id}", path={"id": o["id"]}, json={"status": "confirmed"})
    assert same(a, b, "GET", "/v1/orders/{id}", path={"id": o["id"]})["status"] == "confirmed"
    same(a, b, "PATCH", "/v1/orders/{id}", path={"id": o["id"]}, json={"status": "new"})  # 409 on both
    same(a, b, "DELETE", "/v1/customers/{id}", path={"id": cus["id"]})                   # 409 on both
    # idempotency key stored by one backend is honoured by the other
    key = str(uuid.uuid4())
    body = {"type": "send_invoice"}
    j1 = a.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json=body, headers={"Idempotency-Key": key})
    j2 = b.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json=body, headers={"Idempotency-Key": key})
    assert j1.json() == j2.json() and j2.headers.get("idempotent-replayed") == "true"
    done = wait_job(a, j1.json()["id"])
    assert same(a, b, "GET", "/v1/jobs/{id}", path={"id": done["id"]})["status"] == "succeeded"
    same(a, b, "GET", "/v1/jobs", params={"order_id": o["id"]})


def test_jobs_spread_over_both_workers(pair):
    a, b = pair
    o = a.get("/v1/orders", params={"limit": 1}).json()["items"][0]
    ids = [(a if i % 2 else b).post("/v1/orders/{id}/jobs", path={"id": o["id"]},
                                    json={"type": "sync_crm", "simulate_failures": 1}).json()["id"] for i in range(6)]
    workers = set()
    for jid in ids:
        j = wait_job(a, jid)
        assert j["status"] == "succeeded"
        workers |= {h["worker"] for h in j["history"]}
    assert workers == {"go", "python"}, workers
