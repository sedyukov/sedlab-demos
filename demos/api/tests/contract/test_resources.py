from __future__ import annotations

import base64
from datetime import datetime, timedelta, timezone

import pytest

from conftest import errors

VERR = "validation_error"


# ---- customers ----

def test_customer_crud(api):
    r = api.post("/v1/customers", json={"name": "Acme Ltd", "city": "Tomsk"})
    assert r.status_code == 201
    c = r.json()
    assert c["name"] == "Acme Ltd" and c["city"] == "Tomsk" and c["created_at"] == c["updated_at"]
    assert api.get("/v1/customers/{id}", path={"id": c["id"]}).json() == c
    r = api.patch("/v1/customers/{id}", path={"id": c["id"]}, json={"city": None})
    assert r.status_code == 200 and r.json()["city"] is None and r.json()["name"] == "Acme Ltd"
    assert r.json()["updated_at"] >= c["updated_at"]
    assert api.delete("/v1/customers/{id}", path={"id": c["id"]}).status_code == 204
    r = api.get("/v1/customers/{id}", path={"id": c["id"]})
    assert r.status_code == 404 and r.json()["error"]["message"] == "customer not found"


def test_customer_without_city(api):
    c = api.post("/v1/customers", json={"name": "No City"}).json()
    assert c["city"] is None


@pytest.mark.parametrize("body,details", [
    ({}, [("name", "required")]),
    ({"name": ""}, [("name", "too_short")]),
    ({"name": "x" * 101, "city": "y" * 61}, [("name", "too_long"), ("city", "too_long")]),
    ({"name": None, "city": 5}, [("name", "invalid_type"), ("city", "invalid_type")]),
    ({"name": "ok", "zeta": 1, "alpha": 2}, [("alpha", "unknown"), ("zeta", "unknown")]),
    ({"name": "Ünïcödé ✓" * 12}, []),  # 108 code points > 100
])
def test_customer_validation(api, body, details):
    r = api.post("/v1/customers", json=body)
    if not details:
        assert r.status_code == 422 and errors(r) == (VERR, [("name", "too_long")])
        return
    assert r.status_code == 422
    assert errors(r) == (VERR, details)
    assert r.json()["error"]["message"] == "request validation failed"


@pytest.mark.parametrize("raw,details", [
    (b"", [("body", "invalid_json")]),
    (b"{", [("body", "invalid_json")]),
    (b'{"name":"a"} x', [("body", "invalid_json")]),
    (b"[1]", [("body", "invalid_type")]),
    (b'"str"', [("body", "invalid_type")]),
    (b'{"name": NaN}', [("body", "invalid_json")]),
])
def test_malformed_body(api, raw, details):
    r = api.post("/v1/customers", content=raw, headers={"Content-Type": "application/json"})
    assert r.status_code == 422 and errors(r) == (VERR, details)


def test_customer_patch_empty_and_missing(api):
    c = api.get("/v1/customers", params={"limit": 1}).json()["items"][0]
    r = api.patch("/v1/customers/{id}", path={"id": c["id"]}, json={})
    assert r.status_code == 422 and errors(r) == (VERR, [("body", "empty")])
    r = api.patch("/v1/customers/{id}", path={"id": "cus_missing"}, json={"name": "x"})
    assert r.status_code == 404
    r = api.patch("/v1/customers/{id}", path={"id": "cus_missing"}, json={"name": ""})
    assert r.status_code == 422  # validation goes before lookup


def test_customer_delete_with_orders(api):
    o = api.get("/v1/orders", params={"limit": 1}).json()["items"][0]
    r = api.delete("/v1/customers/{id}", path={"id": o["customer_id"]})
    assert r.status_code == 409 and r.json() == {"error": {"code": "conflict", "message": "customer has orders"}}


def test_customer_search_and_sort(api, edition):
    api.post("/v1/customers", json={"name": "ЗАРЯ Мебель", "city": "ТОМСК"})
    r = api.get("/v1/customers", params={"q": "заря"}).json()
    assert [c["name"] for c in r["items"]] == ["ЗАРЯ Мебель"] and r["total_count"] == 1
    r = api.get("/v1/customers", params={"q": "томск"}).json()
    assert "ЗАРЯ Мебель" in [c["name"] for c in r["items"]]
    names = []
    cursor = None
    while True:
        params = {"sort": "name", "limit": 2}
        if cursor:
            params["cursor"] = cursor
        page = api.get("/v1/customers", params=params).json()
        assert page["total_count"] == 6
        names += [c["name"] for c in page["items"]]
        cursor = page["next_cursor"]
        if not cursor:
            break
    assert names == sorted(names, key=lambda s: s.encode()) and len(names) == 6


def test_customer_list_validation(api):
    r = api.get("/v1/customers", params={"q": "x" * 101, "sort": "city", "limit": "0", "cursor": "%%%"})
    assert r.status_code == 422
    assert errors(r) == (VERR, [("q", "too_long"), ("sort", "invalid"), ("limit", "out_of_range"), ("cursor", "invalid")])


# ---- orders ----

def first_customer(api) -> str:
    return api.get("/v1/customers", params={"limit": 1}).json()["items"][0]["id"]


def test_order_create_defaults(api, edition):
    cid = first_customer(api)
    r = api.post("/v1/orders", json={"customer_id": cid, "title": "Landing page", "total": 150000})
    assert r.status_code == 201
    o = r.json()
    assert o["number"] == 13 and o["status"] == "new" and o["tags"] == [] and o["due_date"] is None
    assert o["currency"] == ("RUB" if edition == "ru" else "USD")
    assert api.get("/v1/orders/{id}", path={"id": o["id"]}).json() == o
    r = api.post("/v1/orders", json={"customer_id": cid, "title": "T", "total": 0, "currency": "EUR",
                                     "tags": ["b2b", "urgent", "b2b"], "due_date": "2028-02-29"})
    o2 = r.json()
    assert o2["number"] == 14 and o2["currency"] == "EUR" and o2["tags"] == ["b2b", "urgent"] and o2["due_date"] == "2028-02-29"


@pytest.mark.parametrize("body,details", [
    ({}, [("customer_id", "required"), ("title", "required"), ("total", "required")]),
    ({"customer_id": "", "title": "x" * 121, "total": -1},
     [("customer_id", "too_short"), ("title", "too_long"), ("total", "out_of_range")]),
    ({"customer_id": 1, "title": "t", "total": 1.0}, [("customer_id", "invalid_type"), ("total", "invalid_type")]),
    ({"customer_id": "c", "title": "t", "total": 1e2}, [("total", "invalid_type")]),
    ({"customer_id": "c", "title": "t", "total": True}, [("total", "invalid_type")]),
    ({"customer_id": "c", "title": "t", "total": 100_000_001}, [("total", "out_of_range")]),
    ({"customer_id": "c", "title": "t", "total": 10 ** 30}, [("total", "out_of_range")]),
    ({"customer_id": "c", "title": "t", "total": 1, "currency": "GBP", "status": "done"},
     [("currency", "invalid"), ("status", "unknown")]),
    ({"customer_id": "c", "title": "t", "total": 1, "tags": "a"}, [("tags", "invalid_type")]),
    ({"customer_id": "c", "title": "t", "total": 1, "tags": ["a", "b", "c", "d", "e", "f"]}, [("tags", "too_long")]),
    ({"customer_id": "c", "title": "t", "total": 1, "tags": ["ok", "Bad", 3, "x" * 25]},
     [("tags[1]", "invalid"), ("tags[2]", "invalid_type"), ("tags[3]", "invalid")]),
    ({"customer_id": "c", "title": "t", "total": 1, "due_date": "2026-02-30"}, [("due_date", "invalid")]),
    ({"customer_id": "c", "title": "t", "total": 1, "due_date": "20261201"}, [("due_date", "invalid")]),
    ({"customer_id": "c", "title": "t", "total": 1, "due_date": 20261201}, [("due_date", "invalid_type")]),
    ({"customer_id": "c", "title": None, "total": None, "tags": None}, [("title", "invalid_type"), ("total", "invalid_type"), ("tags", "invalid_type")]),
])
def test_order_validation(api, body, details):
    r = api.post("/v1/orders", json=body)
    assert r.status_code == 422, r.text
    assert errors(r) == (VERR, details)


def test_order_unknown_customer(api):
    r = api.post("/v1/orders", json={"customer_id": "cus_nope", "title": "t", "total": 1})
    assert r.status_code == 422 and errors(r) == (VERR, [("customer_id", "not_found")])


def test_status_workflow(api):
    cid = first_customer(api)
    o = api.post("/v1/orders", json={"customer_id": cid, "title": "Flow", "total": 1}).json()
    oid = {"id": o["id"]}
    r = api.patch("/v1/orders/{id}", path=oid, json={"status": "done"})
    assert r.status_code == 409
    assert r.json() == {"error": {"code": "invalid_transition", "message": "cannot change status from new to done",
                                  "details": [{"field": "status", "issue": "invalid"}]}}
    for st in ("confirmed", "confirmed", "in_progress", "done"):
        r = api.patch("/v1/orders/{id}", path=oid, json={"status": st})
        assert r.status_code == 200 and r.json()["status"] == st
    r = api.patch("/v1/orders/{id}", path=oid, json={"status": "cancelled"})
    assert r.status_code == 409 and r.json()["error"]["message"] == "cannot change status from done to cancelled"
    r = api.patch("/v1/orders/{id}", path=oid, json={"status": "paid"})
    assert r.status_code == 422 and errors(r) == (VERR, [("status", "invalid")])


def test_order_patch_fields(api):
    orders = api.get("/v1/orders", params={"limit": 2}).json()["items"]
    o, other = orders
    r = api.patch("/v1/orders/{id}", path={"id": o["id"]},
                  json={"title": "Renamed", "total": 99, "tags": [], "due_date": None, "customer_id": other["customer_id"]})
    assert r.status_code == 200
    p = r.json()
    assert (p["title"], p["total"], p["tags"], p["due_date"], p["customer_id"]) == ("Renamed", 99, [], None, other["customer_id"])
    assert p["number"] == o["number"] and p["created_at"] == o["created_at"]
    r = api.patch("/v1/orders/{id}", path={"id": o["id"]}, json={"customer_id": "cus_nope"})
    assert r.status_code == 422 and errors(r) == (VERR, [("customer_id", "not_found")])
    r = api.patch("/v1/orders/{id}", path={"id": "ord_nope"}, json={"customer_id": "cus_nope"})
    assert r.status_code == 404
    r = api.patch("/v1/orders/{id}", path={"id": o["id"]}, json={})
    assert errors(r) == (VERR, [("body", "empty")])


def test_order_delete_cascades_jobs(api):
    o = api.get("/v1/orders", params={"limit": 1}).json()["items"][0]
    j = api.post("/v1/orders/{id}/jobs", path={"id": o["id"]}, json={"type": "sync_crm"}).json()
    assert api.delete("/v1/orders/{id}", path={"id": o["id"]}).status_code == 204
    assert api.get("/v1/jobs/{id}", path={"id": j["id"]}).status_code == 404
    assert api.delete("/v1/orders/{id}", path={"id": o["id"]}).status_code == 404


def _all(api, params):
    items, cursor, totals = [], None, set()
    while True:
        p = dict(params)
        if cursor:
            p["cursor"] = cursor
        page = api.get("/v1/orders", params=p).json()
        totals.add(page["total_count"])
        items += page["items"]
        cursor = page["next_cursor"]
        if not cursor:
            return items, totals


@pytest.mark.parametrize("sort", ["created_at", "-created_at", "total", "-total", "number", "-number"])
def test_order_pagination(api, sort):
    items, totals = _all(api, {"sort": sort, "limit": 5})
    assert totals == {12} and len(items) == 12 and len({o["id"] for o in items}) == 12
    col = sort.lstrip("-")
    keys = [(o[col], o["id"]) for o in items]
    assert keys == sorted(keys, reverse=sort.startswith("-"))


def test_order_filters(api):
    every = _all(api, {"limit": 100})[0]
    by = lambda f: sorted(o["id"] for o in every if f(o))  # noqa: E731
    ids = lambda params: sorted(o["id"] for o in _all(api, params)[0])  # noqa: E731
    assert ids({"status": "new"}) == by(lambda o: o["status"] == "new")
    assert ids({"status": "new,confirmed"}) == by(lambda o: o["status"] in ("new", "confirmed"))
    assert ids({"tag": "b2b"}) == by(lambda o: "b2b" in o["tags"])
    cid = every[0]["customer_id"]
    assert ids({"customer_id": cid}) == by(lambda o: o["customer_id"] == cid)
    assert ids({"min_total": 10000, "max_total": 500000}) == by(lambda o: 10000 <= o["total"] <= 500000)
    word = every[0]["title"].split()[0]
    assert ids({"q": word.upper()}) == by(lambda o: word.lower() in o["title"].lower())
    today = datetime.now(timezone.utc).date()
    week = (today - timedelta(days=7)).isoformat()
    assert ids({"created_from": week, "created_to": today.isoformat()}) == by(lambda o: o["created_at"][:10] >= week)
    assert ids({"created_to": "2000-01-01"}) == []
    assert ids({"status": "new", "tag": "urgent", "sort": "total"}) == by(lambda o: o["status"] == "new" and "urgent" in o["tags"])


def test_order_list_validation(api):
    r = api.get("/v1/orders", params={"status": "new,bogus", "q": "x" * 101, "created_from": "2026-13-01",
                                       "created_to": "yesterday", "min_total": "-1", "max_total": "1.5",
                                       "sort": "title", "limit": "abc", "cursor": "zzz"})
    assert r.status_code == 422
    assert errors(r) == (VERR, [("status", "invalid"), ("q", "too_long"), ("created_from", "invalid"), ("created_to", "invalid"),
                                ("min_total", "out_of_range"), ("max_total", "invalid_type"), ("sort", "invalid"),
                                ("limit", "invalid_type"), ("cursor", "invalid")])
    r = api.get("/v1/orders", params={"limit": "101", "min_total": "+5"})
    assert errors(r) == (VERR, [("min_total", "invalid_type"), ("limit", "out_of_range")])


def test_cursor_bound_to_sort(api):
    page = api.get("/v1/orders", params={"sort": "total", "limit": 3}).json()
    r = api.get("/v1/orders", params={"sort": "-total", "limit": 3, "cursor": page["next_cursor"]})
    assert r.status_code == 422 and errors(r) == (VERR, [("cursor", "invalid")])
    forged = base64.urlsafe_b64encode(b"total|abc|ord_x").rstrip(b"=").decode()
    r = api.get("/v1/orders", params={"sort": "total", "cursor": forged})
    assert errors(r) == (VERR, [("cursor", "invalid")])
    padded = base64.urlsafe_b64encode(b"total|1|ord_xy").decode()
    assert padded.endswith("=")
    r = api.get("/v1/orders", params={"sort": "total", "cursor": padded})
    assert errors(r) == (VERR, [("cursor", "invalid")])
