"""FastAPI app. The OpenAPI document is the shared contract file (contract-first), so FastAPI's own schema
generation is off; handlers live in service.py and run in the threadpool (SQLite is synchronous)."""
from __future__ import annotations

import logging
import os
import re
import secrets
import threading
from contextlib import asynccontextmanager
from typing import Callable

from fastapi import FastAPI, Request
from fastapi.responses import FileResponse, Response
from starlette.concurrency import run_in_threadpool
from starlette.exceptions import HTTPException as StarletteHTTPException

from .core import ApiError, Config, connect, migrate
from .service import Req, Result, Service, dumps
from .worker import Worker, run_housekeeping

MAX_BODY = 64 << 10
_REQ_ID = re.compile(r"[A-Za-z0-9-]{8,64}")

log = logging.getLogger("api_py")


def create_app(cfg: Config | None = None) -> FastAPI:
    cfg = cfg or Config()
    migrate(connect(cfg.db_path), cfg.migrations_dir)
    svc = Service(cfg)
    stop = threading.Event()

    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        threads = [threading.Thread(target=run_housekeeping, args=(cfg, stop), daemon=True, name="housekeeping")]
        if cfg.worker:
            threads.append(threading.Thread(target=Worker(cfg, stop).run, daemon=True, name="worker"))
        for t in threads:
            t.start()
        yield
        stop.set()

    app = FastAPI(title="Orders API (Python)", lifespan=lifespan, openapi_url=None, docs_url=None, redoc_url=None)
    app.state.service = svc
    app.router.redirect_slashes = False  # "/v1/orders/" is 404, as in Go

    def base_headers(request: Request) -> dict[str, str]:
        rid = request.headers.get("x-request-id", "")
        if not _REQ_ID.fullmatch(rid):
            rid = secrets.token_hex(8)
        return {"X-Backend": "python", "X-Request-Id": rid, "Cache-Control": "no-store"}

    def error_response(e: ApiError, headers: dict[str, str]) -> Response:
        body = {"code": e.code, "message": e.message}
        if e.details:
            body["details"] = e.details
        if e.retry_after > 0:
            headers["Retry-After"] = str(e.retry_after)
        return Response(dumps({"error": body}), status_code=e.status, media_type="application/json", headers=headers)

    def client_ip(request: Request) -> str:
        xff = request.headers.get("x-forwarded-for")
        if xff:
            return xff.split(",")[-1].strip()
        return request.client.host if request.client else ""

    async def read_body(request: Request) -> bytes | None:
        buf = bytearray()
        async for chunk in request.stream():
            buf += chunk
            if len(buf) > MAX_BODY:
                return None
        return bytes(buf)

    def dispatch(h: Callable[[Req], Result], auth: bool, idem: bool, q: Req) -> Result:
        if auth:
            q.p = svc.authenticate(q)
            svc.limit_ws(q.p.ws)
        if idem and q.p is not None:
            return svc.idempotent(q, h)
        return h(q)

    def route(method: str, path: str, h: Callable[[Req], Result], auth: bool = True, idem: bool = False) -> None:
        async def endpoint(request: Request) -> Response:
            headers = base_headers(request)
            body = await read_body(request)
            if body is None:
                return error_response(ApiError(413, "payload_too_large", "request body too large"), headers)
            q = Req(headers={k.lower(): v for k, v in request.headers.items()}, cookies=dict(request.cookies),
                    query=_multi(request), params=dict(request.path_params), body=body,
                    client_ip=client_ip(request), path=request.url.path)
            try:
                res = await run_in_threadpool(dispatch, h, auth, idem, q)
            except ApiError as e:
                return error_response(e, headers)
            except Exception:  # noqa: BLE001
                log.exception("%s %s", method, path)
                return error_response(ApiError(500, "internal", "internal error"), headers)
            if res.headers:
                headers.update(res.headers)
            if res.status == 204:
                return Response(status_code=204, headers=headers)
            content = res.raw if res.raw is not None else dumps(res.value)
            return Response(content, status_code=res.status, media_type="application/json", headers=headers)

        endpoint.__name__ = h.__name__
        app.add_api_route(path, endpoint, methods=[method], include_in_schema=False)

    route("GET", "/health", svc.health, auth=False)
    route("POST", "/v1/auth/sandbox", svc.create_sandbox, auth=False)
    route("POST", "/v1/auth/accounts", svc.exchange_accounts, auth=False)
    route("GET", "/v1/me", svc.me)
    route("GET", "/v1/customers", svc.list_customers)
    route("POST", "/v1/customers", svc.create_customer, idem=True)
    route("GET", "/v1/customers/{id}", svc.get_customer)
    route("PATCH", "/v1/customers/{id}", svc.update_customer)
    route("DELETE", "/v1/customers/{id}", svc.delete_customer)
    route("GET", "/v1/orders", svc.list_orders)
    route("POST", "/v1/orders", svc.create_order, idem=True)
    route("GET", "/v1/orders/{id}", svc.get_order)
    route("PATCH", "/v1/orders/{id}", svc.update_order)
    route("DELETE", "/v1/orders/{id}", svc.delete_order)
    route("POST", "/v1/orders/{id}/jobs", svc.enqueue_job, idem=True)
    route("GET", "/v1/jobs", svc.list_jobs)
    route("GET", "/v1/jobs/{id}", svc.get_job)

    @app.get("/openapi.yaml", include_in_schema=False)
    def contract() -> FileResponse:
        return FileResponse(os.path.join(cfg.contract_dir, "openapi.yaml"), media_type="application/yaml",
                            headers={"Cache-Control": "no-cache"})

    @app.exception_handler(StarletteHTTPException)
    async def http_error(request: Request, exc: StarletteHTTPException) -> Response:
        headers = base_headers(request)
        if exc.status_code == 405:
            return error_response(ApiError(405, "method_not_allowed", "method not allowed"), headers)
        return error_response(ApiError(404, "not_found", "route not found"), headers)

    return app


def _multi(request: Request) -> dict[str, list[str]]:
    out: dict[str, list[str]] = {}
    for k, v in request.query_params.multi_items():
        out.setdefault(k, []).append(v)
    return out

