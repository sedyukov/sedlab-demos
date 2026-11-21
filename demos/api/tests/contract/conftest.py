"""Contract tests: one suite, run against any implementation.

    API_BASE_URL=http://127.0.0.1:18080/go  pytest -q tests/contract      # Go
    API_BASE_URL=http://127.0.0.1:18080/py  pytest -q tests/contract      # Python
    + API_OTHER_BASE_URL=<the other one>  → test_parity.py replays requests on both and diffs the answers
    + TEST_TOKEN_SECRET=<API_TOKEN_SECRET of the servers> → expired / foreign-workspace token checks

Every response is validated against contract/openapi.yaml (status code must be documented, body must match).
"""
from __future__ import annotations

import os
import time
from pathlib import Path
from typing import Any

import httpx
import pytest
import yaml
from jsonschema import Draft202012Validator
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012

SPEC_PATH = Path(__file__).resolve().parents[2] / "contract" / "openapi.yaml"
SPEC = yaml.safe_load(SPEC_PATH.read_text(encoding="utf-8"))
REGISTRY = Registry().with_resource("urn:spec", Resource.from_contents(SPEC, default_specification=DRAFT202012))


def _ptr(*parts: str) -> str:
    return "/".join(p.replace("~", "~0").replace("/", "~1") for p in parts)


def _deref(node: dict) -> tuple[dict, str]:
    """Follows a $ref inside the spec; returns (node, json pointer)."""
    ptr = None
    while "$ref" in node:
        ptr = node["$ref"].lstrip("#")
        node = SPEC
        for part in ptr.strip("/").split("/"):
            node = node[part.replace("~1", "/").replace("~0", "~")]
    return node, ptr


def check_contract(method: str, template: str, resp: httpx.Response) -> None:
    op = SPEC["paths"][template][method.lower()]
    responses = op["responses"]
    status = str(resp.status_code)
    assert status != "500", resp.text
    if status not in responses and status in ("404", "405", "413", "429"):  # generic: route, method, size, rate
        _validate({"$ref": "urn:spec#/components/schemas/Error"}, resp.json())
        return
    assert status in responses, f"{method} {template}: undocumented status {status}: {resp.text[:300]}"
    r, ptr = _deref(responses[status])
    content = r.get("content")
    if not content:
        assert resp.content == b"", resp.text
        return
    assert resp.headers["content-type"].startswith("application/json")
    base = ptr or "/" + _ptr("paths", template, method.lower(), "responses", status)
    _validate({"$ref": "urn:spec#" + base + "/content/application~1json/schema"}, resp.json())


def _validate(schema: dict, value: Any) -> None:
    v = Draft202012Validator(schema, registry=REGISTRY)
    errors = sorted(v.iter_errors(value), key=lambda e: list(e.path))
    assert not errors, "; ".join(f"{list(e.path)}: {e.message}" for e in errors[:5])


class Api:
    """Thin client: every call is checked against the contract."""

    def __init__(self, base: str, token: str | None = None):
        self.base = base.rstrip("/")
        self.token = token
        self.http = httpx.Client(timeout=15)

    def call(self, method: str, template: str, *, path: dict | None = None, token: str | None | bool = True,
             check: bool = True, **kw) -> httpx.Response:
        url = template
        for k, v in (path or {}).items():
            url = url.replace("{" + k + "}", v)
        headers = dict(kw.pop("headers", {}) or {})
        tok = self.token if token is True else token
        if tok:
            headers["Authorization"] = "Bearer " + tok
        resp = self.http.request(method, self.base + url, headers=headers, **kw)
        assert resp.headers.get("x-backend") in ("go", "python"), resp.headers
        assert resp.headers.get("x-request-id"), resp.headers
        if check:
            check_contract(method, template, resp)
        return resp

    def get(self, t: str, **kw) -> httpx.Response:
        return self.call("GET", t, **kw)

    def post(self, t: str, **kw) -> httpx.Response:
        return self.call("POST", t, **kw)

    def patch(self, t: str, **kw) -> httpx.Response:
        return self.call("PATCH", t, **kw)

    def delete(self, t: str, **kw) -> httpx.Response:
        return self.call("DELETE", t, **kw)


def new_sandbox(base: str) -> Api:
    api = Api(base)
    r = api.post("/v1/auth/sandbox", token=None)
    assert r.status_code == 201, r.text
    api.token = r.json()["access_token"]
    api.workspace = r.json()["workspace"]
    return api


@pytest.fixture(scope="session")
def base() -> str:
    b = os.environ.get("API_BASE_URL")
    if not b:
        pytest.skip("API_BASE_URL is not set")
    return b


@pytest.fixture(scope="session")
def health(base: str) -> dict:
    r = Api(base).get("/health", token=None)
    assert r.status_code == 200
    return r.json()


@pytest.fixture(scope="session")
def edition(health: dict) -> str:
    return health["edition"]


@pytest.fixture
def api(base: str) -> Api:
    """A fresh sandbox (5 customers, 12 orders) per test."""
    return new_sandbox(base)


def errors(resp: httpx.Response) -> tuple[str, list[tuple[str, str]]]:
    e = resp.json()["error"]
    return e["code"], [(d["field"], d["issue"]) for d in e.get("details", [])]


def wait_job(api: Api, job_id: str, statuses: tuple[str, ...] = ("succeeded", "dead"), timeout: float = 25) -> dict:
    deadline = time.time() + timeout
    while True:
        j = api.get("/v1/jobs/{id}", path={"id": job_id}).json()
        if j["status"] in statuses or time.time() > deadline:
            return j
        time.sleep(0.3)
