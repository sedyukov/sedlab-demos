from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import time

import httpx
import pytest

from conftest import Api, _validate, errors, new_sandbox

ERROR = {"$ref": "urn:spec#/components/schemas/Error"}


def test_health(base, health):
    assert health["status"] == "ok" and health["db"] == "ok"
    assert health["backend"] in ("go", "python")


def test_sandbox_and_me(api, edition):
    me = api.get("/v1/me").json()
    assert me["workspace"] == api.workspace
    assert me["workspace"]["auth_method"] == "sandbox"
    assert me["edition"] == edition
    assert me["quotas"] == {"customers": 100, "orders": 500, "jobs": 300}
    assert me["usage"] == {"customers": 5, "orders": 12, "jobs": 0}


def test_sandboxes_are_isolated(base):
    a, b = new_sandbox(base), new_sandbox(base)
    order = a.get("/v1/orders", params={"limit": 1}).json()["items"][0]
    r = b.get("/v1/orders/{id}", path={"id": order["id"]})
    assert r.status_code == 404 and errors(r) == ("not_found", [])
    assert r.json()["error"]["message"] == "order not found"


@pytest.mark.parametrize("header,message", [
    (None, "missing bearer token"),
    ("Basic abc", "missing bearer token"),
    ("Bearer", "missing bearer token"),
    ("Bearer abc.def.ghi", "invalid token"),
    ("Bearer not-a-jwt", "invalid token"),
])
def test_unauthorized(base, header, message):
    api = Api(base)
    r = api.get("/v1/me", token=None, headers={"Authorization": header} if header else {})
    assert r.status_code == 401
    assert r.json() == {"error": {"code": "unauthorized", "message": message}}


def test_tampered_token(api):
    h, p, s = api.token.split(".")
    claims = json.loads(base64.urlsafe_b64decode(p + "=" * (-len(p) % 4)))
    claims["sub"] = "ws_someoneelse00000"
    p2 = base64.urlsafe_b64encode(json.dumps(claims).encode()).rstrip(b"=").decode()
    r = api.get("/v1/me", token=f"{h}.{p2}.{s}")
    assert r.status_code == 401 and r.json()["error"]["message"] == "invalid token"


def _sign(secret: str, claims: dict) -> str:
    enc = lambda b: base64.urlsafe_b64encode(b).rstrip(b"=").decode()  # noqa: E731
    signing = enc(b'{"alg":"HS256","typ":"JWT"}') + "." + enc(json.dumps(claims).encode())
    return signing + "." + enc(hmac.new(secret.encode(), signing.encode(), hashlib.sha256).digest())


@pytest.mark.skipif(not os.environ.get("TEST_TOKEN_SECRET"), reason="TEST_TOKEN_SECRET not set")
def test_expired_and_deleted_workspace(api):
    secret = os.environ["TEST_TOKEN_SECRET"]
    now = int(time.time())
    ws = api.workspace["id"]
    r = api.get("/v1/me", token=_sign(secret, {"iss": "sedlab-api", "sub": ws, "am": "sandbox", "iat": now - 100, "exp": now - 1}))
    assert r.status_code == 401 and r.json()["error"]["message"] == "token expired"
    r = api.get("/v1/me", token=_sign(secret, {"iss": "sedlab-api", "sub": "ws_gone000000000000", "am": "sandbox", "iat": now, "exp": now + 60}))
    assert r.status_code == 401 and r.json()["error"]["message"] == "workspace expired"
    r = api.get("/v1/me", token=_sign(secret, {"iss": "other", "sub": ws, "am": "sandbox", "iat": now, "exp": now + 60}))
    assert r.json()["error"]["message"] == "invalid token"
    r = api.get("/v1/me", token=_sign(secret, {"iss": "sedlab-api", "sub": ws, "am": "sandbox", "iat": now, "exp": now + 60}))
    assert r.status_code == 200


def test_routes_and_methods(base, api):
    http = httpx.Client(timeout=10)
    r = http.get(base + "/v1/nope", headers={"Authorization": "Bearer " + api.token})
    assert r.status_code == 404 and r.json() == {"error": {"code": "not_found", "message": "route not found"}}
    assert r.headers["x-backend"] in ("go", "python")
    r = http.get(base + "/v1/orders/", headers={"Authorization": "Bearer " + api.token})
    assert r.status_code == 404
    r = http.put(base + "/v1/orders", headers={"Authorization": "Bearer " + api.token}, json={})
    assert r.status_code == 405 and r.json() == {"error": {"code": "method_not_allowed", "message": "method not allowed"}}


def test_body_too_large(api):
    r = api.post("/v1/customers", content=b'{"name":"' + b"a" * (70 << 10) + b'"}')
    assert r.status_code == 413
    assert r.json() == {"error": {"code": "payload_too_large", "message": "request body too large"}}


def test_request_id_echo(api):
    r = api.get("/v1/me", headers={"X-Request-Id": "test-req-12345"})
    assert r.headers["x-request-id"] == "test-req-12345"
    r = api.get("/v1/me", headers={"X-Request-Id": "bad id!"})
    assert r.headers["x-request-id"] != "bad id!"


# ---- sign-in through sedlab accounts (Russian edition) ----

def test_accounts_disabled_in_intl(base, edition):
    if edition != "intl":
        pytest.skip("intl only")
    r = Api(base).post("/v1/auth/accounts", token=None, json={"accounts_token": "x"})
    assert r.status_code == 404
    assert r.json() == {"error": {"code": "feature_disabled", "message": "sign-in via accounts is not available in this edition"}}


def _accounts_token(user: str) -> str:
    sa = pytest.importorskip("sedlab_accounts")
    return sa.Signer(sa.SANDBOX_PRIVATE_KEY).issue_token(user, "sid-" + user, "yandex").token


@pytest.fixture
def ru(base, edition):
    if edition != "ru":
        pytest.skip("ru only")
    if os.environ.get("ACCOUNTS_PROVIDER", "mock") != "mock":
        pytest.skip("needs the mock accounts provider (sandbox key)")
    return Api(base)


def test_accounts_exchange(ru):
    user = "01TESTUSER" + str(int(time.time() * 1000))
    tok = _accounts_token(user)
    r1 = ru.post("/v1/auth/accounts", token=None, json={"accounts_token": tok})
    assert r1.status_code == 200, r1.text
    ws = r1.json()["workspace"]
    assert ws["auth_method"] == "accounts"
    # cookie works too, and the same account gets the same workspace
    r2 = ru.post("/v1/auth/accounts", token=None, headers={"Cookie": "sl_auth=" + tok})
    assert r2.status_code == 200 and r2.json()["workspace"] == ws
    me = ru.get("/v1/me", token=r2.json()["access_token"]).json()
    assert me["workspace"] == ws and me["usage"]["orders"] == 12


def test_accounts_errors(ru):
    r = ru.post("/v1/auth/accounts", token=None)
    assert r.status_code == 401 and r.json()["error"]["message"] == "missing accounts session"
    r = ru.post("/v1/auth/accounts", token=None, json={"accounts_token": "a.b.c"})
    assert r.status_code == 401 and r.json()["error"]["message"] == "invalid accounts session"
    r = ru.post("/v1/auth/accounts", token=None, json={"accounts_token": 5, "x": 1})
    assert r.status_code == 422 and errors(r) == ("validation_error", [("accounts_token", "invalid_type"), ("x", "unknown")])
