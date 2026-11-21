"""RU edition: sign-in and per-user limits through the sedlab accounts service (module sedlab_accounts).

Every live provider call goes through accounts.live_call: consume the user's limit → call → refund if the call fails.
Not signed in, limit used up, auth down, provider key missing → the caller falls back to the mock and shows why.
The mock accounts provider never allows live (the module's own guard), so production must use ACCOUNTS_PROVIDER=http.

Resources (auth limits.json): gigachat, yandex-llm, yandex-stt (units = audio seconds, ≤ 60 per call), yandex-tts.
"""
from __future__ import annotations

import logging
import math
import uuid
from dataclasses import dataclass, field
from typing import Any, Callable, Mapping, TypeVar

from sedlab_accounts import Client as AccountsClient
from sedlab_accounts import Env as AccountsEnv
from sedlab_accounts import ModuleError
from sedlab_accounts import from_env as accounts_from_env

T = TypeVar("T")
log = logging.getLogger("voice.accounts")

LLM_RESOURCE = {"gigachat": "gigachat", "yandexgpt": "yandex-llm"}
STT_RESOURCE, TTS_RESOURCE = "yandex-stt", "yandex-tts"
MAX_STT_UNITS = 60

# accounts reason → notice key shown on the page
NOTICE = {"unauthenticated": "acc_login", "user_daily": "acc_limit", "global_daily": "acc_limit",
          "budget_month": "acc_limit", "unavailable": "acc_down"}


def stt_units(duration: float | None) -> int:
    return max(1, min(MAX_STT_UNITS, math.ceil(duration or 1)))


@dataclass
class Gate:
    """One request's view of the accounts service: token + collected reasons for the UI."""

    acc: AccountsClient
    token: str
    reasons: list[str] = field(default_factory=list)
    user_left: int | None = None

    def call(self, resource: str, units: int, fn: Callable[[], T]) -> T | None:
        """Live call under the user's limit, or None (caller serves the mock). fn raising → refunded and re-raised."""
        res, m = self.acc.live_call(self.token, resource, units, uuid.uuid4().hex, fn)
        if res is None:
            self.reasons.append(m.reason)
        else:
            self.user_left = m.user_left
        return res

    def notices(self) -> list[str]:
        out: list[str] = []
        for r in self.reasons:
            n = NOTICE.get(r)
            if n and n not in out:
                out.append(n)
        return out


class Accounts:
    def __init__(self, environ: Mapping[str, str], enabled: bool) -> None:
        self.enabled = enabled
        self.config_error = ""
        self.client: AccountsClient | None = None
        if not enabled:
            return
        try:
            self.client = accounts_from_env(AccountsEnv("ACCOUNTS_", dict(environ)))
        except ModuleError as e:  # bad config must not take the demo down: fail closed (mock, never live)
            self.config_error = e.code.value
            log.warning("accounts config error: %s; live mode is off", e.code.value)
            self.client = accounts_from_env(AccountsEnv("ACCOUNTS_", {"ACCOUNTS_PROVIDER": "mock"}))

    def record_consent(self, headers: Any, kind: str, version: str) -> bool:
        """Consent log on auth for signed-in users (accounts.record_consent). Anonymous, auth down or the module
        without the method → False: the page keeps the session mark and the upload is never blocked."""
        fn = getattr(self.client, "record_consent", None) if self.client else None
        token = self.client.token_from_request(headers) if self.client else ""
        if fn is None or not token:
            return False
        try:
            r = fn(token, kind, version)
        except Exception as e:  # noqa: BLE001 — fail open for the upload, the session mark stays
            log.warning("consent log failed: %s", getattr(e, "code", type(e).__name__))
            return False
        return bool(getattr(r, "ok", r))

    def gate(self, headers: Any) -> Gate | None:
        if not self.client:
            return None
        return Gate(self.client, self.client.token_from_request(headers))
