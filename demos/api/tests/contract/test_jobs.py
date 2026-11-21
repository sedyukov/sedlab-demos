from __future__ import annotations

import uuid

import pytest

from conftest import errors, wait_job


def an_order(api) -> dict:
    return api.get("/v1/orders", params={"limit": 1, "sort": "number"}).json()["items"][0]


def test_job_succeeds(api):
    o = an_order(api)
    r = api.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json={"type": "send_invoice"})
    assert r.status_code == 202
    j = r.json()
    assert j["status"] == "queued" and j["attempts"] == 0 and j["max_attempts"] == 4 and j["history"] == []
    assert j["next_run_at"] == j["created_at"] and j["result"] is None and j["processed_by"] is None
    done = wait_job(api, j["id"])
    assert done["status"] == "succeeded", done
    assert done["attempts"] == 1 and done["last_error"] is None and done["next_run_at"] is None
    assert done["result"] == {"invoice_number": f"INV-{o['number']:05d}", "amount": o["total"], "currency": o["currency"]}
    assert done["processed_by"] in ("go", "python")
    assert done["history"][0]["outcome"] == "ok" and done["history"][0]["worker"] == done["processed_by"]


@pytest.mark.parametrize("typ,expect", [
    ("sync_crm", lambda o: {"crm_deal_id": f"deal-{o['number']}", "stage": o["status"]}),
    ("notify_customer", lambda o: {"channel": "email", "template": f"order_{o['status']}", "delivered": True}),
])
def test_job_results(api, typ, expect):
    o = an_order(api)
    j = api.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json={"type": typ, "simulate_failures": 0}).json()
    assert wait_job(api, j["id"])["result"] == expect(o)


def test_job_retries_then_succeeds(api):
    o = an_order(api)
    j = api.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json={"type": "sync_crm", "simulate_failures": 2}).json()
    mid = wait_job(api, j["id"], statuses=("retrying", "succeeded", "dead"))
    assert mid["status"] == "retrying" and mid["last_error"] == "upstream returned 503 (simulated)"
    assert mid["next_run_at"] is not None and mid["next_run_at"] > mid["history"][-1]["at"]
    done = wait_job(api, j["id"])
    assert done["status"] == "succeeded" and done["attempts"] == 3 and done["last_error"] is None
    assert [h["outcome"] for h in done["history"]] == ["error", "error", "ok"]
    assert [h["attempt"] for h in done["history"]] == [1, 2, 3]
    assert done["history"][0]["error"] == "upstream returned 503 (simulated)" and done["history"][2]["error"] is None
    # backoff: 1 s, then 2 s between attempts (+ ~0.4 s of work, scheduler slack)
    at = [h["at"] for h in done["history"]]
    from datetime import datetime
    t = [datetime.fromisoformat(x.replace("Z", "+00:00")).timestamp() for x in at]
    assert 0.9 <= t[1] - t[0] <= 3.0 and 1.9 <= t[2] - t[1] <= 4.0


def test_job_dead_after_max_attempts(api):
    o = an_order(api)
    j = api.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json={"type": "notify_customer", "simulate_failures": 5}).json()
    done = wait_job(api, j["id"], timeout=30)
    assert done["status"] == "dead" and done["attempts"] == 4 and done["next_run_at"] is None
    assert done["result"] is None and done["processed_by"] is None
    assert [h["outcome"] for h in done["history"]] == ["error"] * 4


@pytest.mark.parametrize("body,details", [
    ({}, [("type", "required")]),
    ({"type": "fax"}, [("type", "invalid")]),
    ({"type": "sync_crm", "simulate_failures": 6}, [("simulate_failures", "out_of_range")]),
    ({"type": "sync_crm", "simulate_failures": "2", "delay": 1}, [("simulate_failures", "invalid_type"), ("delay", "unknown")]),
])
def test_job_validation(api, body, details):
    o = an_order(api)
    r = api.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json=body)
    assert r.status_code == 422 and errors(r) == ("validation_error", details)


def test_job_unknown_order(api):
    r = api.post("/v1/orders/{id}/jobs", path={"id": "ord_nope"}, json={"type": "sync_crm"})
    assert r.status_code == 404 and r.json()["error"]["message"] == "order not found"
    r = api.get("/v1/jobs/{id}", path={"id": "job_nope"})
    assert r.status_code == 404 and r.json()["error"]["message"] == "job not found"


def test_job_list(api):
    orders = api.get("/v1/orders", params={"limit": 2}).json()["items"]
    a = api.post("/v1/orders/{id}/jobs", path={"id": orders[0]["id"]}, json={"type": "sync_crm"}).json()
    b = api.post("/v1/orders/{id}/jobs", path={"id": orders[1]["id"]}, json={"type": "sync_crm", "simulate_failures": 5}).json()
    c = api.post("/v1/orders/{id}/jobs", path={"id": orders[1]["id"]}, json={"type": "send_invoice"}).json()
    page = api.get("/v1/jobs", params={"limit": 2}).json()
    assert page["total_count"] == 3 and len(page["items"]) == 2 and page["next_cursor"]
    rest = api.get("/v1/jobs", params={"limit": 2, "cursor": page["next_cursor"]}).json()
    assert [j["id"] for j in page["items"] + rest["items"]] == [c["id"], b["id"], a["id"]] or \
        sorted(j["id"] for j in page["items"] + rest["items"]) == sorted([a["id"], b["id"], c["id"]])
    by_order = api.get("/v1/jobs", params={"order_id": orders[1]["id"]}).json()
    assert sorted(j["id"] for j in by_order["items"]) == sorted([b["id"], c["id"]])
    wait_job(api, a["id"])
    assert a["id"] in [j["id"] for j in api.get("/v1/jobs", params={"status": "succeeded"}).json()["items"]]
    r = api.get("/v1/jobs", params={"status": "lost", "limit": 0})
    assert errors(r) == ("validation_error", [("status", "invalid"), ("limit", "out_of_range")])


# ---- idempotency ----

def test_idempotent_create(api):
    key = str(uuid.uuid4())
    body = {"name": "Idem Co"}
    r1 = api.post("/v1/customers", json=body, headers={"Idempotency-Key": key})
    r2 = api.post("/v1/customers", json=body, headers={"Idempotency-Key": key})
    assert r1.status_code == r2.status_code == 201
    assert r2.json() == r1.json() and r2.headers.get("idempotent-replayed") == "true"
    assert "idempotent-replayed" not in r1.headers
    assert api.get("/v1/customers", params={"q": "idem co"}).json()["total_count"] == 1
    r3 = api.post("/v1/customers", json={"name": "Other"}, headers={"Idempotency-Key": key})
    assert r3.status_code == 409
    assert r3.json() == {"error": {"code": "idempotency_conflict", "message": "idempotency key reused with a different request"}}


def test_idempotent_order_and_job(api):
    cid = api.get("/v1/customers", params={"limit": 1}).json()["items"][0]["id"]
    key = str(uuid.uuid4())
    body = {"customer_id": cid, "title": "Once", "total": 5}
    a = api.post("/v1/orders", json=body, headers={"Idempotency-Key": key}).json()
    b = api.post("/v1/orders", json=body, headers={"Idempotency-Key": key}).json()
    assert a == b and a["number"] == 13
    assert api.get("/v1/me").json()["usage"]["orders"] == 13
    # the same key on another route is a conflict, failed requests are not stored
    r = api.post("/v1/orders/{id}/jobs", path={"id": a["id"]}, json={"type": "sync_crm"}, headers={"Idempotency-Key": key})
    assert r.status_code == 409
    k2 = str(uuid.uuid4())
    bad = api.post("/v1/orders", json={"customer_id": cid}, headers={"Idempotency-Key": k2})
    assert bad.status_code == 422
    ok = api.post("/v1/orders", json=body, headers={"Idempotency-Key": k2})
    assert ok.status_code == 201 and ok.json()["number"] == 14


def test_idempotency_key_too_long(api):
    r = api.post("/v1/customers", json={"name": "x"}, headers={"Idempotency-Key": "k" * 101})
    assert r.status_code == 422 and errors(r) == ("validation_error", [("Idempotency-Key", "too_long")])
